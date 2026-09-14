#!/usr/bin/env bash
set -euo pipefail

REPO="DanMotive/TelegramDirect"

APP_NAME="suggestion-bot"
APP_DIR="/opt/$APP_NAME"
DATA_DIR="/var/lib/$APP_NAME"

BINARY="$APP_DIR/$APP_NAME"
RUNNER="$APP_DIR/run.sh"

ENV_FILE="$APP_DIR/.env"
CONFIG_FILE="$APP_DIR/config.json"

SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
PM2_CONFIG="$APP_DIR/ecosystem.config.js"

CLI_SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/telegramdirect"
CONFIG_SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/config.example.json"

if [[ "${EUID}" -ne 0 ]]; then
    echo "Run this script as root or with sudo."
    exit 1
fi

echo "========================================"
echo " TelegramDirect Bot Installer"
echo "========================================"
echo

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

echo "Detected architecture: linux-$ARCH"
echo

if [[ ! -f "$CONFIG_SOURCE" ]]; then
    echo "config.example.json was not found."
    echo "Run the installer from the TelegramDirect project directory."
    exit 1
fi

if [[ ! -f "$CLI_SOURCE" ]]; then
    echo "telegramdirect CLI script was not found."
    echo "Run the installer from the TelegramDirect project directory."
    exit 1
fi

echo "Telegram IDs can be viewed in Telegram Desktop Developer Mode."
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
    1)
        PROCESS_MANAGER="systemd"
        ;;
    2)
        PROCESS_MANAGER="pm2"
        ;;
    *)
        echo "Invalid choice."
        exit 1
        ;;
esac

if [[ -z "$BOT_TOKEN" ||
      -z "$ADMIN_CHAT_ID" ||
      -z "$CHANNEL_ID" ||
      -z "$ADMIN_IDS" ]]; then
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
echo "Repository:     $REPO"
echo "Architecture:   linux-$ARCH"
echo "Manager:        $PROCESS_MANAGER"
echo

read -rp "Continue installation? [y/N]: " CONFIRM

if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

export DEBIAN_FRONTEND=noninteractive

echo
echo "Installing required packages..."

apt-get update
apt-get install -y ca-certificates curl

if [[ "$PROCESS_MANAGER" == "pm2" ]]; then
    if ! command -v pm2 >/dev/null 2>&1; then
        echo "PM2 is not installed."

        if ! command -v npm >/dev/null 2>&1; then
            apt-get install -y nodejs npm
        fi

        npm install -g pm2
    fi
fi

echo
echo "Checking latest GitHub release..."

LATEST_TAG="$(
    curl -fsSL \
        -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' |
        head -n 1
)"

if [[ -z "$LATEST_TAG" ]]; then
    echo "Failed to determine latest release."
    exit 1
fi

echo "Latest release: $LATEST_TAG"

ASSET="suggestion-bot-linux-$ARCH"

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$ASSET"
CHECKSUM_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/checksums.txt"

TMP_BINARY="$(mktemp)"
TMP_CHECKSUMS="$(mktemp)"

cleanup() {
    rm -f "$TMP_BINARY" "$TMP_CHECKSUMS"
}

trap cleanup EXIT

echo
echo "Downloading binary..."
echo "$DOWNLOAD_URL"

curl -fL "$DOWNLOAD_URL" -o "$TMP_BINARY"

echo "Downloading checksums..."

curl -fL "$CHECKSUM_URL" -o "$TMP_CHECKSUMS"

EXPECTED="$(
    awk -v file="$ASSET" '$2 == file {print $1}' "$TMP_CHECKSUMS"
)"

if [[ -z "$EXPECTED" ]]; then
    echo "Checksum for $ASSET was not found."
    exit 1
fi

ACTUAL="$(sha256sum "$TMP_BINARY" | awk '{print $1}')"

if [[ "$EXPECTED" != "$ACTUAL" ]]; then
    echo "SHA-256 verification failed."
    echo "Expected: $EXPECTED"
    echo "Actual:   $ACTUAL"
    exit 1
fi

echo "SHA-256 verification successful."

if ! id -u "$APP_NAME" >/dev/null 2>&1; then
    echo
    echo "Creating system user: $APP_NAME"

    useradd \
        --system \
        --home "$APP_DIR" \
        --shell /usr/sbin/nologin \
        "$APP_NAME"
