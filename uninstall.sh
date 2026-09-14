#!/usr/bin/env bash
set -euo pipefail

APP_NAME="suggestion-bot"
APP_DIR="/opt/$APP_NAME"
DATA_DIR="/var/lib/$APP_NAME"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
APP_USER="$APP_NAME"

if [[ "${EUID}" -ne 0 ]]; then
    echo "Run this script as root or with sudo."
    exit 1
fi

echo "========================================"
echo " TelegramDirect Uninstaller"
echo "========================================"
echo

echo "This will stop TelegramDirect and remove its installed files."
echo "The SQLite database will NOT be removed unless you explicitly confirm."
echo

read -rp "Continue? [y/N]: " CONFIRM

if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

echo

# Stop and remove systemd service
if [[ -f "$SERVICE_FILE" ]] || \
   systemctl list-unit-files --full --no-legend 2>/dev/null \
   | awk '{print $1}' \
   | grep -qx "${APP_NAME}.service"; then

    echo "Stopping systemd service..."

    systemctl disable --now "$APP_NAME" 2>/dev/null || true

    rm -f "$SERVICE_FILE"

    systemctl daemon-reload
fi

# Stop and remove PM2 process
PM2_HOME="$DATA_DIR/.pm2"

if command -v pm2 >/dev/null 2>&1 && \
   id -u "$APP_USER" >/dev/null 2>&1 && \
   [[ -d "$PM2_HOME" ]]; then

    echo "Stopping PM2 process..."

    runuser -u "$APP_USER" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 delete "$APP_NAME" >/dev/null 2>&1 || true

    runuser -u "$APP_USER" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 save >/dev/null 2>&1 || true
fi

echo

# Ask separately about persistent data
REMOVE_DATA="n"

if [[ -d "$DATA_DIR" ]]; then
    echo "Persistent data found at:"
    echo "  $DATA_DIR"
    echo

    read -rp \
        "Delete SQLite database and all bot data? [y/N]: " \
        REMOVE_DATA
fi

# Remove application files
if [[ -d "$APP_DIR" ]]; then
    echo "Removing application files..."
    rm -rf "$APP_DIR"
fi

# Remove persistent data only when explicitly confirmed
if [[ "$REMOVE_DATA" =~ ^[Yy]$ ]]; then
    echo "Removing bot data..."
    rm -rf "$DATA_DIR"
else
    echo "Keeping bot data at: $DATA_DIR"
fi

# Remove dedicated system user only when all data is removed
if [[ "$REMOVE_DATA" =~ ^[Yy]$ ]] && \
   id -u "$APP_USER" >/dev/null 2>&1; then

    echo "Removing system user..."
    userdel "$APP_USER" 2>/dev/null || true
fi

echo
echo "Uninstallation complete."

if [[ ! "$REMOVE_DATA" =~ ^[Yy]$ ]]; then
    echo
    echo "SQLite data was preserved."
    echo "Remove it manually with:"
    echo "  sudo rm -rf $DATA_DIR"
fi
