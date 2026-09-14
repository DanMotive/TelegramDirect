#!/usr/bin/env bash
set -euo pipefail

REPO="DanMotive/TelegramDirect"
APP_NAME="suggestion-bot"
APP_DIR="/opt/$APP_NAME"
DATA_DIR="/var/lib/$APP_NAME"
BINARY="$APP_DIR/$APP_NAME"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
PM2_CONFIG="$APP_DIR/ecosystem.config.js"
ENV_FILE="$APP_DIR/.env"

case "$(uname -m)" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $(uname -m)"
        exit 1
        ;;
esac

if [[ "${EUID}" -ne 0 ]]; then
    echo "Run this script as root or with sudo."
    exit 1
fi
if [[ -f ./telegramdirect ]]; then
    install -m 755 ./telegramdirect /usr/local/bin/telegramdirect
else
    echo "Warning: telegramdirect CLI script not found."
fi
echo "========================================"
echo " TelegramDirect Bot Installer"
echo "========================================"
echo

echo "Detected architecture: $ARCH"
echo

echo "Telegram IDs can be viewed in Telegram Desktop by enabling Developer Mode."
echo "Typical channel/group IDs look like -1001234567890."
echo

read -rsp "Telegram Bot Token: " BOT_TOKEN
printf '\n'

read -rp "Admin group chat ID: " ADMIN_CHAT_ID
read -rp "Target channel ID: " CHANNEL_ID
read -rp "Admin Telegram IDs (comma-separated): " ADMIN_IDS

echo
echo "Process manager:"
echo "  1) systemd (recommended)"
echo "  2) PM2"
read -rp "Choose [1-2]: " PROCESS_MANAGER

case "$PROCESS_MANAGER" in
    1) PROCESS_MANAGER="systemd" ;;
    2) PROCESS_MANAGER="pm2" ;;
    *)
        echo "Invalid choice."
        exit 1
        ;;
esac

if [[ -z "$BOT_TOKEN" || -z "$ADMIN_CHAT_ID" || -z "$CHANNEL_ID" || -z "$ADMIN_IDS" ]]; then
    echo "All fields are required."
    exit 1
fi

if ! [[ "$ADMIN_CHAT_ID" =~ ^-?[0-9]+$ ]]; then
    echo "Invalid admin group chat ID."
    exit 1
fi

if ! [[ "$CHANNEL_ID" =~ ^-?[0-9]+$ ]]; then
    echo "Invalid channel ID."
    exit 1
fi

IFS=',' read -ra IDS <<< "$ADMIN_IDS"

for id in "${IDS[@]}"; do
    id="${id//[[:space:]]/}"

    if ! [[ "$id" =~ ^[0-9]+$ ]]; then
        echo "Invalid admin Telegram ID: $id"
        exit 1
    fi
done

echo
echo "Checking latest TelegramDirect release..."
install -m 755 telegramdirect /usr/local/bin/telegramdirect
apt-get update
apt-get install -y ca-certificates curl sqlite3

LATEST_TAG="$(
    curl -fsSL \
        -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' |
        head -n 1
)"

if [[ -z "$LATEST_TAG" ]]; then
    echo "Failed to determine the latest release."
    echo "Make sure the repository has at least one GitHub Release."
    exit 1
fi

echo "Latest release: $LATEST_TAG"

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/suggestion-bot-linux-$ARCH"

echo "Downloading:"
echo "$DOWNLOAD_URL"
echo

if ! curl -fL "$DOWNLOAD_URL" -o /tmp/"$APP_NAME"; then
    echo
    echo "Failed to download the TelegramDirect binary."
    echo "Make sure release $LATEST_TAG contains:"
    echo "  suggestion-bot-linux-$ARCH"
    exit 1
fi

echo "Binary downloaded successfully."

if [[ "$PROCESS_MANAGER" == "pm2" ]] && ! command -v pm2 >/dev/null 2>&1; then
    echo
    echo "PM2 is not installed. Installing Node.js and PM2..."

    if ! command -v npm >/dev/null 2>&1; then
        apt-get install -y nodejs npm
    fi

    npm install -g pm2
fi

if ! id -u "$APP_NAME" >/dev/null 2>&1; then
    echo "Creating system user: $APP_NAME"

    useradd \
        --system \
        --home "$APP_DIR" \
        --shell /usr/sbin/nologin \
        "$APP_NAME"
fi

mkdir -p "$APP_DIR" "$DATA_DIR"

# Install binary
install -m 755 /tmp/"$APP_NAME" "$BINARY"
rm -f /tmp/"$APP_NAME"

chown root:root "$BINARY"

# Environment file
cat > "$ENV_FILE" <<ENV
BOT_TOKEN=$BOT_TOKEN
ADMIN_CHAT_ID=$ADMIN_CHAT_ID
CHANNEL_ID=$CHANNEL_ID
ADMIN_IDS=$ADMIN_IDS
DB_PATH=$DATA_DIR/data.db
ENV

chown root:"$APP_NAME" "$ENV_FILE"
chmod 640 "$ENV_FILE"