fi

mkdir -p "$APP_DIR"
mkdir -p "$DATA_DIR"

echo
echo "Installing binary..."

install -m 755 "$TMP_BINARY" "$BINARY"
chown root:root "$BINARY"

echo "Installing CLI..."

install -m 755 "$CLI_SOURCE" /usr/local/bin/telegramdirect

echo "Creating runner..."

cat > "$RUNNER" <<'RUNNER'
#!/usr/bin/env bash
set -euo pipefail

APP_DIR="/opt/suggestion-bot"
ENV_FILE="$APP_DIR/.env"
BINARY="$APP_DIR/suggestion-bot"

set -a
source "$ENV_FILE"
set +a

exec "$BINARY"
RUNNER

chown root:root "$RUNNER"
chmod 755 "$RUNNER"

echo
echo "Creating environment configuration..."

cat > "$ENV_FILE" <<ENV
BOT_TOKEN=$BOT_TOKEN
ADMIN_CHAT_ID=$ADMIN_CHAT_ID
CHANNEL_ID=$CHANNEL_ID
ADMIN_IDS=$ADMIN_IDS
DB_PATH=$DATA_DIR/data.db
CONFIG_PATH=$CONFIG_FILE
ENV

chown root:"$APP_NAME" "$ENV_FILE"
chmod 640 "$ENV_FILE"

echo
echo "Creating config.json..."

if [[ ! -f "$CONFIG_FILE" ]]; then
    cp "$CONFIG_SOURCE" "$CONFIG_FILE"
    chown root:"$APP_NAME" "$CONFIG_FILE"
    chmod 640 "$CONFIG_FILE"
else
    echo "Existing config.json preserved."
fi

chown "$APP_NAME":"$APP_NAME" "$DATA_DIR"
chmod 750 "$DATA_DIR"

if [[ "$PROCESS_MANAGER" == "systemd" ]]; then

    echo
    echo "Configuring systemd..."

    # Remove old PM2 process if switching from PM2.
    if command -v pm2 >/dev/null 2>&1 &&
       id -u "$APP_NAME" >/dev/null 2>&1 &&
       [[ -d "$DATA_DIR/.pm2" ]]; then

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
    systemctl restart "$APP_NAME"

else

    echo
    echo "Configuring PM2..."

    # Remove old systemd service if switching from systemd.
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
        script: "$RUNNER",
        interpreter: "/bin/bash",
        cwd: "$APP_DIR",

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

    echo
    echo "Generating PM2 startup configuration..."

    runuser -u "$APP_NAME" -- \
        env PM2_HOME="$PM2_HOME" \
        pm2 startup systemd \
            -u "$APP_NAME" \
            --hp "$APP_DIR" \
        >/tmp/${APP_NAME}-pm2-startup.txt 2>&1 || true

    MANAGER_INFO="PM2"
fi

sleep 2

echo
echo "Checking bot status..."

if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
    RUNNING="$(systemctl is-active "$APP_NAME" 2>/dev/null || true)"
else
    PM2_HOME="$DATA_DIR/.pm2"

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

    echo "Version:         $LATEST_TAG"
    echo "Architecture:    linux-$ARCH"
    echo "Process manager: $PROCESS_MANAGER"
    echo "Binary:          $BINARY"
    echo "Config:          $CONFIG_FILE"
    echo "Database:        $DATA_DIR/data.db"
    echo
    echo "Management:"
    echo "  telegramdirect config"
    echo "  telegramdirect update"
    echo "  telegramdirect uninstall"
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

        if [[ -s "/tmp/${APP_NAME}-pm2-startup.txt" ]]; then
            echo
            echo "PM2 startup instructions:"
            echo
            cat "/tmp/${APP_NAME}-pm2-startup.txt" || true
        fi
    fi

    echo
    echo "No inbound HTTP port is required."
    echo "The bot uses Telegram long polling."
    echo

else

    echo
    echo "The bot did not start successfully."
    echo

    if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
        echo "Logs:"
        echo "  journalctl -u $APP_NAME -n 50 --no-pager"
    else
        echo "Logs:"
        echo "  runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 logs $APP_NAME --lines 50"
    fi

    exit 1
fi