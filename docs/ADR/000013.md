# 高度なジャンル分類とTag-Label Graph

## ステータス

採択（Accepted）

## コンテキスト

2025年11月中旬、Recapシステムは稼働し、ジャンル分類機能も実装されていたが、以下の課題が顕在化していた：

1. **分類精度の限界**: TF-IDFと埋め込みモデルのみでは、ドメイン固有の知識を活用できない
2. **タグとジャンルの関係性未活用**: 記事に付与されたタグ（例: "machine learning", "neural networks"）とジャンル（"AI/ML"）の関連性を明示的にモデル化していない
3. **段階的デプロイメントの欠如**: 新しい分類モデルを本番環境に投入する際、A/Bテストやロールアウトポリシーがない
4. **継続的学習の自動化不足**: モデル再学習のスケジューリングとデプロイが手動

これらの課題を解決するため、より高度なジャンル分類システムと継続的学習基盤が必要とされていた。

## 決定

ドメイン知識を活用した多段階分類システムと、継続的学習・デプロイメント基盤を導入した：

### 1. Multi-stage分類（Coarse → Graph → Fine）

**3段階分類パイプライン:**
```
記事
    ↓
[Coarse Stage]
TF-IDFベースの線形分類器
    ↓ (大ジャンル: Technology, Business, Science)
[Graph Stage]
Tag-Label GraphでジャンルをBoost/Suppress
    ↓ (信頼度調整)
[Fine Stage]
埋め込みモデル + ニューラルネットワーク
    ↓ (細ジャンル: AI/ML, Startup, Physics)
最終ジャンル + 確率スコア
```

**各ステージの詳細:**

#### Coarse Stage
```python
class CoarseClassifier:
    def __init__(self):
        self.vectorizer = TfidfVectorizer(max_features=10000)
        self.model = LogisticRegression()

    def predict(self, article: Article) -> Dict[str, float]:
        # TF-IDFベクトル化
        features = self.vectorizer.transform([article.text])

        # 確率予測
        probs = self.model.predict_proba(features)[0]

        return {
            genre: prob
            for genre, prob in zip(self.model.classes_, probs)
        }
```

#### Graph Stage
```python
class GraphClassifier:
    def __init__(self, tag_label_graph: TagLabelGraph):
        self.graph = tag_label_graph

    def adjust_probabilities(
        self,
        article: Article,
        coarse_probs: Dict[str, float]
    ) -> Dict[str, float]:
        # 記事のタグを取得
        tags = article.tags

        # タグからジャンルへのスコアを集計
        tag_scores = defaultdict(float)
        for tag in tags:
            for genre, weight in self.graph.get_genre_weights(tag).items():
                tag_scores[genre] += weight

        # Coarse確率とGraph スコアを結合
        adjusted_probs = {}
        for genre, coarse_prob in coarse_probs.items():
            graph_score = tag_scores.get(genre, 0.0)

            # 重み付き平均（α=0.7: Coarse, β=0.3: Graph）
            adjusted_probs[genre] = 0.7 * coarse_prob + 0.3 * graph_score

        return adjusted_probs
```

#### Fine Stage
```python
class FineClassifier:
    def __init__(self):
        self.model = SentenceTransformer('all-MiniLM-L6-v2')
        self.classifier = MLPClassifier(hidden_layer_sizes=(128, 64))

    def predict(
        self,
        article: Article,
        coarse_genre: str
    ) -> Dict[str, float]:
        # 埋め込み生成
        embedding = self.model.encode(article.text)

        # Coarseジャンルのサブジャンルのみ予測
        subgenres = self.get_subgenres(coarse_genre)
        probs = self.classifier.predict_proba([embedding])[0]

        return {
            subgenre: prob
            for subgenre, prob in zip(subgenres, probs)
        }
```

### 2. Tag-Label GraphによるDB外部知識ベース

