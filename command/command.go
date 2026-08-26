package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/andreisugu/gate-skinschanger/manager"
	"github.com/andreisugu/gate-skinschanger/model"
	"go.minekube.com/brigodier"
	c "go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/command"
	"go.minekube.com/gate/pkg/edition/java/proxy"
	"go.minekube.com/gate/pkg/util/uuid"
)

const Prefix = "§b[SkinsChanger] §r"

// SuggestionFunc adapts a function into a brigodier.SuggestionProvider.
type SuggestionFunc func(ctx *brigodier.CommandContext, builder *brigodier.SuggestionsBuilder) *brigodier.Suggestions

// Suggestions implements brigodier.SuggestionProvider.
func (f SuggestionFunc) Suggestions(ctx *brigodier.CommandContext, builder *brigodier.SuggestionsBuilder) *brigodier.Suggestions {
	return f(ctx, builder)
}

func suggestCandidates(getCandidates func() []string) SuggestionFunc {
	return func(ctx *brigodier.CommandContext, builder *brigodier.SuggestionsBuilder) *brigodier.Suggestions {
		input := builder.Input
		wordStart := builder.Start
		for wordStart < len(input) && input[wordStart] == ' ' {
			wordStart++
		}
		prefix := ""
		if wordStart < len(input) {
			prefix = strings.ToLower(input[wordStart:])
		}

		var results []*brigodier.Suggestion
		seen := make(map[string]struct{})
		for _, candidate := range getCandidates() {
			if _, exists := seen[candidate]; exists {
				continue
			}
			if strings.HasPrefix(strings.ToLower(candidate), prefix) {
				seen[candidate] = struct{}{}
				results = append(results, &brigodier.Suggestion{
					Range: brigodier.StringRange{
						Start: wordStart,
						End:   len(input),
					},
					Text: candidate,
				})
			}
		}
		return brigodier.CreateSuggestion(builder.Input, results)
	}
}

func hasPerm(src command.Source, required ...string) bool {
	if src.HasPermission("skinschanger.admin") || src.HasPermission("skinschanger.*") || src.HasPermission("*") {
		return true
	}
	for _, r := range required {
		if src.HasPermission(r) {
			return true
		}
	}
	return false
}

// RegisterCommands registers /skin, /skins, /skinchange, /skinchanger into Gate.
func RegisterCommands(p *proxy.Proxy, mgr *manager.Manager, reloadFn func() error) {
	aliases := []string{"skin", "skins", "skinchange", "skinchanger", "skinschanger"}

	onlinePlayers := func() []string {
		if p == nil {
			return nil
		}
		players := p.Players()
		res := make([]string, 0, len(players))
		for _, pl := range players {
			res = append(res, pl.Username())
		}
		return res
	}

	modelOptions := func() []string {
		return []string{"classic", "slim"}
	}

	for _, alias := range aliases {
		tree := buildCommandTree(alias, p, mgr, onlinePlayers, modelOptions, reloadFn)
		p.Command().Register(tree)
	}
}