# Database directory
chown "$APP_NAME":"$APP_NAME" "$DATA_DIR"
chmod 750 "$DATA_DIR"

echo
echo "========================================"
echo " Installation"
echo "========================================"
echo

read -rp "Install/update $APP_NAME with these settings? [y/N]: " CONFIRM

if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

if [[ "$PROCESS_MANAGER" == "systemd" ]]; then

    echo "Configuring systemd..."

    # Stop old PM2 instance if one exists.
    if command -v pm2 >/dev/null 2>&1 && \
       [[ -d "$DATA_DIR/.pm2" ]] && \
       id -u "$APP_NAME" >/dev/null 2>&1; then

        PM2_HOME="$DATA_DIR/.pm2"

        runuser -u "$APP_NAME" -- \
            env PM2_HOME="$PM2_HOME" \
            pm2 delete "$APP_NAME" >/dev/null 2>&1 || true
    fi

    cat > "$SERVICE_FILE" <<SERVICE
[Unit]
Description=TelegramDirect Bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$APP_NAME
Group=$APP_NAME
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$BINARY
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR

[Install]
WantedBy=multi-user.target
SERVICE

    systemctl daemon-reload
    systemctl enable "$APP_NAME"

    # Restart if service already existed, otherwise start it.
    systemctl restart "$APP_NAME"

    MANAGER_INFO="systemd"

else

    echo "Configuring PM2..."

    # Remove old systemd service if present.
    if [[ -f "$SERVICE_FILE" ]]; then
        systemctl disable --now "$APP_NAME" 2>/dev/null || true
        rm -f "$SERVICE_FILE"
        systemctl daemon-reload
    fi

    PM2_HOME="$DATA_DIR/.pm2"

    mkdir -p "$PM2_HOME"
    chown -R "$APP_NAME":"$APP_NAME" "$PM2_HOME"

    cat > "$PM2_CONFIG" <<PM2
module.exports = {
    apps: [{
        name: "$APP_NAME",
        script: "$BINARY",
        interpreter: "none",
        cwd: "$APP_DIR",

        env: {
            BOT_TOKEN: "$BOT_TOKEN",
            ADMIN_CHAT_ID: "$ADMIN_CHAT_ID",
            CHANNEL_ID: "$CHANNEL_ID",
            ADMIN_IDS: "$ADMIN_IDS",
            DB_PATH: "$DATA_DIR/data.db"
        },

        autorestart: true,
        restart_delay: 5000,
        time: true
    }]
};
PM2

    chown root:root "$PM2_CONFIG"
    chmod 644 "$PM2_CONFIG"

    runuser -u "$APP_NAME" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 delete "$APP_NAME" >/dev/null 2>&1 || true

    runuser -u "$APP_NAME" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 start "$PM2_CONFIG"

    runuser -u "$APP_NAME" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 save

    runuser -u "$APP_NAME" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 startup systemd \
            -u "$APP_NAME" \
            --hp "$APP_DIR" \
        >/tmp/${APP_NAME}-pm2-startup.txt 2>&1 || true

    MANAGER_INFO="PM2"
fi

echo
echo "Checking bot status..."
sleep 2

if [[ "$PROCESS_MANAGER" == "systemd" ]]; then

    RUNNING="$(systemctl is-active "$APP_NAME" 2>/dev/null || true)"

else

    RUNNING="$(
        runuser -u "$APP_NAME" -- \
            env PM2_HOME="$PM2_HOME" \
            pm2 jlist 2>/dev/null |
        grep -q '"status":"online"' &&
        echo online ||
        true
    )"

fi

if [[ "$RUNNING" == "active" || "$RUNNING" == "online" ]]; then

    echo
    echo "========================================"
    echo " Installation complete!"
    echo "========================================"
    echo

    echo "Version:        $LATEST_TAG"
    echo "Architecture:   linux-$ARCH"
    echo "Process manager: $MANAGER_INFO"
    echo "Binary:         $BINARY"
    echo "Database:       $DATA_DIR/data.db"
    echo

    if [[ "$PROCESS_MANAGER" == "systemd" ]]; then

        echo "Status:"
        echo "  systemctl status $APP_NAME"
        echo

        echo "Logs:"
        echo "  journalctl -u $APP_NAME -f"

    else

        echo "Status:"
        echo "  runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 status"
        echo

        echo "Logs:"
        echo "  runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 logs $APP_NAME"

        if [[ -s /tmp/${APP_NAME}-pm2-startup.txt ]]; then
            echo
            echo "PM2 startup instructions were saved to:"
            echo "  /tmp/${APP_NAME}-pm2-startup.txt"
            echo
            cat /tmp/${APP_NAME}-pm2-startup.txt || true
        fi
    fi

    echo
    echo "No inbound port is required."
    echo "The bot uses Telegram long polling."
    echo

else

    echo
    echo "The bot did not start successfully."
    echo

    if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
        echo "Check:"
        echo "  journalctl -u $APP_NAME -n 50 --no-pager"
    else
        echo "Check:"
        echo "  runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 logs $APP_NAME --lines 50"
    fi

    exit 1
fi