"""Web版監視ダッシュボード"""

from __future__ import annotations

import asyncio
import logging
import os
import sys
from datetime import datetime

from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from pydantic import BaseModel, ConfigDict

from .monitors import (
    get_cpu_info,
    get_gpu_info,
    get_hanging_processes,
    get_memory_info,
    get_top_processes,
)

logger = logging.getLogger(__name__)

app = FastAPI(title="Recap Job Resource Monitor")

# Loopback-only by default: this dashboard streams every process's command
# line with zero auth once reachable, so it must not default to 0.0.0.0.
WEB_HOST = os.environ.get("UTILIZER_WEB_HOST", "127.0.0.1")
WEB_PORT = int(os.environ.get("UTILIZER_WEB_PORT", "8889"))
# Empty by default (dev/loopback mode); set to require `?token=` on the WS.
WEB_TOKEN = os.environ.get("UTILIZER_WEB_TOKEN", "")


# WS wire-format models. `monitors.py` returns frozen dataclasses for internal
# use; these Pydantic models are the boundary-only shape sent to the browser
# over JSON, decoupling the wire contract from the collector's value objects.
class MemoryPayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    total: int
    used: int
    available: int
    percent: float


class CpuPayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    percent: float


class GpuStatPayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    utilization: float
    memory_used: int
    memory_total: int
    temperature: int
    name: str
    memory_percent: float


class GpuPayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    available: bool
    gpus: list[GpuStatPayload]


class ProcessPayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    user: str
    pid: str
    cpu: float
    mem: float
    rss: int
    command: str


class SnapshotPayload(BaseModel):
    model_config = ConfigDict(frozen=True)

    timestamp: str
    memory: MemoryPayload
    cpu: CpuPayload
    gpu: GpuPayload
    hanging_count: int
    top_processes: list[ProcessPayload]
    error: str | None = None


async def collect_snapshot() -> SnapshotPayload:
    """各種メトリクスを収集する

    psutil / subprocess のブロッキング呼び出しは別スレッドへ逃がし、
    収集中もイベントループが他の接続をブロックしないようにする。
    """
    memory_info, cpu_info, gpu_info, hanging_count, top_processes = await asyncio.gather(
        asyncio.to_thread(get_memory_info),
        asyncio.to_thread(get_cpu_info),
        asyncio.to_thread(get_gpu_info),
        asyncio.to_thread(get_hanging_processes),
        asyncio.to_thread(get_top_processes, 10),
    )
    return SnapshotPayload(
        timestamp=datetime.now().isoformat(),
        memory=MemoryPayload(
            total=memory_info.total,
            used=memory_info.used,
            available=memory_info.available,
            percent=memory_info.percent,
        ),
        cpu=CpuPayload(percent=cpu_info.percent),
        gpu=GpuPayload(
            available=gpu_info.available,
            gpus=[
                GpuStatPayload(
                    utilization=g.utilization,
                    memory_used=g.memory_used,
                    memory_total=g.memory_total,
                    temperature=g.temperature,
                    name=g.name,
                    memory_percent=g.memory_percent,
                )
                for g in gpu_info.gpus
            ],
        ),
        hanging_count=hanging_count,
        top_processes=[
            ProcessPayload(
                user=p.user,
                pid=p.pid,
                cpu=p.cpu,
                mem=p.mem,
                rss=p.rss,
                command=p.command,
            )
            for p in top_processes
        ],
    )


