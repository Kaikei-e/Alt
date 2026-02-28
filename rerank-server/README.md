# Rerank Server for M-series Mac

MPS-accelerated reranking service using `sentence-transformers` CrossEncoder on Apple Silicon.

> **Note:** This server does not include authentication. It is designed to run on a private network (e.g., Tailscale) and should **not** be exposed directly to the public internet. Deploy behind a reverse proxy with authentication if public access is required.

## Requirements

- Python 3.11+
- Apple Silicon Mac (M-series recommended)

## Installation

```bash
pip install -r requirements.txt
```

## Running

```bash
# Development
uvicorn rerank_server:app --host 0.0.0.0 --port 8080

# Or directly
python rerank_server.py
```

## Endpoints

### POST /v1/rerank

Rerank candidates based on query relevance.

```bash
curl -X POST http://localhost:8080/v1/rerank \
  -H "Content-Type: application/json" \
  -d '{"query": "machine learning", "candidates": ["deep learning", "cooking recipes", "neural networks"]}'
```

Response:
```json
{
  "results": [
    {"index": 0, "score": 0.95},
    {"index": 2, "score": 0.85},
    {"index": 1, "score": 0.1}
  ],
  "model": "BAAI/bge-reranker-v2-m3",
  "processing_time_ms": 123.45
}
```

### GET /health

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "ok",
  "device": "mps",
  "model": "BAAI/bge-reranker-v2-m3"
}
```

## Systemd Service (Optional)

Create `/etc/systemd/system/rerank-server.service`:

```ini
[Unit]
Description=Rerank Server
After=network.target

[Service]
Type=simple
User=youruser
WorkingDirectory=/path/to/rerank-server
ExecStart=/usr/bin/python3 -m uvicorn rerank_server:app --host 0.0.0.0 --port 8080
Restart=always

[Install]
WantedBy=multi-user.target
```

Then:
```bash
sudo systemctl daemon-reload
sudo systemctl enable rerank-server
sudo systemctl start rerank-server
```

## launchd (macOS)

Create `~/Library/LaunchAgents/com.alt.rerank-server.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.alt.rerank-server</string>
    <key>ProgramArguments</key>
    <array>
        <string>/opt/homebrew/bin/python3</string>
        <string>-m</string>
        <string>uvicorn</string>
        <string>rerank_server:app</string>
        <string>--host</string>
        <string>0.0.0.0</string>
        <string>--port</string>
        <string>8080</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/path/to/rerank-server</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/rerank-server.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/rerank-server.err</string>
</dict>
</plist>
```

Then:
```bash
launchctl load ~/Library/LaunchAgents/com.alt.rerank-server.plist
```

## Tailscale経由でのアクセス (Docker環境向け)

M-series MacでrerankerをローカルでホストしつつDocker環境からTailscale経由でアクセスするには、socatでフォワーディングを設定します。

### セットアップ

```bash
# M-series Mac上で実行
./setup-socat.sh
```

このスクリプトは以下を行います:
1. TailscaleのIPアドレスを自動検出
2. socatのLaunchAgentを作成 (`~/Library/LaunchAgents/com.user.rerank-socat.plist`)
3. Tailscale IP:8080 → 127.0.0.1:8080 のフォワーディングを設定

### 動作確認

```bash
# ポート8080のリッスン状態を確認
lsof -i :8080
# 期待: Python (0.0.0.0:8080) と socat (Tailscale IP:8080) の両方

# Tailscale経由でアクセス確認
curl http://<YOUR_TAILSCALE_IP>:8080/health
# 期待: {"status":"ok","device":"mps","model":"BAAI/bge-reranker-v2-m3"}
```

### 管理コマンド

```bash
# 停止
launchctl bootout gui/$(id -u)/com.user.rerank-socat

# 開始
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.user.rerank-socat.plist

# ログ確認
tail -f ~/.rerank-server/logs/socat.stderr.log
```

### Docker環境からのアクセス

Docker Compose の `extra_hosts` で Tailscale IP をホスト名にマッピングできます:

```yaml
extra_hosts:
  - "rerank-external:<YOUR_TAILSCALE_IP>"
```

サービスから `http://rerank-external:8080/v1/rerank` でアクセス可能。
