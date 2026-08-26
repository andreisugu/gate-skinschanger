# 🎭 Gate SkinsChanger

A high-performance, resilient, zero-fluff skin management extension built natively for the [Minekube Gate](https://github.com/minekube/gate) Minecraft proxy.

---

## ✨ Features

- **🌐 Multi-Provider Resilient Skin Resolution**:
  - Automatically queries official **Mojang Sessionserver**, falling back to **Ashcon API** and **PlayerDB API** to prevent Mojang rate limits.
- **🖼️ Custom URL Skins via Mineskin**:
  - Convert any direct PNG image URL into a signed Mojang skin using `/skin url <url> [classic|slim]`.
- **⚡ In-Game Live Skin Refresh**:
  - Changing skin in-game seamlessly refreshes the player's connection to the current backend server so their new skin renders immediately without proxy disconnection.
- **🔓 Offline Mode Auto-Skin**:
  - Automatically fetches and applies genuine Java Edition skins for cracked / offline players who join with real Minecraft usernames.
- **💾 Fast & Safe Storage**:
  - Atomic JSON persistence for player skin assignments (`users_skins.json`).
  - TTL-based in-memory and on-disk texture cache (`skins_cache.json`) to minimize redundant network lookups.
- **⏱️ Anti-Spam Cooldowns**:
  - Configurable cooldown timer with permission bypass for staff and VIPs.
- **🛡️ Granular Brigadier Permissions**:
  - Every single command and subcommand has its own permission node.

---

## ⌨️ Commands

| Command | Permission | Description |
| :--- | :--- | :--- |
| `/skin set <target>` | `skinschanger.set` | Set your skin to any Minecraft player or skin name. |
| `/skin setfor <player> <target>` | `skinschanger.setfor` | Set another player's skin (Admin). |
| `/skin url <url> [classic\|slim]` | `skinschanger.url` | Set your skin from a direct image URL. |
| `/skin urlfor <player> <url> [model]` | `skinschanger.urlfor` | Set another player's skin from a URL (Admin). |
| `/skin clear` (or `/skin reset`) | `skinschanger.clear` | Reset your skin back to your default/Mojang skin. |
| `/skin clearfor <player>` | `skinschanger.clearfor` | Reset another player's skin. |
| `/skin update` | `skinschanger.update` | Force re-fetch the latest skin from Mojang session servers. |
| `/skin info [player]` | `skinschanger.info` | View active skin details, arm model, and texture URL. |
| `/skin reload` | `skinschanger.reload` | Reload `skins.toml` and skin caches. |

*Aliases*: `/skin`, `/skins`, `/skinchange`, `/skinchanger`, `/skinschanger`

---

## 🛡️ Permissions

- `skinschanger.admin`: Full access to all skinschanger features.
- `skinschanger.use`: Access to basic `/skin` help and info.
- `skinschanger.set`: Set your own skin via player name.
- `skinschanger.setfor`: Set skins for other players.
- `skinschanger.url`: Set your skin from image URLs.
- `skinschanger.urlfor`: Set image URL skins for other players.
- `skinschanger.clear`: Reset your own skin.
- `skinschanger.clearfor`: Reset other players' skins.
- `skinschanger.update`: Re-fetch your active skin.
- `skinschanger.info`: View skin diagnostics.
- `skinschanger.reload`: Reload configuration.
- `skinschanger.bypass.cooldown`: Bypass cooldown between skin changes.

---

## ⚙️ Configuration (`config/skins.toml`)

```toml
enabled = true
auto_skin_offline = true
default_skin = ""
cache_ttl = "24h"
cooldown = "10s"
mineskin_api_key = ""
refresh_on_change = true

[custom_skins]
# [custom_skins.herobrine]
# value = "eyJ0..."
# signature = "..."
# model = "classic"
```
