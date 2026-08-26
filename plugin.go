package skinschanger

import (
	"context"

	"github.com/andreisugu/gate-skinschanger/command"
	"github.com/andreisugu/gate-skinschanger/fetcher"
	"github.com/andreisugu/gate-skinschanger/manager"
	"github.com/andreisugu/gate-skinschanger/storage"
	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"go.minekube.com/common/minecraft/key"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/edition/java/proxy/message"
)

// Plugin is the SkinsRestorer-like skin management extension for Minekube Gate.
var Plugin = proxy.Plugin{
	Name: "SkinsChanger",
	Init: func(ctx context.Context, p *proxy.Proxy) error {
		log := logr.FromContextOrDiscard(ctx)

		// 1. Config store
		cfgStore := NewConfigStore("config")
		cfg, err := cfgStore.Load()
		if err != nil {
			log.Error(err, "Failed to load skins.toml")
			return err
		}

		// 2. Storage
		store := storage.NewStorage("config", cfg.ParsedCacheTTL)
		if err := store.Load(); err != nil {
			log.Error(err, "Failed to initialize skins storage")
			return err
		}

		// 3. Multi-Provider Fetcher
		skinFetcher := fetcher.NewMultiProviderFetcher(cfg.MineskinAPIKey, 0)

		// 4. Manager
		cfgSnapshotProvider := func() *manager.ConfigSnapshot {
			current := cfgStore.Get()
			if current == nil {
				return nil
			}
			customMap := make(map[string]struct {
				Value     string
				Signature string
				Model     string
			}, len(current.CustomSkins))
			for k, v := range current.CustomSkins {
				customMap[k] = struct {
					Value     string
					Signature string
					Model     string
				}{
					Value:     v.Value,
					Signature: v.Signature,
					Model:     v.Model,
				}
			}

			return &manager.ConfigSnapshot{
				Enabled:         current.Enabled,
				AutoSkinOffline: current.AutoSkinOffline,
				DefaultSkin:     current.DefaultSkin,
				CacheTTL:        current.ParsedCacheTTL,
				Cooldown:        current.ParsedCooldown,
				RefreshOnChange: current.RefreshOnChange,
				CustomSkins:     customMap,
			}
		}

		mgr := manager.NewManager(p, skinFetcher, store, cfgSnapshotProvider)

		// 5. Subscribe to GameProfileRequestEvent (injects skin into player connection on login)
		event.Subscribe(p.Event(), 0, func(e *proxy.GameProfileRequestEvent) {
			newProf := mgr.ProcessProfileRequest(context.Background(), e.Original(), e.OnlineMode())
			e.SetGameProfile(newProf)
		})

		// 6. Subscribe to ServerPostConnectEvent (re-injects skin packets after backend server handshakes)
		event.Subscribe(p.Event(), 0, func(e *proxy.ServerPostConnectEvent) {
			mgr.OnServerPostConnect(e.Player())
		})

		// 7. Register In-Game & Console Commands
		reloadFn := func() error {
			newCfg, err := cfgStore.Load()
			if err != nil {
				return err
			}
			_ = store.SaveCache()
			_ = store.Load()
			log.Info("SkinsChanger configuration reloaded", "enabled", newCfg.Enabled)
			return nil
		}
		command.RegisterCommands(p, mgr, reloadFn)

		// 8. Register SkinsRestorer compatibility channel
		if p.ChannelRegistrar() != nil {
			if k, err := key.Parse("sr:messagechannel"); err == nil {
				p.ChannelRegistrar().Register(&message.MinecraftChannelIdentifier{Key: k})
			}
			if k, err := key.Parse("skinsrestorer:main"); err == nil {
				p.ChannelRegistrar().Register(&message.MinecraftChannelIdentifier{Key: k})
			}
		}

		log.Info("SkinsChanger loaded successfully (Mojang, Ashcon, PlayerDB, Mineskin active)")
		return nil
	},
}
