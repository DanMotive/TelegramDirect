#!/usr/bin/env bash
set -euo pipefail

APP_NAME="suggestion-bot"

APP_DIR="/opt/$APP_NAME"
DATA_DIR="/var/lib/$APP_NAME"

SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
PM2_CONFIG="$APP_DIR/ecosystem.config.js"

CLI_PATH="/usr/local/bin/telegramdirect"

if [[ "${EUID}" -ne 0 ]]; then
    echo "Run this script as root or with sudo."
    exit 1
fi

echo "========================================"
echo " TelegramDirect Uninstaller"
echo "========================================"
echo

echo "This will remove TelegramDirect."
echo "Other PM2 applications will NOT be removed."
echo

read -rp "Continue? [y/N]: " CONFIRM

if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

echo
echo "Stopping TelegramDirect..."

# ----------------------------------------
# systemd
# ----------------------------------------

if [[ -f "$SERVICE_FILE" ]]; then
    echo "Found systemd service."

    systemctl disable --now "$APP_NAME" 2>/dev/null || true

    rm -f "$SERVICE_FILE"

    systemctl daemon-reload

    echo "systemd service removed."
fi

# ----------------------------------------
# Shared PM2
# ----------------------------------------

if command -v pm2 >/dev/null 2>&1; then
    echo "Checking shared PM2..."

    if pm2 jlist 2>/dev/null |
        grep -q "\"name\":\"$APP_NAME\""; then

        echo "Removing $APP_NAME from PM2..."

        pm2 delete "$APP_NAME"

        # Save the remaining PM2 applications.
        pm2 save

        echo "PM2 process removed."
    else
        echo "TelegramDirect is not registered in PM2."
    fi
fi

# ----------------------------------------
# Application files
# ----------------------------------------

echo
echo "Removing application files..."

if [[ -d "$APP_DIR" ]]; then
    rm -rf "$APP_DIR"
    echo "Removed: $APP_DIR"
else
    echo "Application directory not found."
fi

# ----------------------------------------
# Persistent data
# ----------------------------------------

echo

if [[ -d "$DATA_DIR" ]]; then
    echo "Persistent data found:"
    echo "  $DATA_DIR"
    echo

    read -rp \
        "Delete database and all persistent data? [y/N]: " \
        REMOVE_DATA

    if [[ "$REMOVE_DATA" =~ ^[Yy]$ ]]; then

        rm -rf "$DATA_DIR"

        echo "Persistent data removed."

    else

        echo "Persistent data preserved:"
        echo "  $DATA_DIR"
    fi
else
    echo "No persistent data directory found."
fi

# ----------------------------------------
# Management command
# ----------------------------------------

echo

if [[ -f "$CLI_PATH" ]]; then
    rm -f "$CLI_PATH"
    echo "Removed: $CLI_PATH"
fi

echo
echo "========================================"
echo " TelegramDirect has been uninstalled."
echo "========================================"
echo