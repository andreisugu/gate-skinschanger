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
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/legacytablist"
	"go.minekube.com/gate/pkg/edition/java/proto/packet/tablist/playerinfo"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
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

	// In-place profile property injection & broadcast tablist packets to all viewers
	m.BroadcastSkinUpdate(p, skin)

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

	// In-place injection & broadcast
	m.BroadcastSkinUpdate(p, skin)

	return skin, nil
}

// ClearSkin resets a player's skin back to their default/Mojang skin.
func (m *Manager) ClearSkin(ctx context.Context, p proxy.Player) error {
	_, _ = m.storage.RemoveUserSkin(p.ID())

	var skin *model.SkinData
	// If online-mode player, restore genuine Mojang skin
	if p.OnlineMode() {
		if nativeSkin, err := m.ResolveSkin(ctx, p.Username()); err == nil && nativeSkin != nil {
			skin = nativeSkin
		}
	} else {
		// Offline-mode: check if auto_skin_offline restores their name's genuine skin
		cfg := m.configProvider()
		if cfg != nil && cfg.AutoSkinOffline {
			if nativeSkin, err := m.ResolveSkin(ctx, p.Username()); err == nil && nativeSkin != nil {
				skin = nativeSkin
			}
		}
	}

	if skin != nil {
		m.BroadcastSkinUpdate(p, skin)
	} else {
		InjectPlayerProfileProperty(p, profile.Property{})
		m.BroadcastSkinUpdate(p, &model.SkinData{Name: p.Username()})
	}
	return nil
}

// GetActiveSkin retrieves the assigned SkinData for a player from storage or defaults.
func (m *Manager) GetActiveSkin(playerID uuid.UUID, username string, onlineMode bool) *model.SkinData {
	cfg := m.configProvider()
	if cfg != nil && !cfg.Enabled {
		return nil
	}

	// 1. Explicit mapping by UUID
	if u := m.storage.GetUserSkin(playerID); u != nil && u.Skin != nil {
		return u.Skin
	}
	// 2. Explicit mapping by username
	if u := m.storage.GetUserSkinByUsername(username); u != nil && u.Skin != nil {
		return u.Skin
	}
	// 3. Offline-mode auto skin from cache
	if !onlineMode && cfg != nil && cfg.AutoSkinOffline {
		if cached := m.storage.GetCachedSkin(username); cached != nil {
			return cached
		}
	}
	// 4. Default skin fallback
	if cfg != nil && cfg.DefaultSkin != "" {
		if cached := m.storage.GetCachedSkin(cfg.DefaultSkin); cached != nil {
			return cached
		}
	}
	return nil
}

// BroadcastSkinUpdate updates player profile in-place and safely broadcasts TabList/PlayerInfo packets to online viewers.
func (m *Manager) BroadcastSkinUpdate(p proxy.Player, skin *model.SkinData) {
	if p == nil {
		return
	}

	var prop profile.Property
	if skin != nil && skin.Value != "" {
		prop = skin.ToProperty()
	}
	InjectPlayerProfileProperty(p, prop)

	if m.proxy == nil {
		return
	}

	go func() {
		defer func() {
			_ = recover()
		}()

		players := m.proxy.Players()

		// 1. Send Remove packet to OTHER active viewers only.
		// (NEVER send Remove to the player themselves, as Minecraft client will nullify localPlayerInfo and revert to Steve/Alex).
		for _, viewer := range players {
			if !viewer.Active() || viewer.ID() == p.ID() {
				continue
			}
			if viewer.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
				_ = viewer.WritePacket(&playerinfo.Remove{
					PlayersToRemove: []uuid.UUID{p.ID()},
				})
			} else {
				_ = viewer.WritePacket(&legacytablist.PlayerListItem{
					Action: legacytablist.RemovePlayerListItemAction,
					Items: []legacytablist.PlayerListItemEntry{
						{ID: p.ID()},
					},
				})
			}
		}

		// 2. Short sleep to allow Minecraft client skin cache invalidation
		time.Sleep(60 * time.Millisecond)

		// 3. Send Add/Upsert packet with updated GameProfile containing the new skin
		var props []profile.Property
		if prop.Value != "" {
			props = []profile.Property{prop}
		}

		newProfile := profile.GameProfile{
			ID:         p.ID(),
			Name:       p.Username(),
			Properties: props,
		}

		latency := 0
		if p.Ping() > 0 {
			latency = int(p.Ping().Milliseconds())
		}

		for _, viewer := range players {
			if !viewer.Active() {
				continue
			}
			if viewer.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
				_ = viewer.WritePacket(&playerinfo.Upsert{
					ActionSet: []playerinfo.UpsertAction{
						playerinfo.AddPlayerAction,
						playerinfo.UpdateGameModeAction,
						playerinfo.UpdateListedAction,
						playerinfo.UpdateLatencyAction,
					},
					Entries: []*playerinfo.Entry{
						{
							ProfileID: p.ID(),
							Profile:   newProfile,
							GameMode:  0,
							Listed:    true,
							Latency:   latency,
						},
					},
				})
			} else {
				_ = viewer.WritePacket(&legacytablist.PlayerListItem{
					Action: legacytablist.AddPlayerListItemAction,
					Items: []legacytablist.PlayerListItemEntry{
						{
							ID:         p.ID(),
							Name:       p.Username(),
							Properties: props,
							GameMode:   0,
							Latency:    latency,
						},
					},
				})
			}
		}
	}()
}

