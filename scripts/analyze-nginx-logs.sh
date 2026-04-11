#!/bin/bash
# nginxログ解析スクリプト
# SvelteKit（/sv）へのリクエストのパフォーマンスを分析

set -euo pipefail

# 色の定義
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# デフォルト値
CONTAINER_NAME="${NGINX_CONTAINER:-nginx}"
LOG_LINES="${LOG_LINES:-1000}"
THRESHOLD="${THRESHOLD:-1.0}"  # 1秒以上のリクエストを警告

echo "=== nginxログ解析: SvelteKit (/sv) パフォーマンス分析 ==="
echo ""

# Dockerコンテナが存在するか確認
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo -e "${RED}エラー: コンテナ '${CONTAINER_NAME}' が見つかりません${NC}"
    echo "利用可能なコンテナ:"
    docker ps --format '{{.Names}}' | grep -E '(nginx|alt-)' || echo "  なし"
    exit 1
fi

echo "コンテナ: ${CONTAINER_NAME}"
echo "分析対象: 直近 ${LOG_LINES} 行"
echo ""

# 一時ファイル
TEMP_LOG=$(mktemp)
TEMP_PARSED=$(mktemp)

# クリーンアップ関数
cleanup() {
    rm -f "$TEMP_LOG" "$TEMP_PARSED"
}
trap cleanup EXIT

# nginxログを取得（/svパスへのリクエストのみ）
echo "ログを取得中..."
docker exec "$CONTAINER_NAME" tail -n "$LOG_LINES" /var/log/nginx/access.log | \
    grep -E '"/sv' > "$TEMP_LOG" || {
    echo -e "${YELLOW}警告: /sv へのリクエストが見つかりませんでした${NC}"
    exit 0
}

LOG_COUNT=$(wc -l < "$TEMP_LOG")
if [ "$LOG_COUNT" -eq 0 ]; then
    echo -e "${YELLOW}警告: /sv へのリクエストが見つかりませんでした${NC}"
    exit 0
fi

echo "見つかったリクエスト数: ${LOG_COUNT}"
echo ""

# ログを解析してレスポンスタイムを抽出
echo "ログを解析中..."
while IFS= read -r line; do
    # rt= から数値を抽出（リクエストタイム）
    request_time=$(echo "$line" | grep -oP 'rt=\K[0-9.]+' || echo "0")
    upstream_connect_time=$(echo "$line" | grep -oP 'uct=\K[0-9.]+' || echo "0")
    upstream_header_time=$(echo "$line" | grep -oP 'uht=\K[0-9.]+' || echo "0")
    upstream_response_time=$(echo "$line" | grep -oP 'urt=\K[0-9.]+' || echo "0")

    # リクエストパスを抽出
    request_path=$(echo "$line" | grep -oP '"\K[^"]*' | head -1 | awk '{print $2}')

    # ステータスコードを抽出
    status=$(echo "$line" | awk '{print $9}')

    # タイムスタンプを抽出
    timestamp=$(echo "$line" | grep -oP '\[\K[^\]]+')

    # パース済みデータを出力
    printf "%s|%s|%s|%s|%s|%s|%s|%s\n" \
        "$timestamp" \
        "$request_path" \
        "$status" \
        "$request_time" \
        "$upstream_connect_time" \
        "$upstream_header_time" \
        "$upstream_response_time" \
        "$line"
done < "$TEMP_LOG" > "$TEMP_PARSED"

# 統計情報を計算
echo "=== 統計情報 ==="
echo ""

# 平均レスポンスタイム
avg_rt=$(awk -F'|' '{sum+=$4; count++} END {if(count>0) printf "%.3f", sum/count; else print "0"}' "$TEMP_PARSED")
echo "平均リクエストタイム: ${avg_rt}秒"

# 中央値
median_rt=$(awk -F'|' '{print $4}' "$TEMP_PARSED" | sort -n | awk '{
    a[NR]=$1
} END {
    if(NR%2==1) print a[(NR+1)/2]
    else print (a[NR/2]+a[NR/2+1])/2
}')

echo "中央値リクエストタイム: ${median_rt}秒"

# 最大値
max_rt=$(awk -F'|' 'BEGIN{max=0} {if($4>max) max=$4} END {printf "%.3f", max}' "$TEMP_PARSED")
max_line=$(awk -F'|' -v max="$max_rt" '$4==max {print $2" (status: "$3")"}' "$TEMP_PARSED" | head -1)
echo "最大リクエストタイム: ${max_rt}秒 (${max_line})"

# 最小値
min_rt=$(awk -F'|' 'BEGIN{min=999} {if($4<min && $4>0) min=$4} END {printf "%.3f", min}' "$TEMP_PARSED")
echo "最小リクエストタイム: ${min_rt}秒"

# アップストリーム統計
echo ""
echo "=== アップストリーム統計 ==="
avg_urt=$(awk -F'|' '{sum+=$7; count++} END {if(count>0) printf "%.3f", sum/count; else print "0"}' "$TEMP_PARSED")
echo "平均アップストリーム応答タイム: ${avg_urt}秒"

avg_uct=$(awk -F'|' '{sum+=$5; count++} END {if(count>0) printf "%.3f", sum/count; else print "0"}' "$TEMP_PARSED")
echo "平均アップストリーム接続タイム: ${avg_uct}秒"

# 遅いリクエストを特定
echo ""
echo "=== 遅いリクエスト (${THRESHOLD}秒以上) ==="
slow_count=$(awk -F'|' -v threshold="$THRESHOLD" '$4 >= threshold {count++} END {print count+0}' "$TEMP_PARSED")

if [ "$slow_count" -eq 0 ]; then
    echo -e "${GREEN}遅いリクエストは見つかりませんでした${NC}"
else
    echo -e "${YELLOW}遅いリクエスト数: ${slow_count}${NC}"
    echo ""
    echo "トップ10の遅いリクエスト:"
    echo "----------------------------------------------------------------------"
    printf "%-20s %-40s %-6s %-8s %-8s\n" "時刻" "パス" "Status" "RT(秒)" "URT(秒)"
    echo "----------------------------------------------------------------------"
    awk -F'|' -v threshold="$THRESHOLD" '$4 >= threshold {
        printf "%-20s %-40s %-6s %-8.3f %-8.3f\n", $1, substr($2,1,40), $3, $4, $7
    }' "$TEMP_PARSED" | sort -k4 -rn | head -10
fi

# ステータスコード別統計
echo ""
echo "=== ステータスコード別統計 ==="
awk -F'|' '{
    status[$3]++
    total++
} END {
    for (s in status) {
        printf "  %s: %d (%.1f%%)\n", s, status[s], (status[s]/total)*100
    }
}' "$TEMP_PARSED" | sort -rn

# エンドポイント別統計
echo ""
echo "=== エンドポイント別統計 (トップ10) ==="
awk -F'|' '{
    # パスからエンドポイントを抽出（クエリパラメータを除去）
    endpoint=$2
    gsub(/\?.*/, "", endpoint)
    endpoints[endpoint]++
    rt_sum[endpoint] += $4
    count[endpoint]++
} END {
    for (ep in endpoints) {
        avg = rt_sum[ep] / count[ep]
        printf "%.3f|%d|%s\n", avg, count[ep], ep
    }
}' "$TEMP_PARSED" | sort -rn | head -10 | while IFS='|' read -r avg count endpoint; do
    printf "  %-50s: 平均 %.3f秒 (%d回)\n" "$endpoint" "$avg" "$count"
done

echo ""
echo "=== 分析完了 ==="