@app.get("/")
async def index() -> HTMLResponse:
    """メインダッシュボード"""
    html = """
<!DOCTYPE html>
<html>
<head>
    <title>Recap Job Resource Monitor</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        :root {
            --bg-primary: #0a0e27;
            --bg-secondary: #151932;
            --bg-card: #1e2342;
            --bg-card-hover: #252b4d;
            --text-primary: #e8eaf6;
            --text-secondary: #9ca3af;
            --accent-primary: #6366f1;
            --accent-secondary: #8b5cf6;
            --success: #10b981;
            --warning: #f59e0b;
            --danger: #ef4444;
            --border: rgba(255, 255, 255, 0.1);
            --shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
            --shadow-lg: 0 20px 60px rgba(0, 0, 0, 0.5);
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', sans-serif;
            background: var(--bg-primary);
            background-image:
                radial-gradient(at 0% 0%, rgba(99, 102, 241, 0.15) 0%, transparent 50%),
                radial-gradient(at 100% 100%, rgba(139, 92, 246, 0.15) 0%, transparent 50%);
            color: var(--text-primary);
            padding: 24px;
            min-height: 100vh;
            line-height: 1.6;
        }

        .container {
            max-width: 1600px;
            margin: 0 auto;
        }

        h1 {
            color: var(--text-primary);
            text-align: center;
            margin-bottom: 40px;
            font-size: 2.5rem;
            font-weight: 700;
            letter-spacing: -0.02em;
            background: linear-gradient(135deg, var(--accent-primary), var(--accent-secondary));
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
            gap: 24px;
            margin-bottom: 24px;
        }

        .card {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 16px;
            padding: 24px;
            box-shadow: var(--shadow);
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            backdrop-filter: blur(10px);
        }

        .card:hover {
            transform: translateY(-2px);
            box-shadow: var(--shadow-lg);
            border-color: rgba(99, 102, 241, 0.3);
        }

        .card h2 {
            font-size: 0.875rem;
            font-weight: 600;
            margin-bottom: 20px;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .metric {
            font-size: 3rem;
            font-weight: 700;
            color: var(--text-primary);
            line-height: 1.2;
            margin-bottom: 8px;
            font-variant-numeric: tabular-nums;
        }

        .metric-label {
            font-size: 0.875rem;
            color: var(--text-secondary);
            margin-top: 4px;
        }

        .progress-bar {
            width: 100%;
            height: 8px;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 9999px;
            overflow: hidden;
            margin-top: 16px;
            position: relative;
        }

        .progress-fill {
            height: 100%;
            background: linear-gradient(90deg, var(--success), #34d399);
            transition: width 0.5s cubic-bezier(0.4, 0, 0.2, 1);
            border-radius: 9999px;
            position: relative;
            overflow: hidden;
        }

        .progress-fill::after {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.2), transparent);
            animation: shimmer 2s infinite;
        }

        @keyframes shimmer {
            0% { transform: translateX(-100%); }
            100% { transform: translateX(100%); }
        }

        .progress-fill.warning {
            background: linear-gradient(90deg, var(--warning), #fbbf24);
        }

        .progress-fill.danger {
            background: linear-gradient(90deg, var(--danger), #f87171);
        }

        .progress-text {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            font-size: 0.75rem;
            font-weight: 600;
            color: var(--text-primary);
            text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
            z-index: 1;
        }

        .alert {
            background: rgba(245, 158, 11, 0.1);
            border: 1px solid rgba(245, 158, 11, 0.3);
            border-radius: 8px;
            padding: 12px 16px;
            margin-top: 16px;
            color: #fbbf24;
            font-size: 0.875rem;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .alert.danger {
            background: rgba(239, 68, 68, 0.1);
            border-color: rgba(239, 68, 68, 0.3);
            color: #f87171;
        }

        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 16px;
        }

        th, td {
            padding: 12px 16px;
            text-align: left;
            border-bottom: 1px solid var(--border);
        }

        th {
            background: rgba(255, 255, 255, 0.02);
            font-weight: 600;
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: var(--text-secondary);
        }

        td {
            color: var(--text-primary);
            font-size: 0.875rem;
        }

        tr:hover {
            background: rgba(255, 255, 255, 0.02);
        }

        .status {
            display: inline-block;
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: var(--success);
            margin-right: 8px;
            box-shadow: 0 0 8px var(--success);
            animation: pulse 2s infinite;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        .status.disconnected {
            background: var(--danger);
            box-shadow: 0 0 8px var(--danger);
        }

        .status-section {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .status-text {
            font-size: 0.875rem;
            color: var(--text-secondary);
        }

        .last-update {
            margin-top: 12px;
            font-size: 0.75rem;
            color: var(--text-secondary);
        }

        .gpu-card {
            position: relative;
        }

        .gpu-name {
            font-size: 0.75rem;
            color: var(--text-secondary);
            margin-bottom: 8px;
            font-weight: 500;
        }

        .gpu-temp {
            position: absolute;
            top: 24px;
            right: 24px;
            font-size: 0.875rem;
            color: var(--text-secondary);
            display: flex;
            align-items: center;
            gap: 4px;
        }

        .gpu-memory {
            margin-top: 12px;
            font-size: 0.875rem;
            color: var(--text-secondary);
        }

        /* スクロールバーのスタイル */
        ::-webkit-scrollbar {
            width: 8px;
            height: 8px;
        }

        ::-webkit-scrollbar-track {
            background: var(--bg-secondary);
        }

        ::-webkit-scrollbar-thumb {
            background: var(--accent-primary);
            border-radius: 4px;
        }

        ::-webkit-scrollbar-thumb:hover {
            background: var(--accent-secondary);
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>📊 Recap Job Resource Monitor</h1>
        <div class="grid">
            <div class="card">
                <h2>💾 メモリ使用状況</h2>
                <div class="metric" id="mem-used">-</div>
                <div class="metric-label" id="mem-total">-</div>
                <div class="progress-bar">
                    <div class="progress-fill" id="mem-progress" style="width: 0%">
                        <span class="progress-text" id="mem-progress-text">0%</span>
                    </div>
                </div>
                <div class="metric-label" id="mem-available">利用可能: -</div>
            </div>
            <div class="card">
                <h2>⚡ CPU使用率</h2>
                <div class="metric" id="cpu-percent">-</div>
                <div class="metric-label">リアルタイム</div>
                <div class="progress-bar">
                    <div class="progress-fill" id="cpu-progress" style="width: 0%">
                        <span class="progress-text" id="cpu-progress-text">0%</span>
                    </div>
                </div>
            </div>
            <div class="card">
                <h2>🔴 ハングプロセス</h2>
                <div class="metric" id="hanging-count">-</div>
                <div class="metric-label">spawn_main / multiprocessing-fork</div>
                <div id="hanging-alert"></div>
            </div>
        </div>
        <div class="grid" id="gpu-grid" style="display: none;">
            <!-- GPUカードは動的に生成 -->
        </div>
        <div class="card">
            <h2>📈 トッププロセス（メモリ使用量）</h2>
            <table id="process-table">
                <thead>
                    <tr>
                        <th>ユーザー</th>
                        <th>PID</th>
                        <th>CPU%</th>
                        <th>メモリ%</th>
                        <th>RSS (MB)</th>
                        <th>コマンド</th>
                    </tr>
                </thead>
                <tbody id="process-body">
                </tbody>
            </table>
        </div>
        <div class="card">
            <h2>📡 接続状態</h2>
            <div class="status-section">
                <span class="status" id="status-indicator"></span>
                <span class="status-text" id="status-text">接続中...</span>
            </div>
            <div class="last-update">最終更新: <span id="last-update">-</span></div>
        </div>
    </div>

    <script>
        const ws = new WebSocket(`ws://${window.location.host}/ws`);
        const statusIndicator = document.getElementById('status-indicator');
        const statusText = document.getElementById('status-text');
        const lastUpdate = document.getElementById('last-update');

        ws.onopen = () => {
            statusIndicator.classList.remove('disconnected');
            statusText.textContent = '接続中';
        };

        ws.onclose = () => {
            statusIndicator.classList.add('disconnected');
            statusText.textContent = '切断されました';
        };

        ws.onmessage = (event) => {
            let data;
            try {
                data = JSON.parse(event.data);
            } catch (e) {
                console.error('Failed to parse WebSocket data:', e, event.data);
                return;
            }

            // デバッグ用: データが正しく受信されているか確認
            if (!data.top_processes) {
                console.warn('top_processes data missing:', data);
            }

            // メモリ情報
            document.getElementById('mem-used').textContent = `${data.memory.used} GB`;
            document.getElementById('mem-total').textContent = `/ ${data.memory.total} GB`;
            document.getElementById('mem-available').textContent = `利用可能: ${data.memory.available} GB`;
            const memProgress = document.getElementById('mem-progress');
            const memProgressText = document.getElementById('mem-progress-text');
            memProgress.style.width = `${data.memory.percent}%`;
            memProgressText.textContent = `${data.memory.percent}%`;
            memProgress.className = 'progress-fill';
            if (data.memory.percent > 90) {
                memProgress.classList.add('danger');
            } else if (data.memory.percent > 70) {
                memProgress.classList.add('warning');
            } else {
                memProgress.classList.remove('warning', 'danger');
            }

            // CPU情報
            document.getElementById('cpu-percent').textContent = `${data.cpu.percent}%`;
            const cpuProgress = document.getElementById('cpu-progress');
            const cpuProgressText = document.getElementById('cpu-progress-text');
            cpuProgress.style.width = `${data.cpu.percent}%`;
            cpuProgressText.textContent = `${data.cpu.percent}%`;
            cpuProgress.className = 'progress-fill';
            if (data.cpu.percent > 90) {
                cpuProgress.classList.add('danger');
            } else if (data.cpu.percent > 70) {
                cpuProgress.classList.add('warning');
            } else {
                cpuProgress.classList.remove('warning', 'danger');
            }

            // ハングプロセス
            document.getElementById('hanging-count').textContent = `${data.hanging_count}個`;
            const hangingAlert = document.getElementById('hanging-alert');
            if (data.hanging_count > 10) {
                hangingAlert.innerHTML = '<div class="alert danger">⚠ ハングプロセスが多すぎます！</div>';
            } else if (data.hanging_count > 5) {
                hangingAlert.innerHTML = '<div class="alert">⚠ ハングプロセスが増加しています</div>';
            } else {
                hangingAlert.innerHTML = '';
            }

            // GPU情報
            // 注意: gpu.name はホスト側コマンド出力由来の文字列であり、
            // 信頼できない入力として扱う。innerHTML には絶対に連結せず、
            // DOM API + textContent でのみ挿入すること（XSS対策）。
            const gpuGrid = document.getElementById('gpu-grid');
            while (gpuGrid.firstChild) {
                gpuGrid.removeChild(gpuGrid.firstChild);
            }
            if (data.gpu && data.gpu.available && data.gpu.gpus && data.gpu.gpus.length > 0) {
                gpuGrid.style.display = 'grid';
                data.gpu.gpus.forEach((gpu, index) => {
                    const gpuUtilClass = gpu.utilization > 90 ? 'danger' : gpu.utilization > 70 ? 'warning' : '';
                    const memUtilClass = gpu.memory_percent > 90 ? 'danger' : gpu.memory_percent > 70 ? 'warning' : '';

                    const card = document.createElement('div');
                    card.className = 'card gpu-card';

                    const heading = document.createElement('h2');
                    heading.textContent = `🎮 GPU ${index} - ${gpu.name}`;
                    card.appendChild(heading);

                    const temp = document.createElement('div');
                    temp.className = 'gpu-temp';
                    temp.textContent = `🌡️ ${gpu.temperature}°C`;
                    card.appendChild(temp);

                    const utilMetric = document.createElement('div');
                    utilMetric.className = 'metric';
                    utilMetric.id = `gpu-util-${index}`;
                    utilMetric.textContent = `${gpu.utilization}%`;
                    card.appendChild(utilMetric);

                    const utilLabel = document.createElement('div');
                    utilLabel.className = 'metric-label';
                    utilLabel.textContent = '使用率';
                    card.appendChild(utilLabel);

                    const utilBar = document.createElement('div');
                    utilBar.className = 'progress-bar';
                    const utilFill = document.createElement('div');
                    utilFill.className = `progress-fill ${gpuUtilClass}`.trim();
                    utilFill.id = `gpu-progress-${index}`;
                    utilFill.style.width = `${gpu.utilization}%`;
                    const utilFillText = document.createElement('span');
                    utilFillText.className = 'progress-text';
                    utilFillText.textContent = `${gpu.utilization}%`;
                    utilFill.appendChild(utilFillText);
                    utilBar.appendChild(utilFill);
                    card.appendChild(utilBar);

                    const memory = document.createElement('div');
                    memory.className = 'gpu-memory';
                    memory.textContent = `メモリ: ${(gpu.memory_used / 1024).toFixed(1)}GB / ${(gpu.memory_total / 1024).toFixed(1)}GB (${gpu.memory_percent}%)`;
                    card.appendChild(memory);

                    const memBar = document.createElement('div');
                    memBar.className = 'progress-bar';
                    memBar.style.marginTop = '8px';
                    const memFill = document.createElement('div');
                    memFill.className = `progress-fill ${memUtilClass}`.trim();
                    memFill.style.width = `${gpu.memory_percent}%`;
                    const memFillText = document.createElement('span');
                    memFillText.className = 'progress-text';
                    memFillText.textContent = `${gpu.memory_percent}%`;
                    memFill.appendChild(memFillText);
                    memBar.appendChild(memFill);
                    card.appendChild(memBar);

                    gpuGrid.appendChild(card);
                });
            } else {
                gpuGrid.style.display = 'none';
            }

            // プロセス一覧 - 必ず更新されるようにする
            // 注意: user/command はサーバー上の実プロセス情報由来であり、
            // 悪意あるプロセス名/コマンドラインが含まれ得る信頼できない入力。
            // innerHTML には絶対に連結せず、DOM API + textContent でのみ挿入する（XSS対策）。
            const tbody = document.getElementById('process-body');
            if (!tbody) {
                console.error('process-body element not found');
                return;
            }

            while (tbody.firstChild) {
                tbody.removeChild(tbody.firstChild);
            }

            if (data.top_processes && Array.isArray(data.top_processes) && data.top_processes.length > 0) {
                // デバッグ: 受信したデータを確認
                console.debug('Updating process list with', data.top_processes.length, 'processes');

                data.top_processes.forEach(p => {
                    const user = p.user || '-';
                    const pid = p.pid || '-';
                    const cpu = (p.cpu || 0).toFixed(1);
                    const mem = (p.mem || 0).toFixed(1);
                    const rss = p.rss || 0;
                    const command = (p.command || '').substring(0, 50);
                    const commandSuffix = (p.command || '').length > 50 ? '...' : '';

                    const row = document.createElement('tr');
                    [user, pid, `${cpu}%`, `${mem}%`, rss, `${command}${commandSuffix}`].forEach(cellText => {
                        const cell = document.createElement('td');
                        cell.textContent = cellText;
                        row.appendChild(cell);
                    });
                    tbody.appendChild(row);
                });
            } else {
                // データがない場合も明示的に表示
                console.warn('No process data received or empty array');
                const row = document.createElement('tr');
                const cell = document.createElement('td');
                cell.colSpan = 6;
                cell.style.textAlign = 'center';
                cell.style.color = 'var(--text-secondary)';
                cell.textContent = 'プロセス情報を取得中...';
                row.appendChild(cell);
                tbody.appendChild(row);
            }

            // 最終更新時刻
            lastUpdate.textContent = new Date().toLocaleTimeString('ja-JP');
        };

        ws.onerror = () => {
            statusIndicator.classList.add('disconnected');
            statusText.textContent = 'エラーが発生しました';
        };
    </script>
</body>
</html>
    """
    return HTMLResponse(content=html)