func buildCommandTree(
	alias string,
	p *proxy.Proxy,
	mgr *manager.Manager,
	onlinePlayers func() []string,
	modelOptions func() []string,
	reloadFn func() error,
) brigodier.LiteralNodeBuilder {
	return brigodier.Literal(alias).
		Requires(command.Requires(func(c *command.RequiresContext) bool {
			return hasPerm(c.Source, "skinschanger.use", "skinschanger.set", "skinschanger.url",
				"skinschanger.clear", "skinschanger.reset", "skinschanger.update",
				"skinschanger.info", "skinschanger.setfor", "skinschanger.urlfor",
				"skinschanger.clearfor", "skinschanger.reload")
		})).
		Executes(command.Command(func(cmdCtx *command.Context) error {
			return sendHelp(cmdCtx.Source, alias)
		})).
		Then(brigodier.Literal("help").
			Executes(command.Command(func(cmdCtx *command.Context) error {
				return sendHelp(cmdCtx.Source, alias)
			})),
		).
		Then(brigodier.Literal("reload").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.reload")
			})).
			Executes(command.Command(func(cmdCtx *command.Context) error {
				if reloadFn != nil {
					if err := reloadFn(); err != nil {
						return cmdCtx.Source.SendMessage(&c.Text{Content: fmt.Sprintf("§c[SkinsChanger] Reload failed: %v", err)})
					}
				}
				return cmdCtx.Source.SendMessage(&c.Text{Content: "§a[SkinsChanger] Configuration & skin caches reloaded successfully!"})
			})),
		).
		Then(brigodier.Literal("set").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.set", "skinschanger.use")
			})).
			Then(brigodier.Argument("target", brigodier.String).
				Suggests(suggestCandidates(onlinePlayers)).
				Executes(command.Command(func(cmdCtx *command.Context) error {
					player, ok := cmdCtx.Source.(proxy.Player)
					if !ok {
						return cmdCtx.Source.SendMessage(&c.Text{Content: "§cOnly players can change their own skin. Use /skin setfor <player> <target>."})
					}
					target := cmdCtx.String("target")
					return handleSetSkin(cmdCtx.Source, player, target, mgr)
				})),
			),
		).
		Then(brigodier.Literal("setfor").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.setfor")
			})).
			Then(brigodier.Argument("player", brigodier.String).
				Suggests(suggestCandidates(onlinePlayers)).
				Then(brigodier.Argument("target", brigodier.String).
					Suggests(suggestCandidates(onlinePlayers)).
					Executes(command.Command(func(cmdCtx *command.Context) error {
						targetPlayerName := cmdCtx.String("player")
						targetSkin := cmdCtx.String("target")

						targetPlayer := p.PlayerByName(targetPlayerName)
						if targetPlayer == nil {
							return cmdCtx.Source.SendMessage(&c.Text{Content: fmt.Sprintf("§c[SkinsChanger] Player '%s' is not online.", targetPlayerName)})
						}

						return handleSetSkin(cmdCtx.Source, targetPlayer, targetSkin, mgr)
					})),
				),
			),
		).
		Then(brigodier.Literal("url").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.url")
			})).
			Then(brigodier.Argument("url", brigodier.String).
				Executes(command.Command(func(cmdCtx *command.Context) error {
					player, ok := cmdCtx.Source.(proxy.Player)
					if !ok {
						return cmdCtx.Source.SendMessage(&c.Text{Content: "§cOnly players can use /skin url."})
					}
					urlStr := cmdCtx.String("url")
					return handleSetSkinURL(cmdCtx.Source, player, urlStr, model.ModelClassic, mgr)
				})).
				Then(brigodier.Argument("model", brigodier.String).
					Suggests(suggestCandidates(modelOptions)).
					Executes(command.Command(func(cmdCtx *command.Context) error {
						player, ok := cmdCtx.Source.(proxy.Player)
						if !ok {
							return cmdCtx.Source.SendMessage(&c.Text{Content: "§cOnly players can use /skin url."})
						}
						urlStr := cmdCtx.String("url")
						mStr := strings.ToLower(cmdCtx.String("model"))
						skinModel := model.ModelClassic
						if mStr == "slim" || mStr == "alex" {
							skinModel = model.ModelSlim
						}
						return handleSetSkinURL(cmdCtx.Source, player, urlStr, skinModel, mgr)
					})),
				),
			),
		).
		Then(brigodier.Literal("urlfor").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.urlfor")
			})).
			Then(brigodier.Argument("player", brigodier.String).
				Suggests(suggestCandidates(onlinePlayers)).
				Then(brigodier.Argument("url", brigodier.String).
					Executes(command.Command(func(cmdCtx *command.Context) error {
						targetPlayerName := cmdCtx.String("player")
						urlStr := cmdCtx.String("url")
						targetPlayer := p.PlayerByName(targetPlayerName)
						if targetPlayer == nil {
							return cmdCtx.Source.SendMessage(&c.Text{Content: fmt.Sprintf("§c[SkinsChanger] Player '%s' is not online.", targetPlayerName)})
						}
						return handleSetSkinURL(cmdCtx.Source, targetPlayer, urlStr, model.ModelClassic, mgr)
					})).
					Then(brigodier.Argument("model", brigodier.String).
						Suggests(suggestCandidates(modelOptions)).
						Executes(command.Command(func(cmdCtx *command.Context) error {
							targetPlayerName := cmdCtx.String("player")
							urlStr := cmdCtx.String("url")
							mStr := strings.ToLower(cmdCtx.String("model"))
							skinModel := model.ModelClassic
							if mStr == "slim" || mStr == "alex" {
								skinModel = model.ModelSlim
							}
							targetPlayer := p.PlayerByName(targetPlayerName)
							if targetPlayer == nil {
								return cmdCtx.Source.SendMessage(&c.Text{Content: fmt.Sprintf("§c[SkinsChanger] Player '%s' is not online.", targetPlayerName)})
							}
							return handleSetSkinURL(cmdCtx.Source, targetPlayer, urlStr, skinModel, mgr)
						})),
					),
				),
			),
		).
		Then(brigodier.Literal("clear").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.clear", "skinschanger.reset")
			})).
			Executes(command.Command(func(cmdCtx *command.Context) error {
				player, ok := cmdCtx.Source.(proxy.Player)
				if !ok {
					return cmdCtx.Source.SendMessage(&c.Text{Content: "§cOnly players can clear their skin. Use /skin clearfor <player>."})
				}
				return handleClearSkin(cmdCtx.Source, player, mgr)
			})),
		).
		Then(brigodier.Literal("reset").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.clear", "skinschanger.reset")
			})).
			Executes(command.Command(func(cmdCtx *command.Context) error {
				player, ok := cmdCtx.Source.(proxy.Player)
				if !ok {
					return cmdCtx.Source.SendMessage(&c.Text{Content: "§cOnly players can reset their skin."})
				}
				return handleClearSkin(cmdCtx.Source, player, mgr)
			})),
		).
		Then(brigodier.Literal("clearfor").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.clearfor")
			})).
			Then(brigodier.Argument("player", brigodier.String).
				Suggests(suggestCandidates(onlinePlayers)).
				Executes(command.Command(func(cmdCtx *command.Context) error {
					targetPlayerName := cmdCtx.String("player")
					targetPlayer := p.PlayerByName(targetPlayerName)
					if targetPlayer == nil {
						return cmdCtx.Source.SendMessage(&c.Text{Content: fmt.Sprintf("§c[SkinsChanger] Player '%s' is not online.", targetPlayerName)})
					}
					return handleClearSkin(cmdCtx.Source, targetPlayer, mgr)
				})),
			),
		).
		Then(brigodier.Literal("update").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.update")
			})).
			Executes(command.Command(func(cmdCtx *command.Context) error {
				player, ok := cmdCtx.Source.(proxy.Player)
				if !ok {
					return cmdCtx.Source.SendMessage(&c.Text{Content: "§cOnly players can update their active skin."})
				}
				userSkin := mgr.Storage().GetUserSkin(player.ID())
				targetName := player.Username()
				if userSkin != nil && userSkin.SkinName != "" {
					targetName = userSkin.SkinName
				}
				return handleSetSkin(cmdCtx.Source, player, targetName, mgr)
			})),
		).
		Then(brigodier.Literal("info").
			Requires(command.Requires(func(c *command.RequiresContext) bool {
				return hasPerm(c.Source, "skinschanger.info")
			})).
			Executes(command.Command(func(cmdCtx *command.Context) error {
				player, ok := cmdCtx.Source.(proxy.Player)
				if !ok {
					return cmdCtx.Source.SendMessage(&c.Text{Content: "§cUse /skin info <player> from console."})
				}
				return handleInfo(cmdCtx.Source, player.Username(), player.ID(), mgr)
			})).
			Then(brigodier.Argument("player", brigodier.String).
				Suggests(suggestCandidates(onlinePlayers)).
				Executes(command.Command(func(cmdCtx *command.Context) error {
					targetName := cmdCtx.String("player")
					var targetID uuid.UUID
					if targetPlayer := p.PlayerByName(targetName); targetPlayer != nil {
						targetID = targetPlayer.ID()
					}
					return handleInfo(cmdCtx.Source, targetName, targetID, mgr)
				})),
			),
		)
}

