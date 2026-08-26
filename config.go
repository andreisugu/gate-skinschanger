package skinschanger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the TOML configuration file for gate-skinschanger.
type Config struct {
	Enabled          bool                   `toml:"enabled"`
	AutoSkinOffline  bool                   `toml:"auto_skin_offline"`
	DefaultSkin      string                 `toml:"default_skin"`
	CacheTTLStr      string                 `toml:"cache_ttl"`
	CooldownStr      string                 `toml:"cooldown"`
	MineskinAPIKey   string                 `toml:"mineskin_api_key"`
	RefreshOnChange  bool                   `toml:"refresh_on_change"`
	CustomSkins      map[string]CustomSkin  `toml:"custom_skins"`

	ParsedCacheTTL time.Duration `toml:"-"`
	ParsedCooldown time.Duration `toml:"-"`
}

// CustomSkin represents a pre-signed alias skin.
type CustomSkin struct {
	Value     string `toml:"value"`
	Signature string `toml:"signature"`
	Model     string `toml:"model"`
}

const defaultSkinsConfigTemplate = `# =============================================================================
#                        Gate SkinsChanger Configuration
# =============================================================================

# Enable or disable the skinschanger extension.
enabled = true

# Automatically fetch & apply real Mojang skins for offline-mode / cracked players
# matching genuine Java Edition usernames on join.
auto_skin_offline = true

# Default fallback skin if player has no custom skin set (leave empty for vanilla default).
default_skin = ""

# How long to cache resolved Mojang/Mineskin skin textures in memory & disk.
cache_ttl = "24h"

# Cooldown between in-game skin changes to prevent rate-limit spam (e.g. "10s", "30s", "1m").
# Players with permission "skinschanger.bypass.cooldown" or "skinschanger.admin" bypass this.
cooldown = "10s"

# Optional Mineskin API key for higher rate limits with /skin url.
# (Free API works without a key, but an API key improves speed).
mineskin_api_key = ""

# Automatically refresh player on current server when changing skin in-game.
refresh_on_change = true

# Pre-defined custom skin aliases. You can define custom value/signatures here.
[custom_skins]
# [custom_skins.herobrine]
# value = "eyJ0..."
# signature = "..."
# model = "classic"
`

// ConfigStore manages loading and thread-safe access to Config.
type ConfigStore struct {
	filePath string
	mu       sync.RWMutex
	cfg      *Config
}

// NewConfigStore initializes a ConfigStore for the given config directory.
func NewConfigStore(configDir string) *ConfigStore {
	return &ConfigStore{
		filePath: filepath.Join(configDir, "skins.toml"),
	}
}

// Load loads or creates skins.toml with defaults.
func (s *ConfigStore) Load() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		_ = os.MkdirAll(filepath.Dir(s.filePath), 0o755)
		_ = os.WriteFile(s.filePath, []byte(defaultSkinsConfigTemplate), 0o644)
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", s.filePath, err)
	}

	cfg := &Config{
		Enabled:         true,
		AutoSkinOffline: true,
		CacheTTLStr:     "24h",
		CooldownStr:     "10s",
		RefreshOnChange: true,
		CustomSkins:     make(map[string]CustomSkin),
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse skins.toml: %w", err)
	}

	// Parse durations
	if ttl, err := time.ParseDuration(cfg.CacheTTLStr); err == nil && ttl > 0 {
		cfg.ParsedCacheTTL = ttl
	} else {
		cfg.ParsedCacheTTL = 24 * time.Hour
	}

	if cd, err := time.ParseDuration(cfg.CooldownStr); err == nil && cd >= 0 {
		cfg.ParsedCooldown = cd
	} else {
		cfg.ParsedCooldown = 10 * time.Second
	}

	s.cfg = cfg
	return cfg, nil
}

// Get returns the current active configuration snapshot.
func (s *ConfigStore) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}