// sendSkinToViewer sends a target player's skin update directly to a specific viewer.
func (m *Manager) sendSkinToViewer(viewer proxy.Player, target proxy.Player, skin *model.SkinData) {
	if viewer == nil || !viewer.Active() || target == nil || !target.Active() || skin == nil || skin.Value == "" {
		return
	}

	prop := skin.ToProperty()
	targetProfile := profile.GameProfile{
		ID:         target.ID(),
		Name:       target.Username(),
		Properties: []profile.Property{prop},
	}

	latency := 0
	if target.Ping() > 0 {
		latency = int(target.Ping().Milliseconds())
	}

	if viewer.Protocol().GreaterEqual(version.Minecraft_1_19_3) {
		_ = viewer.WritePacket(&playerinfo.Remove{
			PlayersToRemove: []uuid.UUID{target.ID()},
		})
		_ = viewer.WritePacket(&playerinfo.Upsert{
			ActionSet: []playerinfo.UpsertAction{
				playerinfo.AddPlayerAction,
				playerinfo.UpdateGameModeAction,
				playerinfo.UpdateListedAction,
				playerinfo.UpdateLatencyAction,
			},
			Entries: []*playerinfo.Entry{
				{
					ProfileID: target.ID(),
					Profile:   targetProfile,
					GameMode:  0,
					Listed:    true,
					Latency:   latency,
				},
			},
		})
	} else {
		_ = viewer.WritePacket(&legacytablist.PlayerListItem{
			Action: legacytablist.RemovePlayerListItemAction,
			Items: []legacytablist.PlayerListItemEntry{
				{ID: target.ID()},
			},
		})
		_ = viewer.WritePacket(&legacytablist.PlayerListItem{
			Action: legacytablist.AddPlayerListItemAction,
			Items: []legacytablist.PlayerListItemEntry{
				{
					ID:         target.ID(),
					Name:       target.Username(),
					Properties: []profile.Property{prop},
					GameMode:   0,
					Latency:    latency,
				},
			},
		})
	}
}

// OnServerPostConnect handles re-injecting custom skins after a player completes a backend server transition.
func (m *Manager) OnServerPostConnect(p proxy.Player) {
	if p == nil || !p.Active() || m.proxy == nil {
		return
	}

	// 1. Broadcast p's skin after a short delay so the backend server's initial world packets are overridden
	if skin := m.GetActiveSkin(p.ID(), p.Username(), p.OnlineMode()); skin != nil && skin.Value != "" {
		go func() {
			defer func() { _ = recover() }()
			time.Sleep(100 * time.Millisecond)
			if !p.Active() {
				return
			}
			m.BroadcastSkinUpdate(p, skin)
		}()
	}

	// 2. Push skins of other online players to the newly connected viewer
	m.SendAllOnlineSkinsToViewer(p)
}

// SendAllOnlineSkinsToViewer pushes all online players' skins to the specified viewer.
func (m *Manager) SendAllOnlineSkinsToViewer(viewer proxy.Player) {
	if viewer == nil || !viewer.Active() || m.proxy == nil {
		return
	}

	go func() {
		defer func() { _ = recover() }()
		time.Sleep(150 * time.Millisecond)
		if !viewer.Active() {
			return
		}

		for _, target := range m.proxy.Players() {
			if target.ID() == viewer.ID() || !target.Active() {
				continue
			}
			if targetSkin := m.GetActiveSkin(target.ID(), target.Username(), target.OnlineMode()); targetSkin != nil && targetSkin.Value != "" {
				m.sendSkinToViewer(viewer, target, targetSkin)
			}
		}
	}()
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

	// 3. Auto-skin: if user has no assigned skin, and profile lacks textures or auto_skin_offline is on
	hasTextures := false
	for _, prop := range orig.Properties {
		if prop.Name == "textures" && prop.Value != "" {
			hasTextures = true
			break
		}
	}

	if activeSkin == nil && (!hasTextures || (!onlineMode && cfg != nil && cfg.AutoSkinOffline)) {
		if cached := m.storage.GetCachedSkin(orig.Name); cached != nil {
			activeSkin = cached
		} else {
			// Synchronous fetch with short timeout during login
			fetchCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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