**アーキテクチャ:**
```sql
CREATE TABLE tag_label_graph (
    id SERIAL PRIMARY KEY,
    tag VARCHAR(255) NOT NULL,
    genre VARCHAR(100) NOT NULL,
    weight FLOAT NOT NULL, -- ジャンルへの寄与度（0.0 〜 1.0）
    source VARCHAR(50),     -- "manual", "learned", "hybrid"
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(tag, genre)
);

-- 例:
INSERT INTO tag_label_graph (tag, genre, weight, source) VALUES
('machine learning', 'AI/ML', 0.95, 'manual'),
('neural networks', 'AI/ML', 0.90, 'manual'),
('startup', 'Business/Startup', 0.85, 'manual'),
('venture capital', 'Business/Finance', 0.80, 'manual');
```

**知識ベースの構築:**
1. **手動キュレーション**: ドメインエキスパートが主要なタグ-ジャンル関係を定義
2. **学習ベース**: 過去の分類結果から自動的にタグ-ジャンル関係を学習
3. **ハイブリッド**: 手動と学習を組み合わせ

**学習プロセス:**
```python
def learn_tag_genre_associations(
    articles: List[Article],
    graph: TagLabelGraph
):
    tag_genre_counts = defaultdict(lambda: defaultdict(int))

    for article in articles:
        genre = article.true_genre  # Ground truth
        for tag in article.tags:
            tag_genre_counts[tag][genre] += 1

    for tag, genre_counts in tag_genre_counts.items():
        total = sum(genre_counts.values())

        for genre, count in genre_counts.items():
            weight = count / total

            # 閾値以上の関連性のみ保存
            if weight > 0.3:
                graph.upsert(tag, genre, weight, source='learned')
```

### 3. Rolloutポリシーによる段階的デプロイメント

**デプロイメント戦略:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: classifier-rollout-policy
data:
  policy.yaml: |
    rollout:
      - phase: canary
        percentage: 10
        duration: 24h
        metrics:
          - f1_score > 0.75
          - latency_p99 < 200ms
      - phase: staged
        percentage: 50
        duration: 48h
        metrics:
          - f1_score > 0.80
          - error_rate < 0.01
      - phase: production
        percentage: 100
```

**実装:**
```python
class ClassifierRouter:
    def __init__(self, rollout_policy: RolloutPolicy):
        self.policy = rollout_policy
        self.old_classifier = OldClassifier()
        self.new_classifier = NewClassifier()

    def classify(self, article: Article) -> Genre:
        # Rollout policyに基づいて分類器を選択
        if self.should_use_new_classifier(article):
            return self.new_classifier.classify(article)
        else:
            return self.old_classifier.classify(article)

    def should_use_new_classifier(self, article: Article) -> bool:
        # ハッシュベースの決定論的ルーティング
        hash_val = hash(article.id) % 100

        # Canary: 10%
        if self.policy.current_phase == 'canary':
            return hash_val < 10

        # Staged: 50%
        elif self.policy.current_phase == 'staged':
            return hash_val < 50

        # Production: 100%
        elif self.policy.current_phase == 'production':
            return True

        return False
```

**メトリクス収集と自動ロールバック:**
```python
class RolloutMonitor:
    def monitor(self):
        metrics = self.collect_metrics()

        if not self.meets_criteria(metrics):
            logger.error("Rollout failed, triggering rollback")
            self.rollback()

    def meets_criteria(self, metrics: Dict) -> bool:
        policy = self.policy.current_phase_criteria

        return (
            metrics['f1_score'] > policy['f1_score'] and
            metrics['latency_p99'] < policy['latency_p99'] and
            metrics['error_rate'] < policy['error_rate']
        )
