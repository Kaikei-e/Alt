# 包括的可観測性とAdmin Dashboard

## ステータス

採択（Accepted）

## コンテキスト

2025年12月上旬、Altプロジェクトは高度な機能（RAG、Recap、ジャンル分類）を備えた本番環境で稼働していたが、運用の観点から以下の課題が顕在化していた：

1. **可視性の不足**: 複雑なパイプライン（Recap、分類、クラスタリング）の内部状態が見えない
2. **デバッグの困難性**: 処理失敗時に、どのステージで何が起きたかを追跡できない
3. **パフォーマンス監視の欠如**: CPU、GPU、メモリ使用率のリアルタイム監視ができない
4. **アドホックジョブの管理**: 再分類、再クラスタリングなどのアドホック操作を実行する手段がない
5. **ログ分析の非効率性**: ClickHouseにログが蓄積されているが、分析ツールが不足

特に、機械学習パイプラインの運用では、ジョブの状態、分類精度、モデルバージョンなどを可視化し、問題を素早く特定する能力が不可欠であった。

## 決定

本番環境の運用可視性とトラブルシューティング能力を向上させるため、包括的な可観測性基盤とAdmin Dashboardを導入した：

### 1. Streamlitベースのメトリクス可視化

**なぜStreamlit?**
- **Pythonネイティブ**: 既存のデータ分析コードをそのまま活用
- **迅速なプロトタイピング**: 数行のコードでダッシュボード作成
- **インタラクティブ**: ウィジェット、グラフ、テーブルを動的に表示

**Dashboard実装:**
```python
import streamlit as st
import psycopg2
import pandas as pd
import plotly.express as px

# ページ設定
st.set_page_config(
    page_title="Alt Admin Dashboard",
    layout="wide"
)

# サイドバー
st.sidebar.title("Navigation")
page = st.sidebar.radio("Select Page", [
    "Recap Overview",
    "Classification Metrics",
    "System Metrics",
    "Job Queue",
])

if page == "Recap Overview":
    st.title("Recap System Overview")

    # 直近のRecapジョブ
    jobs = fetch_recent_recap_jobs()
    st.dataframe(jobs)

    # ジョブステータス分布
    fig = px.pie(jobs, names='status', title='Job Status Distribution')
    st.plotly_chart(fig)

elif page == "Classification Metrics":
    st.title("Classification Metrics")

    # ジャンル別の分類精度
    metrics = fetch_genre_metrics()

    col1, col2 = st.columns(2)
    with col1:
        st.metric("Overall F1 Score", f"{metrics['f1_score']:.3f}")
    with col2:
        st.metric("Accuracy", f"{metrics['accuracy']:.3f}")

    # ジャンル別のF1スコア
    fig = px.bar(metrics['per_genre'], x='genre', y='f1_score', title='F1 Score by Genre')
    st.plotly_chart(fig)

elif page == "System Metrics":
    st.title("System Metrics")

    # リアルタイムメトリクス
    cpu, gpu, memory = fetch_system_metrics()

    col1, col2, col3 = st.columns(3)
    with col1:
        st.metric("CPU Usage", f"{cpu}%")
    with col2:
        st.metric("GPU Usage", f"{gpu}%")
    with col3:
        st.metric("Memory Usage", f"{memory}%")

    # 時系列グラフ
    history = fetch_metrics_history()
    fig = px.line(history, x='timestamp', y=['cpu', 'gpu', 'memory'],
                  title='Resource Usage Over Time')
    st.plotly_chart(fig)
```

**ダッシュボード機能:**
- **Recap Overview**: ジョブ一覧、ステータス分布、処理時間
- **Classification Metrics**: ジャンル別精度、混同行列、時系列推移
- **System Metrics**: CPU/GPU/メモリ使用率、プロセス一覧
- **Job Queue**: 処理待ちジョブ、失敗ジョブの詳細

### 2. recap_system_metricsテーブル

**スキーマ:**
```sql
CREATE TABLE recap_system_metrics (
    id SERIAL PRIMARY KEY,
    job_id VARCHAR(100) NOT NULL,
    stage VARCHAR(50) NOT NULL, -- "deduplication", "classification", "clustering", "summarization"
    metric_name VARCHAR(100) NOT NULL,
    metric_value FLOAT NOT NULL,
    metadata JSONB,
    recorded_at TIMESTAMP DEFAULT NOW(),
    INDEX idx_job_stage (job_id, stage),
    INDEX idx_recorded_at (recorded_at DESC)
);
```