func handleSetSkin(src command.Source, targetPlayer proxy.Player, skinTarget string, mgr *manager.Manager) error {
	// Check cooldown for non-admin executing player
	if executingPlayer, ok := src.(proxy.Player); ok {
		if !hasPerm(executingPlayer, "skinschanger.bypass.cooldown", "skinschanger.admin") {
			if onCd, remaining := mgr.CheckCooldown(executingPlayer.ID()); onCd {
				return src.SendMessage(&c.Text{
					Content: fmt.Sprintf("§c[SkinsChanger] Please wait §e%.1fs §cbefore changing skins again.", remaining.Seconds()),
				})
			}
		}
	}

	_ = src.SendMessage(&c.Text{Content: fmt.Sprintf("§7[SkinsChanger] Fetching skin §f'%s'§7...", skinTarget)})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		skin, err := mgr.ApplySkin(ctx, targetPlayer, skinTarget)
		if err != nil {
			_ = src.SendMessage(&c.Text{
				Content: fmt.Sprintf("§c[SkinsChanger] Failed to apply skin '%s': %v", skinTarget, err),
			})
			return
		}

		if executingPlayer, ok := src.(proxy.Player); ok {
			mgr.RecordCooldown(executingPlayer.ID())
		}

		modelDisplay := "classic"
		if skin.Model == model.ModelSlim {
			modelDisplay = "slim"
		}

		_ = src.SendMessage(&c.Text{
			Content: fmt.Sprintf("§a[SkinsChanger] Successfully applied skin §f'%s' §a(model: §e%s§a) to §f%s§a!",
				skinTarget, modelDisplay, targetPlayer.Username()),
		})

		if src != targetPlayer {
			_ = targetPlayer.SendMessage(&c.Text{
				Content: fmt.Sprintf("§a[SkinsChanger] Your skin was updated to §f'%s'§a!", skinTarget),
			})
		}
	}()

	return nil
}

