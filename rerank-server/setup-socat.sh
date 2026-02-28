#!/bin/bash

# rerank-server用 socat フォワーダーセットアップスクリプト
# Tailscale経由でrerank-serverにアクセスできるようにする
#
# 使用方法: M-series Mac上で実行
#   ./setup-socat.sh
#
# 前提:
#   - rerank-serverが127.0.0.1:8080でリッスン中
#   - Tailscaleが起動している
#   - socatがインストールされている (brew install socat)

set -e

echo "=== rerank-server socat フォワーダーセットアップ ==="
echo ""

# ディレクトリ作成
mkdir -p ~/Library/LaunchAgents
mkdir -p ~/.rerank-server/logs

# TailscaleのIPアドレスを取得
TAILSCALE_IP=""

# 方法1: tailscaleコマンド
if command -v tailscale &> /dev/null; then
    TAILSCALE_IP=$(tailscale ip -4 2>/dev/null)
fi

# 方法2: Tailscale.appのCLI (Mac)
if [ -z "$TAILSCALE_IP" ] && [ -x "/Applications/Tailscale.app/Contents/MacOS/Tailscale" ]; then
    TAILSCALE_IP=$(/Applications/Tailscale.app/Contents/MacOS/Tailscale ip -4 2>/dev/null)
fi

# 方法3: ifconfigからutunインターフェースを検索 (100.x.x.x)
if [ -z "$TAILSCALE_IP" ]; then
    TAILSCALE_IP=$(ifconfig 2>/dev/null | grep -A1 'utun' | grep 'inet 100\.' | awk '{print $2}' | head -1)
fi

# 方法4: 全インターフェースから100.x.x.xを検索
if [ -z "$TAILSCALE_IP" ]; then
    TAILSCALE_IP=$(ifconfig 2>/dev/null | grep 'inet 100\.' | awk '{print $2}' | head -1)
fi

if [ -z "$TAILSCALE_IP" ]; then
    echo "エラー: TailscaleのIPアドレスが取得できません"
    echo "  Tailscaleが起動していることを確認してください"
    exit 1
fi

echo "✓ Tailscale IP: $TAILSCALE_IP"

# socatの存在確認
SOCAT_BIN=""
if command -v socat &> /dev/null; then
    SOCAT_BIN=$(command -v socat)
elif [ -x "/opt/homebrew/bin/socat" ]; then
    SOCAT_BIN="/opt/homebrew/bin/socat"
elif [ -x "/usr/local/bin/socat" ]; then
    SOCAT_BIN="/usr/local/bin/socat"
fi

if [ -z "$SOCAT_BIN" ]; then
    echo "エラー: socatが見つかりません"
    echo "  インストール: brew install socat"
    exit 1
fi

echo "✓ socat: $SOCAT_BIN"

# rerank-serverが起動しているか確認
echo ""
echo "rerank-serverの状態を確認中..."
if lsof -i :8080 2>/dev/null | grep -q "Python"; then
    echo "✓ rerank-serverがポート8080でリッスン中"
else
    echo "⚠ rerank-serverがポート8080でリッスンしていません"
    echo "  先にrerank-serverを起動してください"
    echo "  (セットアップは続行します)"
fi

# ユーザー情報
USER_ID=$(id -u)
DOMAIN="gui/$USER_ID"

SOCAT_PLIST_PATH="$HOME/Library/LaunchAgents/com.user.rerank-socat.plist"
SOCAT_SERVICE_LABEL="com.user.rerank-socat"

echo ""
echo "socat LaunchAgentを作成中..."

cat > "$SOCAT_PLIST_PATH" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${SOCAT_SERVICE_LABEL}</string>

    <key>ProgramArguments</key>
    <array>
        <string>${SOCAT_BIN}</string>
        <string>TCP4-LISTEN:8080,fork,reuseaddr,bind=${TAILSCALE_IP}</string>
        <string>TCP4:127.0.0.1:8080</string>
    </array>

    <key>StandardOutPath</key>
    <string>${HOME}/.rerank-server/logs/socat.stdout.log</string>
    <key>StandardErrorPath</key>
    <string>${HOME}/.rerank-server/logs/socat.stderr.log</string>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>ProcessType</key>
    <string>Background</string>
