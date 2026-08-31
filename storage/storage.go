package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/gate/pkg/util/uuid"
)

// Storage manages persistent storage for user skin assignments and cached textures.
type Storage struct {
	dataDir     string
	usersPath   string
	cachePath   string
	mu          sync.RWMutex
	users       map[string]*model.UserSkin // key: lowercase uuid string
	nameToUUID  map[string]string          // key: lowercase player username -> uuid string
	cache       map[string]*model.SkinData // key: lowercase skin name or url
	cacheTTL    time.Duration
}

type usersDataFile struct {
	Users []*model.UserSkin `json:"users"`
}

type cacheDataFile struct {
	Skins []*model.SkinData `json:"skins"`
}

// NewStorage initializes storage directory and paths.
func NewStorage(dataDir string, cacheTTL time.Duration) *Storage {
	if cacheTTL <= 0 {
		cacheTTL = 24 * time.Hour
	}
	return &Storage{
		dataDir:    dataDir,
		usersPath:  filepath.Join(dataDir, "users_skins.json"),
		cachePath:  filepath.Join(dataDir, "skins_cache.json"),
		users:      make(map[string]*model.UserSkin),
		nameToUUID: make(map[string]string),
		cache:      make(map[string]*model.SkinData),
		cacheTTL:   cacheTTL,
	}
}

// Load reads persisted user skin mappings and texture caches from disk.
func (s *Storage) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return fmt.Errorf("failed to create storage dir: %w", err)
	}

	// 1. Load users
	if data, err := os.ReadFile(s.usersPath); err == nil {
		var uFile usersDataFile
		if err := json.Unmarshal(data, &uFile); err == nil {
			s.users = make(map[string]*model.UserSkin, len(uFile.Users))
			s.nameToUUID = make(map[string]string, len(uFile.Users))
			for _, u := range uFile.Users {
				if u != nil {
					key := strings.ToLower(u.PlayerUUID.String())
					s.users[key] = u
					if u.Username != "" {
						s.nameToUUID[strings.ToLower(u.Username)] = key
					}
				}
			}
		}
	}

	// 2. Load cache
	if data, err := os.ReadFile(s.cachePath); err == nil {
		var cFile cacheDataFile
		if err := json.Unmarshal(data, &cFile); err == nil {
			s.cache = make(map[string]*model.SkinData, len(cFile.Skins))
			for _, sk := range cFile.Skins {
				if sk != nil {
					s.cache[strings.ToLower(sk.Name)] = sk
				}
			}
		}
	}

	return nil
}

// SaveUsers persists user skin mappings to disk atomically.
func (s *Storage) SaveUsers() error {
	s.mu.RLock()
	usersList := make([]*model.UserSkin, 0, len(s.users))
	for _, u := range s.users {
		usersList = append(usersList, u)
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(usersDataFile{Users: usersList}, "", "  ")
	if err != nil {
		return err
	}

	return s.atomicWrite(s.usersPath, data)
}

// SaveCache persists the texture cache to disk atomically.
func (s *Storage) SaveCache() error {
	s.mu.RLock()
	now := time.Now()
	skinsList := make([]*model.SkinData, 0, len(s.cache))
	for _, sk := range s.cache {
		// Prune expired entries
		if now.Sub(sk.FetchedAt) < s.cacheTTL {
			skinsList = append(skinsList, sk)
		}
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(cacheDataFile{Skins: skinsList}, "", "  ")
	if err != nil {
		return err
	}

	return s.atomicWrite(s.cachePath, data)
}

// SetUserSkin associates a player UUID and username with a skin and persists.
func (s *Storage) SetUserSkin(playerID uuid.UUID, username string, skinName string, skin *model.SkinData) error {
	s.mu.Lock()
	key := strings.ToLower(playerID.String())
	user := &model.UserSkin{
		PlayerUUID: playerID,
		Username:   username,
		SkinName:   skinName,
		Skin:       skin,
		UpdatedAt:  time.Now().UTC(),
	}
	s.users[key] = user
	if username != "" {
		s.nameToUUID[strings.ToLower(username)] = key
	}
	s.mu.Unlock()

	return s.SaveUsers()
}

// RemoveUserSkin clears a player's custom skin and persists.
func (s *Storage) RemoveUserSkin(playerID uuid.UUID) (bool, error) {
	s.mu.Lock()
	key := strings.ToLower(playerID.String())
	user, exists := s.users[key]
	if !exists {
		s.mu.Unlock()
		return false, nil
	}
	delete(s.users, key)
	if user.Username != "" {
		delete(s.nameToUUID, strings.ToLower(user.Username))
	}
	s.mu.Unlock()

	return true, s.SaveUsers()
}

// GetUserSkin returns the custom skin assigned to a player UUID.
func (s *Storage) GetUserSkin(playerID uuid.UUID) *model.UserSkin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[strings.ToLower(playerID.String())]
}

// GetUserSkinByUsername returns the custom skin assigned to a username.
func (s *Storage) GetUserSkinByUsername(username string) *model.UserSkin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, exists := s.nameToUUID[strings.ToLower(username)]
	if !exists {
		return nil
	}
	return s.users[key]
}

// SetCachedSkin adds a skin to the texture cache.
func (s *Storage) SetCachedSkin(identifier string, skin *model.SkinData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[strings.ToLower(identifier)] = skin
}

// GetCachedSkin returns a valid (non-expired) cached skin, or nil.
func (s *Storage) GetCachedSkin(identifier string) *model.SkinData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sk, exists := s.cache[strings.ToLower(identifier)]
	if !exists || sk == nil {
		return nil
	}
	if time.Since(sk.FetchedAt) > s.cacheTTL {
		return nil
	}
	return sk
}

// AllUsers returns a copy of all user skin mappings.
func (s *Storage) AllUsers() []*model.UserSkin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*model.UserSkin, 0, len(s.users))
	for _, u := range s.users {
		res = append(res, u)
	}
	return res
}

func (s *Storage) atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