**メトリクス例:**
```sql
-- デュープリケーションステージ
INSERT INTO recap_system_metrics (job_id, stage, metric_name, metric_value, metadata) VALUES
('job_abc123', 'deduplication', 'articles_processed', 1000, '{"duplicates_found": 150}'),
('job_abc123', 'deduplication', 'processing_time_ms', 2500, NULL);

-- 分類ステージ
INSERT INTO recap_system_metrics (job_id, stage, metric_name, metric_value, metadata) VALUES
('job_abc123', 'classification', 'f1_score', 0.85, '{"model": "ensemble_v2"}'),
('job_abc123', 'classification', 'articles_classified', 850, NULL);

-- クラスタリングステージ
INSERT INTO recap_system_metrics (job_id, stage, metric_name, metric_value, metadata) VALUES
('job_abc123', 'clustering', 'clusters_created', 12, '{"algorithm": "hdbscan"}'),
('job_abc123', 'clustering', 'silhouette_score', 0.72, NULL);
```

**クエリ例:**
```sql
-- ジョブ全体のパフォーマンス
SELECT
    stage,
    AVG(metric_value) AS avg_time
FROM recap_system_metrics
WHERE metric_name = 'processing_time_ms'
    AND recorded_at > NOW() - INTERVAL '7 days'
GROUP BY stage
ORDER BY avg_time DESC;

-- 時系列での分類精度
SELECT
    DATE_TRUNC('day', recorded_at) AS day,
    AVG(metric_value) AS avg_f1_score
FROM recap_system_metrics
WHERE metric_name = 'f1_score'
    AND recorded_at > NOW() - INTERVAL '30 days'
GROUP BY day
ORDER BY day;
```

### 3. Admin Jobs Queue（非同期ジョブ管理）

**ユースケース:**
- 特定期間の記事を再分類
- クラスタリングパラメータを変更して再実行
- ジャンルモデルの手動トリガー
- 失敗したジョブの再試行

**スキーマ:**
```sql
CREATE TABLE admin_jobs (
    id SERIAL PRIMARY KEY,
    job_type VARCHAR(50) NOT NULL, -- "reclassify", "recluster", "retrain"
    parameters JSONB NOT NULL,
    status VARCHAR(20) DEFAULT 'pending', -- "pending", "running", "completed", "failed"
    created_by VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW(),
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    result JSONB
);
```

**ジョブ実行フロー:**
```python
class AdminJobExecutor:
    def execute_job(self, job: AdminJob):
        try:
            job.status = 'running'
            job.started_at = datetime.now()
            job.save()

            if job.job_type == 'reclassify':
                result = self.reclassify_articles(job.parameters)
            elif job.job_type == 'recluster':
                result = self.recluster_articles(job.parameters)
            elif job.job_type == 'retrain':
                result = self.retrain_model(job.parameters)

            job.status = 'completed'
            job.completed_at = datetime.now()
            job.result = result
            job.save()

        except Exception as e:
            job.status = 'failed'
            job.error_message = str(e)
            job.save()
```

**Streamlit UI:**
```python
st.title("Admin Jobs")

# 新規ジョブ作成
with st.form("create_job"):
    job_type = st.selectbox("Job Type", ["reclassify", "recluster", "retrain"])

    if job_type == "reclassify":
        start_date = st.date_input("Start Date")
        end_date = st.date_input("End Date")
        parameters = {"start_date": str(start_date), "end_date": str(end_date)}

    submitted = st.form_submit_button("Create Job")
    if submitted:
        create_admin_job(job_type, parameters)
        st.success("Job created successfully!")

# ジョブ一覧
jobs = fetch_admin_jobs()
st.dataframe(jobs)
```

### 4. Utilizer（リアルタイムCPU/GPUモニタリング）

**実装:**
```python
import psutil
import GPUtil

class SystemUtilizer:
    @staticmethod
    def get_cpu_usage() -> float:
        return psutil.cpu_percent(interval=1)

    @staticmethod
    def get_memory_usage() -> dict:
        mem = psutil.virtual_memory()
        return {
            'total': mem.total,
            'used': mem.used,
            'percent': mem.percent
        }

    @staticmethod
    def get_gpu_usage() -> list:
        gpus = GPUtil.getGPUs()
        return [{
            'id': gpu.id,
            'name': gpu.name,
            'load': gpu.load * 100,
            'memory_used': gpu.memoryUsed,
            'memory_total': gpu.memoryTotal,
        } for gpu in gpus]

    @staticmethod
    def get_process_info() -> list:
        processes = []
        for proc in psutil.process_iter(['pid', 'name', 'cpu_percent', 'memory_percent']):
            try:
                processes.append(proc.info)
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                pass

        # CPU使用率でソート
        processes.sort(key=lambda p: p['cpu_percent'], reverse=True)
        return processes[:20]  # トップ20
```

