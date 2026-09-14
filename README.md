# TelegramDirect

Self-hosted Telegram suggestion bot for anonymous and non-anonymous submissions.

TelegramDirect is a lightweight monolithic Go application using SQLite. It is designed for small Telegram channels and communities and requires no web server or external database.

## Features

* Anonymous and named suggestions
* Private user interaction
* Dedicated admin group for moderation
* Multiple administrators
* Approve/reject buttons
* Automatic publishing to a Telegram channel
* Admin action logging
* SQLite storage
* Fully customizable bot texts, buttons and message templates
* Self-hosted configuration
* Automatic binary releases via GitHub Actions
* `systemd` or `PM2` process management
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
 ├── Approve ──────► Telegram Channel
 └── Reject
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
```

User-facing texts and bot customization are stored separately in:

```text
/opt/suggestion-bot/config.json
```

You can customize the bot's messages, buttons, labels, error messages, moderation messages, publication format and other text without modifying the source code.

## Management

The installer also installs the `telegramdirect` management command.

### Edit configuration

```bash
sudo telegramdirect config
```

Opens `config.json` in an editor and restarts the bot after saving.

### Update

```bash
sudo telegramdirect update
```

Downloads the latest GitHub Release, verifies its SHA-256 checksum, replaces the binary and restarts the bot.

Configuration, database and `.env` are preserved during updates.

### Uninstall

```bash
sudo telegramdirect uninstall
```

Stops the bot and removes the installed application. The database and persistent data are deleted only after confirmation.

## Process Management

### systemd

```bash
systemctl status suggestion-bot
journalctl -u suggestion-bot -f
```

### PM2

```bash
runuser -u suggestion-bot -- env PM2_HOME=/var/lib/suggestion-bot/.pm2 pm2 status
runuser -u suggestion-bot -- env PM2_HOME=/var/lib/suggestion-bot/.pm2 pm2 logs suggestion-bot
```

## Releases

GitHub Actions automatically builds Linux binaries when a version tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Releases include binaries for:

```text
linux-amd64
linux-arm64
```

The VPS installer and updater use the latest published release.

## Requirements

* Debian-based Linux VPS
* Telegram Bot Token
* Admin Telegram group
* Telegram channel for published suggestions