func handleSetSkinURL(src command.Source, targetPlayer proxy.Player, imageURL string, skinModel model.SkinModel, mgr *manager.Manager) error {
	if executingPlayer, ok := src.(proxy.Player); ok {
		if !hasPerm(executingPlayer, "skinschanger.bypass.cooldown", "skinschanger.admin") {
			if onCd, remaining := mgr.CheckCooldown(executingPlayer.ID()); onCd {
				return src.SendMessage(&c.Text{
					Content: fmt.Sprintf("§c[SkinsChanger] Please wait §e%.1fs §cbefore changing skins again.", remaining.Seconds()),
				})
			}
		}
	}

	_ = src.SendMessage(&c.Text{Content: "§7[SkinsChanger] Generating signed skin from URL via Mineskin (this may take a few seconds)..."})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		skin, err := mgr.ApplySkinURL(ctx, targetPlayer, imageURL, skinModel)
		if err != nil {
			_ = src.SendMessage(&c.Text{
				Content: fmt.Sprintf("§c[SkinsChanger] Failed to generate skin from URL: %v", err),
			})
			return
		}

		if executingPlayer, ok := src.(proxy.Player); ok {
			mgr.RecordCooldown(executingPlayer.ID())
		}

		_ = src.SendMessage(&c.Text{
			Content: fmt.Sprintf("§a[SkinsChanger] Successfully applied URL skin to §f%s§a!", targetPlayer.Username()),
		})
		_ = skin
	}()

	return nil
}

func handleClearSkin(src command.Source, targetPlayer proxy.Player, mgr *manager.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.ClearSkin(ctx, targetPlayer); err != nil {
		return src.SendMessage(&c.Text{Content: fmt.Sprintf("§c[SkinsChanger] Failed to clear skin: %v", err)})
	}

	_ = src.SendMessage(&c.Text{
		Content: fmt.Sprintf("§a[SkinsChanger] Skin for §f%s §ahas been reset to default.", targetPlayer.Username()),
	})
	return nil
}

func handleInfo(src command.Source, username string, playerID uuid.UUID, mgr *manager.Manager) error {
	var userSkin *model.UserSkin
	if playerID != [16]byte{} {
		userSkin = mgr.Storage().GetUserSkin(playerID)
	}
	if userSkin == nil {
		userSkin = mgr.Storage().GetUserSkinByUsername(username)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("§b[SkinsChanger Info] §7Player: §f%s\n", username))
	if userSkin == nil || userSkin.Skin == nil {
		b.WriteString("  §7• Custom Skin: §eNone (Default/Native)\n")
	} else {
		b.WriteString(fmt.Sprintf("  §7• Skin Target: §f%s\n", userSkin.SkinName))
		modelType := "classic (4px)"
		if userSkin.Skin.Model == model.ModelSlim {
			modelType = "slim (3px Alex)"
		}
		b.WriteString(fmt.Sprintf("  §7• Arm Model: §f%s\n", modelType))
		if userSkin.Skin.SkinURL != "" {
			b.WriteString(fmt.Sprintf("  §7• Texture URL: §f%s\n", userSkin.Skin.SkinURL))
		}
		b.WriteString(fmt.Sprintf("  §7• Last Updated: §f%s\n", userSkin.UpdatedAt.Format("2006-01-02 15:04:05 UTC")))
	}

	return src.SendMessage(&c.Text{Content: strings.TrimRight(b.String(), "\n")})
}

func sendHelp(src command.Source, alias string) error {
	msg := fmt.Sprintf("§b=== SkinsChanger Commands ===\n"+
		"§f/%s set <player/skin> §7- Set your skin to a Minecraft user's skin\n"+
		"§f/%s setfor <player> <skin> §7- Set skin for another player\n"+
		"§f/%s url <image_url> [classic|slim] §7- Set skin from image URL\n"+
		"§f/%s urlfor <player> <url> [model] §7- Set URL skin for another player\n"+
		"§f/%s clear §7- Reset your skin to default\n"+
		"§f/%s update §7- Re-fetch and update current skin\n"+
		"§f/%s info [player] §7- View active skin information\n"+
		"§f/%s reload §7- Reload configuration and skin cache",
		alias, alias, alias, alias, alias, alias, alias, alias)

	return src.SendMessage(&c.Text{Content: msg})
}
