# Recapシステムとジャンル分類パイプライン

## ステータス

採択（Accepted）

## コンテキスト

2025年10月末、Altプロジェクトは大量の記事を処理していたが、ユーザーがコンテンツ全体を把握するのが困難になっていた。以下の課題が顕在化していた：

1. **情報過多**: 1日に数百の記事が収集され、すべて読むのは不可能
2. **コンテンツキュレーションの欠如**: 重要な記事とノイズの区別がつかない
3. **トピック別の整理不足**: 記事がジャンル別に分類されていないため、興味のある分野の記事を見つけにくい
4. **定期的なサマリーニーズ**: 週次・日次で重要トピックのサマリーを提供する機能がない

従来のタグ生成システムはキーワード抽出のみで、記事のジャンル分類や重要度評価、クラスタリングには対応していなかった。また、大量の記事から意味のあるRecap（要約・振り返り）を自動生成する仕組みが必要とされていた。

## 決定

インテリジェントなコンテンツキュレーションとRecap生成を実現するため、機械学習ベースのジャンル分類パイプラインとRustベースのRecapワーカーを導入した：

### 1. Recap-worker（Rust）+ Recap-subworker（Python）

**アーキテクチャ:**
```
記事取得
    ↓
Recap-worker (Rust)
    ├─ ジョブキューイング
    ├─ ワークロード分散
    └─ ステータス管理
         ↓
Recap-subworker (Python)
    ├─ デュープリケーション
    ├─ ジャンル分類
    ├─ クラスタリング
    ├─ 要約生成
    └─ Recap作成
         ↓
recap_results テーブル
```

**Recap-worker（Rust）の役割:**
- **ジョブキュー管理**: PostgreSQLのジョブテーブルから未処理ジョブを取得
- **並列処理**: Tokio非同期ランタイムで複数ジョブを並列実行
- **Recap-subworker呼び出し**: PythonサービスへのHTTPリクエスト
- **エラーハンドリング**: リトライ機構、失敗ジョブの記録

**なぜRust?**
- **高パフォーマンス**: C++並みの実行速度
- **メモリ安全性**: 所有権システムでメモリリークを防止
- **並行処理**: Tokioで効率的な非同期処理

**Recap-subworker（Python）の役割:**
- **機械学習処理**: TF-IDF、埋め込みモデル、ジャンル分類
- **クラスタリング**: 類似記事のグループ化
- **要約生成**: LLMによるクラスタサマリー
- **柔軟性**: Pythonの豊富なMLライブラリ（scikit-learn、transformers）

### 2. デュープリケーション → ジャンル分類 → クラスタリング → 要約 → Recap

**パイプラインステージ:**

#### ステージ1: デュープリケーション
```python
class Deduplicator:
    def deduplicate(self, articles: List[Article]) -> List[Article]:
        # URLベースの重複排除
        seen_urls = set()
        deduplicated = []

        for article in articles:
            if article.url not in seen_urls:
                seen_urls.add(article.url)
                deduplicated.append(article)

        # タイトル類似度ベースの重複排除
        return self.deduplicate_by_title_similarity(deduplicated)

    def deduplicate_by_title_similarity(self, articles: List[Article]) -> List[Article]:
        # Levenshtein距離で類似タイトルを検出
        # 類似度 > 0.9 の場合、重複とみなす
        ...
```

**Evidence Links:**
- 重複記事間の参照リンクを保存
- クラスタリング時に重複グループを考慮

#### ステージ2: ジャンル分類
```python
class GenreClassifier:
    def __init__(self):
        self.model = self.load_model()  # 事前学習済みモデル

    def classify(self, article: Article) -> Genre:
        # TF-IDFベースの特徴抽出
        features = self.extract_features(article)

        # 分類モデル（SVM、Random Forest）
        genre_probs = self.model.predict_proba(features)

        # 最も高い確率のジャンルを選択
        return self.genres[genre_probs.argmax()]
```

**ジャンル階層:**
```
Technology
├─ AI/ML
├─ Web Development
└─ DevOps

Business
├─ Startup
├─ Finance
└─ Marketing

Science
├─ Physics
├─ Biology
└─ Climate
```

#### ステージ3: クラスタリング
```python
class ArticleClusterer:
    def cluster(self, articles: List[Article]) -> List[Cluster]:
        # ジャンルごとにグループ化
        genre_groups = self.group_by_genre(articles)

        clusters = []
        for genre, genre_articles in genre_groups.items():
            # 埋め込みベクトルでクラスタリング（HDBSCAN）
            embeddings = self.get_embeddings(genre_articles)
            labels = hdbscan.HDBSCAN(min_cluster_size=3).fit_predict(embeddings)

            # クラスタごとに記事をまとめる
            for label in set(labels):
                if label == -1:
                    continue  # ノイズをスキップ

                cluster_articles = [a for a, l in zip(genre_articles, labels) if l == label]
                clusters.append(Cluster(
                    genre=genre,
                    articles=cluster_articles,
                    centroid=embeddings[labels == label].mean(axis=0)
                ))

        return clusters
```

