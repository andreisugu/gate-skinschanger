package manager

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/andreisugu/gate-skinschanger/fetcher"
	"github.com/andreisugu/gate-skinschanger/model"
	"github.com/andreisugu/gate-skinschanger/storage"
	"go.minekube.com/gate/pkg/edition/java/profile"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

type ConfigProvider interface {
	Get() *ConfigSnapshot
}

// ConfigSnapshot contains configuration fields needed by the Manager.
type ConfigSnapshot struct {
	Enabled         bool
	AutoSkinOffline bool
	DefaultSkin     string
	CacheTTL        time.Duration
	Cooldown        time.Duration
	RefreshOnChange bool
	CustomSkins     map[string]struct {
		Value     string
		Signature string
		Model     string
	}
}

// Manager coordinates skin resolution, persistence, profile injection, and in-game refresh.
type Manager struct {
	proxy          *proxy.Proxy
	fetcher        fetcher.Fetcher
	storage        *storage.Storage
	configProvider func() *ConfigSnapshot
	cooldowns      map[uuid.UUID]time.Time
	cooldownMu     sync.Mutex
}

// NewManager constructs a new skinschanger Manager.
func NewManager(
	p *proxy.Proxy,
	f fetcher.Fetcher,
	s *storage.Storage,
	cfgProvider func() *ConfigSnapshot,
) *Manager {
	return &Manager{
		proxy:          p,
		fetcher:        f,
		storage:        s,
		configProvider: cfgProvider,
		cooldowns:      make(map[uuid.UUID]time.Time),
	}
}

// CheckCooldown checks if a player is on cooldown for skin changes.
func (m *Manager) CheckCooldown(playerID uuid.UUID) (bool, time.Duration) {
	cfg := m.configProvider()
	if cfg == nil || cfg.Cooldown <= 0 {
		return false, 0
	}

	m.cooldownMu.Lock()
	defer m.cooldownMu.Unlock()

	last, exists := m.cooldowns[playerID]
	if !exists {
		return false, 0
	}

	elapsed := time.Since(last)
	if elapsed < cfg.Cooldown {
		return true, cfg.Cooldown - elapsed
	}
	return false, 0
}

// RecordCooldown marks the current timestamp as player's last skin change.
func (m *Manager) RecordCooldown(playerID uuid.UUID) {
	m.cooldownMu.Lock()
	defer m.cooldownMu.Unlock()
	m.cooldowns[playerID] = time.Now()
}

// ResolveSkin resolves skin textures from cache, custom config, or external providers.
func (m *Manager) ResolveSkin(ctx context.Context, target string) (*model.SkinData, error) {
	clean := strings.TrimSpace(target)
	if clean == "" {
		return nil, fetcher.ErrSkinNotFound
	}
	lower := strings.ToLower(clean)

	// 1. Check in-memory cache
	if cached := m.storage.GetCachedSkin(lower); cached != nil {
		return cached, nil
	}

	// 2. Check pre-configured custom skins
	cfg := m.configProvider()
	if cfg != nil && cfg.CustomSkins != nil {
		if cs, ok := cfg.CustomSkins[lower]; ok && cs.Value != "" {
			skin := &model.SkinData{
				Name:      clean,
				Value:     cs.Value,
				Signature: cs.Signature,
				Model:     model.SkinModel(cs.Model),
				FetchedAt: time.Now().UTC(),
			}
			skin.ExtractMetadata()
			m.storage.SetCachedSkin(lower, skin)
			return skin, nil
		}
	}

	// 3. Check if target is UUID vs Nickname
	var skin *model.SkinData
	var err error
	if parsedUUID, parseErr := uuid.Parse(clean); parseErr == nil {
		skin, err = m.fetcher.FetchSkinByUUID(ctx, parsedUUID)
	} else {
		skin, err = m.fetcher.FetchSkinByName(ctx, clean)
	}

	if err != nil {
		return nil, err
	}

	// Save to cache
	m.storage.SetCachedSkin(lower, skin)
	_ = m.storage.SaveCache()
	return skin, nil
}

// ApplySkin sets and applies a skin to a player.
func (m *Manager) ApplySkin(ctx context.Context, p proxy.Player, target string) (*model.SkinData, error) {
	skin, err := m.ResolveSkin(ctx, target)
	if err != nil {
		return nil, err
	}

	// Persist user skin mapping
	if err := m.storage.SetUserSkin(p.ID(), p.Username(), target, skin); err != nil {
		return nil, fmt.Errorf("failed to save skin mapping: %w", err)
	}

	// In-place profile property injection
	InjectPlayerProfileProperty(p, skin.ToProperty())

	// Trigger in-game refresh if connected to a backend server
	m.RefreshPlayerInGame(ctx, p)

	return skin, nil
}