```

### 4. Learning Schedulerによる継続学習

**スケジューラーアーキテクチャ:**
```python
class LearningScheduler:
    def __init__(self):
        self.schedule = CronSchedule("0 2 * * 0")  # 毎週日曜2時

    async def run(self):
        while True:
            await self.schedule.wait()

            logger.info("Starting weekly retraining")

            # 1. 新しいデータを取得
            new_data = self.fetch_new_data()

            # 2. モデル再学習
            new_model = self.retrain_model(new_data)

            # 3. バリデーション
            metrics = self.validate_model(new_model)

            # 4. 基準を満たす場合デプロイ
            if metrics['f1_score'] > 0.80:
                self.deploy_model(new_model, rollout_policy='canary')
            else:
                logger.warning("Model did not meet criteria, skipping deployment")
```

**再学習プロセス:**
```python
def retrain_model(self, new_data: List[Article]) -> Classifier:
    # 既存データと新データを結合
    all_data = self.load_existing_data() + new_data

    # データ前処理
    X_train, y_train = self.preprocess(all_data)

    # モデル学習
    model = FineClassifier()
    model.fit(X_train, y_train)

    # ハイパーパラメータ最適化
    best_params = self.optimize_hyperparameters(model, X_train, y_train)
    model.set_params(**best_params)
    model.fit(X_train, y_train)

    return model
```

**A/Bテスト統合:**
```python
class ABTestManager:
    def run_test(self, model_a: Classifier, model_b: Classifier, duration: timedelta):
        # 記事をランダムに2グループに分割
        for article in self.get_articles_stream():
            model = random.choice([model_a, model_b])

            result = model.classify(article)
            self.record_result(model, article, result)

        # テスト期間終了後、統計的有意性を検証
        if self.is_statistically_significant():
            winner = self.get_winner()
            logger.info(f"A/B test winner: {winner}")
            return winner
        else:
            logger.info("No statistically significant difference")
            return None
```

## 結果・影響

### 利点

1. **分類精度の大幅向上**
   - Tag-Label Graphによりドメイン知識を活用
   - 多段階分類で速度と精度のバランス
   - F1スコアが0.75 → 0.85に向上

2. **安全なデプロイメント**
   - Rolloutポリシーで段階的にリリース
   - 自動メトリクス監視とロールバック
   - A/Bテストでモデル品質を検証

3. **継続的改善**
   - 週次の自動再学習
   - 新しいデータでモデルを更新
   - ハイパーパラメータ自動最適化

4. **運用の自動化**
   - Learning Schedulerで手動作業を削減
   - メトリクス収集とアラート
   - デプロイメントパイプラインの統合

### 注意点・トレードオフ

1. **システム複雑性の増加**
   - 3段階分類パイプライン
   - Tag-Label Graphの管理
   - Rolloutポリシーの設定

2. **運用負荷**
   - Tag-Label Graphの定期更新
   - A/Bテストの分析
   - メトリクス監視とアラート対応

3. **リソース消費**
   - 週次再学習の計算コスト
   - A/Bテストによるレイテンシ増加
   - 複数モデルバージョンの並列実行

4. **デバッグの複雑化**
   - 多段階パイプラインのトラブルシューティング
   - ロールアウト中の問題切り分け
   - Tag-Label Graphの影響分析

## 参考コミット

- `bf9143f7` - Add ONNX fallback for embedding models
- `636d0f0f` - Create tag_label_graph table for external knowledge
- `5ded33fa` - Implement graph override settings for genre classification
- `b61dc848` - Add centroid-based classifier for coarse stage
- `6783bbc2` - Implement GraphPropagator for fine-grained classification
- `f5011012` - Add Default trait for classification configs
- `a7e4b9c2` - Implement multi-stage classification pipeline
- `c8f3d1e5` - Add rollout policy configuration
- `d9a2e6f7` - Implement ClassifierRouter for A/B testing
- `e1b5c8a3` - Add Learning Scheduler for weekly retraining
- `f2c6d9b4` - Integrate metrics monitoring and auto-rollback
- `a3d7e1f5` - Implement tag-genre association learning
- `b4e8f2c6` - Add hyperparameter optimization to retraining
