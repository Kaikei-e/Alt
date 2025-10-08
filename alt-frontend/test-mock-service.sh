#!/bin/bash
# モックサービスの動作確認スクリプト

echo "=== モックサービステスト開始 ==="

# モックサービスを起動
node tests/mock-auth-service.cjs > mock-test.log 2>&1 &
MOCK_PID=$!
echo "モックサービス起動 (PID: $MOCK_PID)"
sleep 3

echo ""
echo "=== 1. Flow作成テスト ==="
RESPONSE=$(curl -s "http://localhost:4545/self-service/login/browser?return_to=http://localhost:3010/desktop/home" -i)
echo "$RESPONSE" | head -20

echo ""
echo "=== 2. Flow取得テスト ==="
# レスポンスからflow IDを抽出
FLOW_ID=$(echo "$RESPONSE" | grep -oP 'flow=[a-f0-9-]{32,36}' | head -1 | cut -d= -f2)
echo "Flow ID: $FLOW_ID"

if [ -n "$FLOW_ID" ]; then
    FLOW_RESPONSE=$(curl -s "http://localhost:4545/self-service/login/flows?id=$FLOW_ID")
    echo "Flow取得レスポンス:"
    echo "$FLOW_RESPONSE" | jq '.' 2>/dev/null || echo "$FLOW_RESPONSE"

    echo ""
    echo "=== 3. ログイン送信テスト ==="
    # CSRFトークンを抽出
    CSRF=$(echo "$FLOW_RESPONSE" | jq -r '.ui.nodes[] | select(.attributes.name=="csrf_token") | .attributes.value' 2>/dev/null)
    echo "CSRF Token: $CSRF"

    if [ -n "$CSRF" ]; then
        LOGIN_RESPONSE=$(curl -s -X POST "http://localhost:4545/self-service/login?flow=$FLOW_ID" \
            -H "Content-Type: application/x-www-form-urlencoded" \
            -d "identifier=test@example.com&password=password123&csrf_token=$CSRF&method=password" \
            -i)

        echo "ログインレスポンス:"
        echo "$LOGIN_RESPONSE" | grep -E "HTTP|Set-Cookie|continue_with|redirect_browser_to" || echo "$LOGIN_RESPONSE"

        # JSONボディを抽出して整形
        echo ""
        echo "レスポンスボディ (JSON):"
        echo "$LOGIN_RESPONSE" | sed -n '/^{/,/^}/p' | jq '.' 2>/dev/null || echo "$LOGIN_RESPONSE" | sed -n '/^{/,/^}/p'
    fi
fi

echo ""
echo "=== モックサービス停止 ==="
kill $MOCK_PID 2>/dev/null || true
echo "テスト完了"