// ApplySkinURL sets and applies a custom skin from an image URL via Mineskin.
func (m *Manager) ApplySkinURL(ctx context.Context, p proxy.Player, imageURL string, skinModel model.SkinModel) (*model.SkinData, error) {
	skin, err := m.fetcher.FetchSkinByURL(ctx, imageURL, skinModel)
	if err != nil {
		return nil, err
	}

	// Persist
	skinName := "url:" + imageURL
	if err := m.storage.SetUserSkin(p.ID(), p.Username(), skinName, skin); err != nil {
		return nil, fmt.Errorf("failed to save skin mapping: %w", err)
	}

	// Cache
	m.storage.SetCachedSkin(imageURL, skin)
	_ = m.storage.SaveCache()

	// In-place injection & refresh
	InjectPlayerProfileProperty(p, skin.ToProperty())
	m.RefreshPlayerInGame(ctx, p)

	return skin, nil
}

// ClearSkin resets a player's skin back to their default/Mojang skin.
func (m *Manager) ClearSkin(ctx context.Context, p proxy.Player) error {
	_, _ = m.storage.RemoveUserSkin(p.ID())

	var skinProp profile.Property
	// If online-mode player, restore genuine Mojang skin
	if p.OnlineMode() {
		if nativeSkin, err := m.ResolveSkin(ctx, p.Username()); err == nil && nativeSkin != nil {
			skinProp = nativeSkin.ToProperty()
		}
	} else {
		// Offline-mode: check if auto_skin_offline restores their name's genuine skin
		cfg := m.configProvider()
		if cfg != nil && cfg.AutoSkinOffline {
			if nativeSkin, err := m.ResolveSkin(ctx, p.Username()); err == nil && nativeSkin != nil {
				skinProp = nativeSkin.ToProperty()
			}
		}
	}

	InjectPlayerProfileProperty(p, skinProp)
	m.RefreshPlayerInGame(ctx, p)
	return nil
}

// RefreshPlayerInGame re-connects the player to their current backend server so the updated skin takes effect immediately.
func (m *Manager) RefreshPlayerInGame(ctx context.Context, p proxy.Player) {
	cfg := m.configProvider()
	if cfg == nil || !cfg.RefreshOnChange || p == nil {
		return
	}

	currSrv := p.CurrentServer()
	if currSrv == nil || currSrv.Server() == nil {
		return
	}

	// Reconnect to current server seamlessly to force backend respawn with updated forwarded profile
	_ = p.CreateConnectionRequest(currSrv.Server()).ConnectWithIndication(ctx)
}

// ProcessProfileRequest modifies the GameProfile during login (for GameProfileRequestEvent).
func (m *Manager) ProcessProfileRequest(ctx context.Context, orig profile.GameProfile, onlineMode bool) profile.GameProfile {
	cfg := m.configProvider()
	if cfg != nil && !cfg.Enabled {
		return orig
	}

	var activeSkin *model.SkinData

	// 1. Check if user has explicit skin mapped by UUID
	if u := m.storage.GetUserSkin(orig.ID); u != nil && u.Skin != nil {
		activeSkin = u.Skin
	}

	// 2. Check if user has explicit skin mapped by Username
	if activeSkin == nil {
		if u := m.storage.GetUserSkinByUsername(orig.Name); u != nil && u.Skin != nil {
			activeSkin = u.Skin
		}
	}

	// 3. If offline mode and auto_skin_offline is true: try fetching real Mojang skin
	if activeSkin == nil && !onlineMode && cfg != nil && cfg.AutoSkinOffline {
		if cached := m.storage.GetCachedSkin(orig.Name); cached != nil {
			activeSkin = cached
		} else {
			// Synchronous fetch with short timeout during login
			fetchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			if fetched, err := m.ResolveSkin(fetchCtx, orig.Name); err == nil && fetched != nil {
				activeSkin = fetched
			}
			cancel()
		}
	}

	// 4. Default skin fallback if set
	if activeSkin == nil && cfg != nil && cfg.DefaultSkin != "" {
		if cached := m.storage.GetCachedSkin(cfg.DefaultSkin); cached != nil {
			activeSkin = cached
		}
	}

	// If no custom skin to apply, keep original
	if activeSkin == nil || activeSkin.Value == "" {
		return orig
	}

	// Replace or append textures property
	var newProps []profile.Property
	for _, p := range orig.Properties {
		if p.Name != "textures" {
			newProps = append(newProps, p)
		}
	}
	newProps = append(newProps, activeSkin.ToProperty())

	return profile.GameProfile{
		ID:         orig.ID,
		Name:       orig.Name,
		Properties: newProps,
	}
}

// Storage returns the underlying Storage instance.
func (m *Manager) Storage() *storage.Storage {
	return m.storage
}
