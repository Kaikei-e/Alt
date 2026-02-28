#!/bin/bash
# systemd タイマーのインストール
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ALT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Installing Alt auto-deploy systemd units..."

# Substitute template variables in the service file
sed -e "s|__ALT_ROOT__|${ALT_ROOT}|g" \
    -e "s|__USER__|${USER}|g" \
    -e "s|__HOME__|${HOME}|g" \
    "$SCRIPT_DIR/systemd/alt-deploy.service" | sudo tee /etc/systemd/system/alt-deploy.service > /dev/null
sudo cp "$SCRIPT_DIR/systemd/alt-deploy.timer" /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now alt-deploy.timer

echo "Alt auto-deploy timer installed and started."
echo ""
echo "Useful commands:"
echo "  systemctl status alt-deploy.timer    # Check timer status"
echo "  systemctl list-timers alt-deploy*    # List timer schedule"
echo "  journalctl -u alt-deploy.service     # View deploy logs"
echo "  sudo systemctl stop alt-deploy.timer # Stop auto-deploy"
