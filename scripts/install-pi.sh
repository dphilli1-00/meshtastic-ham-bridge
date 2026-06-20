#!/bin/bash
# install-pi.sh — run this on the Raspberry Pi to set up the bridge.
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/dphilli/meshtastic-ham-bridge/main/scripts/install-pi.sh | bash
# Or copy and run directly:
#   bash scripts/install-pi.sh

set -e

BINARY_URL="https://github.com/dphilli/meshtastic-ham-bridge/releases/latest/download/meshtastic-ham-bridge-arm64"
BINARY=/usr/local/bin/meshtastic-ham-bridge
SERVICE=/etc/systemd/system/meshtastic-ham-bridge.service
CONFIG_DIR="$HOME/.config/meshtastic-ham-bridge"
CONFIG="$CONFIG_DIR/config.toml"

echo "=== Meshtastic Ham Bridge — Pi Installer ==="

# --- Dependencies ---
echo "[1/5] Installing dependencies..."
sudo apt-get update -qq
sudo apt-get install -y -qq direwolf alsa-utils

# Add pi user to dialout (serial) and audio groups
sudo usermod -aG dialout,audio "$USER"

# --- Binary ---
echo "[2/5] Installing bridge binary..."
if [ -f "./meshtastic-ham-bridge-arm64" ]; then
    # Running from project directory after cross-compile
    sudo cp ./meshtastic-ham-bridge-arm64 "$BINARY"
else
    # Download from GitHub releases
    sudo curl -sL "$BINARY_URL" -o "$BINARY"
fi
sudo chmod +x "$BINARY"

# --- Config ---
echo "[3/5] Setting up config..."
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG" ]; then
    "$BINARY" -init-config
    echo "Config written to $CONFIG — edit it to set your serial ports."
else
    echo "Config already exists at $CONFIG — skipping."
fi

# --- Direwolf config ---
echo "[4/5] Setting up Direwolf..."
DIREWOLF_CONF="$HOME/direwolf.conf"
if [ ! -f "$DIREWOLF_CONF" ]; then
cat > "$DIREWOLF_CONF" << 'DIREWOLF'
# Direwolf config for Meshtastic Ham Bridge on Pi
# Edit MYCALL and ADEVICE to match your setup

MYCALL NOCALL-1
ADEVICE plughw:1,0    # Digirig audio device — run 'aplay -l' to find yours
CHANNEL 0
MODEM 1200
KISSPORT 8001
DIREWOLF
    echo "Direwolf config written to $DIREWOLF_CONF — edit MYCALL and ADEVICE."
else
    echo "Direwolf config already exists — skipping."
fi

# Direwolf systemd service
cat > /tmp/direwolf.service << 'DWSVC'
[Unit]
Description=Direwolf APRS/KISS TNC
After=sound.target

[Service]
Type=simple
User=pi
ExecStart=/usr/bin/direwolf -c /home/pi/direwolf.conf -p
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
DWSVC
sudo mv /tmp/direwolf.service /etc/systemd/system/direwolf.service

# --- Bridge systemd service ---
echo "[5/5] Installing systemd service..."
sudo cp "$(dirname "$0")/../deploy/systemd/meshtastic-ham-bridge.service" "$SERVICE" 2>/dev/null || \
cat > /tmp/bridge.service << 'BSVC'
[Unit]
Description=Meshtastic Ham Bridge
After=network.target direwolf.service

[Service]
Type=simple
User=pi
Group=pi
ExecStart=/usr/local/bin/meshtastic-ham-bridge
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=meshtastic-ham-bridge
SupplementaryGroups=dialout audio

[Install]
WantedBy=multi-user.target
BSVC
[ -f /tmp/bridge.service ] && sudo mv /tmp/bridge.service "$SERVICE"

sudo systemctl daemon-reload
sudo systemctl enable direwolf meshtastic-ham-bridge
sudo systemctl start direwolf

echo ""
echo "=== Done! ==="
echo ""
echo "Next steps:"
echo "  1. Edit $CONFIG to set your Meshtastic serial port"
echo "  2. Edit $HOME/direwolf.conf to set MYCALL and audio device"
echo "     (run 'aplay -l' to list audio devices, look for Digirig)"
echo "  3. sudo systemctl start meshtastic-ham-bridge"
echo "  4. sudo journalctl -fu meshtastic-ham-bridge   # watch logs"
echo ""
echo "Note: log out and back in for group changes (dialout/audio) to take effect."
