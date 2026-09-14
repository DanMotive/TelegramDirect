# TelegramDirect

Self-hosted Telegram suggestion bot for anonymous and named submissions.

TelegramDirect is a lightweight monolithic Go application using SQLite. It is designed for small Telegram channels and communities and requires no web server or external database.

## Features

* Anonymous and named suggestions
* Private user interaction
* Text, photos, videos, files, audio, voice messages and video notes
* Media albums
* GIFs and stickers can be disabled
* Dedicated admin group for moderation
* Multiple administrators
* Approve/reject buttons
* Admin replies directly to suggestions
* Anonymous IDs for identifying anonymous users without exposing their Telegram account
* Automatic publishing to a Telegram channel
* Admin action logging
* SQLite storage
* Fully customizable texts, buttons and message templates
* Self-hosted configuration
* Automatic binary releases via GitHub Actions
* `systemd` or shared `PM2` process management
* No incoming HTTP port required — uses Telegram long polling

## Architecture

```text
User
 │
 │ Private message
 ▼
Telegram Bot
 │
 ├──► SQLite
 │
 ▼
Admin Group
 │
 ├── Approve ──────► Telegram Channel
 ├── Reject
 └── Reply ────────► User
```

## Installation

Clone the repository and run the installer:

```bash
sudo git clone https://github.com/DanMotive/TelegramDirect.git /opt/TelegramDirect
cd /opt/TelegramDirect
sudo chmod +x install.sh
sudo ./install.sh
```

The installer asks for:

* Telegram Bot Token
* Admin group ID
* Target channel ID
* Admin Telegram IDs
* Process manager (`systemd` or `PM2`)

The installer automatically detects the VPS architecture and downloads the latest prebuilt binary from GitHub Releases.

Go does **not** need to be installed on the VPS.

Telegram IDs can be obtained using Telegram Desktop Developer Mode.

## Configuration

Technical settings are stored in:

```text
/opt/suggestion-bot/.env
```

Example:

```env
BOT_TOKEN=your_bot_token
ADMIN_CHAT_ID=-1001234567890
CHANNEL_ID=-1009876543210
ADMIN_IDS=123456789,987654321
DB_PATH=/var/lib/suggestion-bot/data.db
CONFIG_PATH=/opt/suggestion-bot/config.json
```

User-facing texts and customization are stored in:

```text
/opt/suggestion-bot/config.json
```

You can customize messages, buttons, labels, moderation text, publication format, reply messages, error messages and other text without modifying the source code.

Anonymous users are assigned a persistent identifier such as:

```text
ANON-X7K4P2
```

This identifier is shown to administrators instead of the user's Telegram account.

## Management

The installer provides the `telegramdirect` management command.

### Edit configuration

```bash
sudo telegramdirect config
```

Edits `config.json` and can restart the bot after saving.

### Update

```bash
sudo telegramdirect update
```

Downloads the latest GitHub Release, verifies its SHA-256 checksum, replaces the binary and restarts the bot.

The database, `.env` and `config.json` are preserved.

### Uninstall

```bash
sudo telegramdirect uninstall
```

Stops TelegramDirect and removes its application files.

Persistent data is deleted only after confirmation.

## Process Management

### systemd

```bash
systemctl status suggestion-bot
journalctl -u suggestion-bot -f
```

### PM2

TelegramDirect can use the existing system-wide PM2 installation together with other applications:

```bash
pm2 status
pm2 logs suggestion-bot
```

TelegramDirect removes or updates only its own PM2 process and does not affect other applications.

## Releases

GitHub Actions automatically builds Linux binaries when a version tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Releases include:

```text
linux-amd64
linux-arm64
```

The installer and updater use the latest published release.

## Requirements

* Debian-based Linux VPS
* Telegram Bot Token
* Telegram admin group
* Telegram channel for published suggestions

No Go installation, incoming HTTP port, web server or external database is required.