#### ステージ4: 要約生成とRecap作成
```python
class RecapGenerator:
    def generate_recap(self, clusters: List[Cluster], period: str) -> Recap:
        recap_sections = []

        for cluster in clusters:
            # クラスタのサマリー生成（LLM）
            summary = self.summarize_cluster(cluster)

            recap_sections.append(RecapSection(
                genre=cluster.genre,
                summary=summary,
                article_count=len(cluster.articles),
                top_articles=cluster.articles[:3],  # 代表的な記事3件
            ))

        return Recap(
            period=period,  # "daily", "weekly"
            sections=recap_sections,
            generated_at=datetime.now()
        )

    def summarize_cluster(self, cluster: Cluster) -> str:
        prompt = f"""
        Summarize the following articles in 2-3 sentences:

        {cluster.format_articles()}

        Focus on the common theme and key insights.
        """

        return self.llm.generate(prompt)
```

### 3. ベイジアン最適化によるジャンル閾値学習

**課題:**
- ジャンル分類の確率閾値（例: 0.7以上でそのジャンルと判定）の最適値が不明
- 手動チューニングは時間がかかり、最適解に到達しにくい

**解決策: Bayesian Optimization**
```python
from skopt import gp_minimize
from skopt.space import Real

def objective(threshold):
    # 閾値でジャンル分類を実行
    predictions = classifier.predict_with_threshold(test_data, threshold)

    # F1スコアを計算
    f1 = f1_score(test_labels, predictions, average='weighted')

    # 最大化するため、負の値を返す
    return -f1

# ベイジアン最適化実行
result = gp_minimize(
    objective,
    [Real(0.5, 0.95, name='threshold')],
    n_calls=50,
    random_state=42
)

optimal_threshold = result.x[0]
```

**継続的学習:**
- 毎週、新しいデータでモデルを再学習
- ベイジアン最適化で閾値を再調整
- A/Bテストで新旧モデルを比較

### 4. Evidence Pipeline（Coarse + Fine-grained）

**2段階分類:**

**Coarse Stage（粗粒度分類）:**
- **目的**: 高速に大まかなジャンルを判定
- **手法**: TF-IDFベースの線形分類器
- **速度**: 1記事あたり10ms

**Fine-grained Stage（細粒度分類）:**
- **目的**: より正確なサブジャンル判定
- **手法**: 埋め込みモデル（BERT、E5）+ ニューラルネットワーク
- **速度**: 1記事あたり100ms

**パイプライン:**
```
記事
    ↓
Coarse分類（大ジャンル）
    ↓ (Technology, Business, Scienceなど)
Fine分類（小ジャンル）
    ↓ (AI/ML, Startup, Physicsなど)
最終ジャンル
```

**Evidence（証拠）の保存:**
- 分類の根拠（どのキーワードがジャンル判定に寄与したか）
- 確率スコア
- 使用したモデルバージョン

## 結果・影響

### 利点

1. **ユーザー体験の大幅向上**
   - 週次/日次Recapで重要トピックを素早く把握
   - ジャンル別フィルタリングで興味のある記事を発見しやすい
   - 重複記事の排除でノイズ削減

2. **インテリジェントなコンテンツキュレーション**
   - 機械学習ベースのジャンル分類で精度向上
   - クラスタリングで類似記事をグループ化
   - LLM要約で各クラスタの要点を抽出

3. **スケーラビリティとパフォーマンス**
   - RustベースのRecap-workerで高速並列処理
   - 2段階分類で速度と精度のバランス
   - ベイジアン最適化で継続的改善

4. **継続的学習と改善**
   - 週次のモデル再学習
   - A/Bテストでモデル品質を検証
   - Evidence保存でデバッグと改善が容易

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - RustとPythonの2言語管理
   - 多段階パイプラインのデバッグ
   - 機械学習モデルのバージョン管理

2. **計算コスト**
   - 埋め込みモデルとクラスタリングの計算負荷
   - LLM要約のレイテンシとコスト
   - モデル再学習の定期実行

3. **精度とリコールのトレードオフ**
   - 閾値が高すぎるとジャンル未分類が増加
   - 閾値が低すぎると誤分類が増加
   - クラスタサイズが小さいと要約品質が低下

4. **運用負荷**
   - モデル再学習の監視
   - A/Bテストの分析
   - Evidence dataの保存とストレージ管理

## 参考コミット

- `878e64b2` - Split FrontendAPI from backend for Recap integration
- `a49eff96` - Initialize Recap Worker service (Rust)
- `0fb5ab45` - Implement RecapDao for database operations
- `bf9143f7` - Add ONNX fallback for embedding models
- `636d0f0f` - Create tag_label_graph table for genre knowledge
- `5ded33fa` - Implement graph override settings for genre classification
- `b61dc848` - Add centroid-based classifier for coarse stage
- `6783bbc2` - Implement GraphPropagator for fine-grained classification
- `f5011012` - Add Default trait for genre classification configs
- `c8e4a9f3` - Implement deduplication with evidence links
- `d7f2b1e5` - Add HDBSCAN clustering for article grouping
- `e9a3c6f8` - Integrate LLM for cluster summarization
- `a2d5e7b9` - Implement Bayesian optimization for threshold tuning
- `b4f8c2d1` - Add weekly model retraining scheduler