**定期収集とデータベース保存:**
```python
async def collect_metrics_periodically():
    while True:
        cpu = SystemUtilizer.get_cpu_usage()
        memory = SystemUtilizer.get_memory_usage()
        gpu = SystemUtilizer.get_gpu_usage()

        # recap_system_metricsに保存
        save_metrics({
            'cpu_percent': cpu,
            'memory_percent': memory['percent'],
            'gpu_load': gpu[0]['load'] if gpu else 0,
        })

        await asyncio.sleep(60)  # 1分ごと
```

### 5. SQLベースのログ解析

**ClickHouseクエリ例:**

**エラー率の時系列:**
```sql
SELECT
    toStartOfHour(timestamp) AS hour,
    countIf(level = 'ERROR') AS error_count,
    count() AS total_count,
    (error_count / total_count) * 100 AS error_rate
FROM logs
WHERE timestamp > NOW() - INTERVAL 24 HOUR
GROUP BY hour
ORDER BY hour;
```

**サービス別のレイテンシ:**
```sql
SELECT
    service_name,
    quantile(0.5)(latency_ms) AS p50,
    quantile(0.95)(latency_ms) AS p95,
    quantile(0.99)(latency_ms) AS p99
FROM logs
WHERE timestamp > NOW() - INTERVAL 1 HOUR
    AND latency_ms IS NOT NULL
GROUP BY service_name
ORDER BY p99 DESC;
```

**頻出エラーメッセージ:**
```sql
SELECT
    message,
    count() AS occurrence
FROM logs
WHERE level = 'ERROR'
    AND timestamp > NOW() - INTERVAL 7 DAY
GROUP BY message
ORDER BY occurrence DESC
LIMIT 10;
```

**Streamlit統合:**
```python
st.title("Log Analysis")

# エラー率グラフ
error_rate = fetch_error_rate()
fig = px.line(error_rate, x='hour', y='error_rate', title='Error Rate Over Time')
st.plotly_chart(fig)

# レイテンシ分布
latency = fetch_latency_distribution()
st.dataframe(latency)

# 頻出エラー
errors = fetch_frequent_errors()
st.table(errors)
```

## 結果・影響

### 利点

1. **可視性の大幅向上**
   - Streamlit Dashboardでパイプライン全体を可視化
   - リアルタイムメトリクスで問題を即座に発見
   - ジョブの詳細な追跡とデバッグ

2. **運用効率の改善**
   - Admin Jobs Queueでアドホック操作を簡単に実行
   - SQLベースのログ解析で深い洞察
   - CPU/GPUモニタリングでリソース最適化

3. **問題解決の高速化**
   - エラーログとメトリクスの統合表示
   - 時系列グラフで異常検出
   - ジョブ履歴から再現性のある問題を特定

4. **データ駆動の意思決定**
   - 分類精度の時系列推移
   - A/Bテスト結果の可視化
   - システムパフォーマンスの継続監視

### 注意点・トレードオフ

1. **運用負荷**
   - Streamlit Dashboardの保守
   - メトリクステーブルのストレージ管理
   - Admin Jobsの監視と失敗対応

2. **リソース消費**
   - Streamlitの実行コスト
   - 定期的なメトリクス収集
   - ClickHouseへのクエリ負荷

3. **セキュリティリスク**
   - Admin Dashboardへのアクセス制御
   - Admin Jobsの実行権限管理
   - ログに含まれる機密情報

4. **複雑性の増加**
   - 複数の監視ツール（Streamlit、ClickHouse、メトリクステーブル）
   - ダッシュボードのカスタマイズとメンテナンス
   - メトリクススキーマの進化管理

## 参考コミット

- `07e6dde1` - Refactor dashboard for improved metrics visualization
- `9d559f81` - Add Procps for process monitoring
- `c1c46b94` - Implement Admin jobs queue for asynchronous operations
- `a7e4b9c2` - Create recap_system_metrics table for pipeline metrics
- `d9a2e6f7` - Implement Utilizer for real-time CPU/GPU monitoring
- `e1b5c8a3` - Integrate Streamlit dashboard with PostgreSQL and ClickHouse
- `f2c6d9b4` - Add SQL-based log analysis queries
- `a3d7e1f5` - Implement job tracking and detailed logging
- `b4e8f2c6` - Add error rate and latency visualization
- `c5f9a3d7` - Integrate metrics collection with admin jobs
- `d6e1b4c8` - Add authentication for admin dashboard
- `e7f2c5a9` - Implement alert system for anomaly detection