</dict>
</plist>
EOF

echo "✓ plistを作成しました: $SOCAT_PLIST_PATH"

# 既存のサービスをアンロード
echo ""
echo "既存のsocatサービスをアンロード中..."
launchctl bootout "$DOMAIN/$SOCAT_SERVICE_LABEL" 2>/dev/null || true
pkill -f "socat.*TCP4-LISTEN:8080" 2>/dev/null || true
sleep 1

# 新しいサービスをロード
echo ""
echo "socatサービスをロード中..."
if launchctl bootstrap "$DOMAIN" "$SOCAT_PLIST_PATH" 2>&1; then
    echo "✓ socatサービスを正常にロードしました"
else
    if launchctl load "$SOCAT_PLIST_PATH" 2>&1; then
        echo "✓ socatサービスを正常にロードしました（load方式）"
    else
        echo "エラー: socatサービスのロードに失敗しました"
        exit 1
    fi
fi

sleep 2

# 確認
echo ""
echo "=== 確認 ==="

echo ""
echo "ポート8080のリッスン状態:"
lsof -i :8080 2>/dev/null | head -5 || echo "  (リッスンなし)"

echo ""
echo "socatリッスン状態:"
if lsof -i :8080 2>/dev/null | grep -q socat; then
    lsof -i :8080 2>/dev/null | grep socat | head -1
    echo "✓ socatがTailscale IP ($TAILSCALE_IP) でリッスン中"
else
    echo "⚠ socatがリッスンしていません"
    echo "  ログを確認: tail -20 ~/.rerank-server/logs/socat.stderr.log"
fi

# Tailscale経由でテスト
echo ""
echo "Tailscale経由API確認 (5秒タイムアウト):"
TS_URL="http://${TAILSCALE_IP}:8080/health"
TS_RESULT=$(curl -s --connect-timeout 5 --max-time 5 "$TS_URL" 2>&1) && TS_SUCCESS=true || TS_SUCCESS=false

if [ "$TS_SUCCESS" = true ] && [ -n "$TS_RESULT" ]; then
    echo "✓ Tailscale経由応答: $TS_RESULT"
else
    echo "⚠ Tailscale経由でAPIが応答しません"
    echo ""
    echo "  原因の可能性:"
    echo "    1. rerank-serverが起動していない"
    echo "    2. ファイアウォールでブロックされている"
    echo ""
    echo "  ファイアウォールにsocatを追加:"
    echo "    sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add $SOCAT_BIN"
    echo "    sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp $SOCAT_BIN"
fi

echo ""
echo "=== セットアップ完了 ==="
echo ""
echo "設定:"
echo "  Tailscale IP: $TAILSCALE_IP"
echo "  ポート: 8080"
echo "  plist: $SOCAT_PLIST_PATH"
echo ""
echo "ログファイル:"
echo "  stdout: ~/.rerank-server/logs/socat.stdout.log"
echo "  stderr: ~/.rerank-server/logs/socat.stderr.log"
echo ""
echo "管理コマンド:"
echo "  停止:   launchctl bootout $DOMAIN/$SOCAT_SERVICE_LABEL"
echo "  開始:   launchctl bootstrap $DOMAIN $SOCAT_PLIST_PATH"
echo "  状態:   launchctl print $DOMAIN/$SOCAT_SERVICE_LABEL"
echo "  ログ:   tail -f ~/.rerank-server/logs/socat.stderr.log"
echo ""
echo "接続テスト (他のマシンから):"
echo "  curl http://$TAILSCALE_IP:8080/health"
echo ""
echo "期待される応答:"
echo '  {"status":"ok","device":"mps","model":"BAAI/bge-reranker-v2-m3"}'