@app.websocket("/ws")
async def websocket_endpoint(websocket: WebSocket) -> None:
    """WebSocketエンドポイントでリアルタイムデータを送信"""
    if WEB_TOKEN and websocket.query_params.get("token") != WEB_TOKEN:
        await websocket.close(code=4401)
        return
    await websocket.accept()
    try:
        while True:
            try:
                data = await collect_snapshot()
                await websocket.send_json(data.model_dump(mode="json"))
            except OSError:
                # 収集エラーの詳細はサーバー側ログにのみ残し、クライアントへは
                # 内部情報を含まない汎用メッセージだけを送る。
                logger.exception("Failed to collect monitoring snapshot")
                # エラーが発生しても接続を維持し、空のデータを送信
                fallback = SnapshotPayload(
                    timestamp=datetime.now().isoformat(),
                    memory=MemoryPayload(total=0, used=0, available=0, percent=0),
                    cpu=CpuPayload(percent=0.0),
                    gpu=GpuPayload(available=False, gpus=[]),
                    hanging_count=0,
                    top_processes=[],
                    error="failed to collect monitoring data",
                )
                await websocket.send_json(fallback.model_dump(mode="json"))
            await asyncio.sleep(2)  # 2秒間隔で更新
    except WebSocketDisconnect:
        pass


def main() -> None:
    """Webサーバーを起動"""
    try:
        import uvicorn
    except ImportError:
        print("エラー: uvicornがインストールされていません")
        print("以下のコマンドでインストールしてください:")
        print("  uv sync")
        sys.exit(1)

    try:
        import websockets
    except ImportError:
        print("警告: websocketsがインストールされていません")
        print("WebSocket機能が動作しない可能性があります")
        print("以下のコマンドでインストールしてください:")
        print("  uv sync")

    if WEB_TOKEN:
        logger.info("utilizer_web_auth_enabled: WS connections require ?token=")
    else:
        logger.warning(
            "utilizer_web_auth_disabled: WS is unauthenticated "
            "(set UTILIZER_WEB_TOKEN to require a token)"
        )
    uvicorn.run(app, host=WEB_HOST, port=WEB_PORT)


if __name__ == "__main__":
    main()
