#!/usr/bin/env bash
set -euo pipefail

APP_NAME="suggestion-bot"
APP_DIR="/opt/$APP_NAME"
DATA_DIR="/var/lib/$APP_NAME"
SERVICE_FILE="/etc/systemd/system/$APP_NAME.service"
PM2_CONFIG="$APP_DIR/ecosystem.config.js"
ENV_FILE="$APP_DIR/.env"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root or with sudo."
  exit 1
fi

if [[ ! -f main.go ]]; then
  echo "main.go was not found. Run the installer from the project directory."
  exit 1
fi

echo "========================================"
echo " Telegram Suggestion Bot Installer"
echo "========================================"
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
  *) echo "Invalid choice."; exit 1 ;;
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
read -rp "Install/update $APP_NAME with these settings? [y/N]: " CONFIRM
if [[ ! "$CONFIRM" =~ ^[Yy]$ ]]; then
  echo "Cancelled."
  exit 0
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl sqlite3 golang-go

if [[ "$PROCESS_MANAGER" == "pm2" ]] && ! command -v pm2 >/dev/null 2>&1; then
  if ! command -v npm >/dev/null 2>&1; then
    apt-get install -y nodejs npm
  fi
  npm install -g pm2
fi

if ! id -u "$APP_NAME" >/dev/null 2>&1; then
  useradd --system --home "$APP_DIR" --shell /usr/sbin/nologin "$APP_NAME"
fi

mkdir -p "$APP_DIR" "$DATA_DIR"
cp main.go go.mod "$APP_DIR/"
chown -R root:root "$APP_DIR"
chmod 755 "$APP_DIR"
chmod 644 "$APP_DIR/main.go" "$APP_DIR/go.mod"

cat > "$ENV_FILE" <<ENV
BOT_TOKEN=$BOT_TOKEN
ADMIN_CHAT_ID=$ADMIN_CHAT_ID
CHANNEL_ID=$CHANNEL_ID
ADMIN_IDS=$ADMIN_IDS
DB_PATH=$DATA_DIR/data.db
ENV

chown root:"$APP_NAME" "$ENV_FILE"
chmod 640 "$ENV_FILE"

cd "$APP_DIR"
go mod tidy
go build -trimpath -ldflags="-s -w" -o "$APP_DIR/$APP_NAME" .
chown root:root "$APP_DIR/$APP_NAME"
chmod 755 "$APP_DIR/$APP_NAME"

chown -R "$APP_NAME":"$APP_NAME" "$DATA_DIR"

if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
  cat > "$SERVICE_FILE" <<SERVICE
[Unit]
Description=Telegram Suggestion Bot
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$APP_NAME
Group=$APP_NAME
WorkingDirectory=$APP_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$APP_DIR/$APP_NAME
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
  systemctl enable --now "$APP_NAME"

  MANAGER_INFO="systemd"
else
  rm -f "$SERVICE_FILE"
  cat > "$PM2_CONFIG" <<PM2
module.exports = {
  apps: [{
    name: "$APP_NAME",
    script: "$APP_DIR/$APP_NAME",
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

  PM2_HOME="$DATA_DIR/.pm2"
  mkdir -p "$PM2_HOME"
  chown -R "$APP_NAME":"$APP_NAME" "$PM2_HOME"
  runuser -u "$APP_NAME" -- env PM2_HOME="$PM2_HOME" pm2 delete "$APP_NAME" >/dev/null 2>&1 || true
  runuser -u "$APP_NAME" -- env PM2_HOME="$PM2_HOME" pm2 start "$PM2_CONFIG"
  runuser -u "$APP_NAME" -- env PM2_HOME="$PM2_HOME" pm2 save
  runuser -u "$APP_NAME" -- env PM2_HOME="$PM2_HOME" pm2 startup systemd -u "$APP_NAME" --hp "$APP_DIR" >/tmp/${APP_NAME}-pm2-startup.txt 2>&1 || true

  MANAGER_INFO="PM2"
fi

sleep 2
if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
  RUNNING="$(systemctl is-active "$APP_NAME" 2>/dev/null || true)"
else
  RUNNING="$(runuser -u "$APP_NAME" -- env PM2_HOME="$PM2_HOME" pm2 jlist 2>/dev/null | grep -q '"status":"online"' && echo online || true)"
fi

if [[ "$RUNNING" == "active" || "$RUNNING" == "online" ]]; then
  echo
  echo "Installation complete."
  echo "Process manager: $MANAGER_INFO"
  if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
    echo "Status: systemctl status $APP_NAME"
    echo "Logs:   journalctl -u $APP_NAME -f"
  else
    echo "Status: runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 status"
    echo "Logs:   runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 logs $APP_NAME"
    if [[ -s /tmp/${APP_NAME}-pm2-startup.txt ]]; then
      echo
      echo "PM2 startup instructions were saved to /tmp/${APP_NAME}-pm2-startup.txt"
      cat /tmp/${APP_NAME}-pm2-startup.txt || true
    fi
  fi
  echo "Data:   $DATA_DIR/data.db"
  echo
  echo "No inbound port is required: the bot uses Telegram long polling."
else
  echo
  echo "The bot did not start successfully."
  if [[ "$PROCESS_MANAGER" == "systemd" ]]; then
    echo "Check: journalctl -u $APP_NAME -n 50 --no-pager"
  else
    echo "Check: runuser -u $APP_NAME -- env PM2_HOME=$PM2_HOME pm2 logs $APP_NAME --lines 50"
  fi
  exit 1
fi
