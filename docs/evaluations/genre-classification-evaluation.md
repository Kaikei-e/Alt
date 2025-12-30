# ジャンル分類評価レポート

本ドキュメントは、Alt RSSリーダープラットフォームにおけるジャンル分類モデルの評価結果をまとめたものです。

## 概要

ジャンル分類モデルは、記事を30種類のジャンルに分類します。日本語（JA）と英語（EN）の2言語に対応した Dual Model 構成です。

### 対象ジャンル一覧

| カテゴリ | ジャンル |
|---------|---------|
| テクノロジー | `ai_data`, `consumer_tech`, `cybersecurity`, `internet_platforms`, `software_dev` |
| ビジネス・経済 | `economics_macro`, `markets_finance`, `startups_innovation`, `industry_logistics` |
| 社会・政治 | `politics_government`, `diplomacy_security`, `law_crime`, `society_demographics`, `labor_workplace` |
| 科学 | `life_science`, `health_medicine`, `space_astronomy`, `climate_environment`, `energy_transition` |
| ライフスタイル | `consumer_products`, `food_cuisine`, `home_living`, `travel_places`, `education` |
| エンタメ | `culture_arts`, `film_tv`, `games_esports`, `music_audio`, `sports` |
| その他 | `mobility_automotive` |

---

## 評価指標

### Top-1 評価
最も確信度の高いジャンルのみで評価。単一ラベル分類タスクとしての性能を測定。

### Strict マルチラベル評価
閾値0.5を用いて、全ての正解ラベルとの完全一致（Exact Match）を評価。補助的なサブジャンルの検出能力を測定。

| 指標 | 説明 |
|------|------|
| **Accuracy** | 正解率 |
| **Macro F1** | クラス間平均 F1 スコア |
| **Micro F1** | 全体での F1 スコア |
| **Precision** | 適合率（予測の正確さ） |
| **Recall** | 再現率（正解の網羅度） |

---

## 時系列推移サマリ

### 日本語モデル (JA)

| フェーズ | Top-1 Accuracy | Strict Accuracy | Macro F1 (Top-1) | Macro F1 (Strict) |
|---------|----------------|-----------------|------------------|-------------------|
| Phase 1 | 0.8469 | - | 0.8937 | - |
| Phase 2 | 0.8031 | 0.3854 | 0.8091 | 0.4873 |

### 英語モデル (EN)

| フェーズ | Top-1 Accuracy | Strict Accuracy | Macro F1 (Top-1) | Macro F1 (Strict) |
|---------|----------------|-----------------|------------------|-------------------|
| Phase 1 | 0.9260 | - | 0.9541 | - |
| Phase 2 | 0.9292 | 0.2594 | 0.9222 | 0.2883 |

---

## 詳細評価結果

### 日本語モデル (JA)

#### 基本メトリクス

| 指標 | Top-1 | Strict |
|------|-------|--------|
| Accuracy | 0.8031 | 0.3854 |
| Macro F1 | 0.8091 | 0.4873 |
| Micro F1 | 0.8925 | 0.5524 |
| サンプル数 | 960 | 960 |

#### ジャンル別パフォーマンス（Top-1）

**高パフォーマンス（F1 > 0.90）**:
| ジャンル | Precision | Recall | F1 |
|----------|-----------|--------|------|
| film_tv | 1.0000 | 0.9375 | 0.9677 |
| space_astronomy | 0.9394 | 0.9688 | 0.9538 |
| music_audio | 0.9677 | 0.9375 | 0.9524 |
| games_esports | 0.9118 | 0.9688 | 0.9394 |
| economics_macro | 1.0000 | 0.8750 | 0.9333 |
| sports | 0.8649 | 1.0000 | 0.9275 |

**低パフォーマンス（F1 < 0.85）**:
| ジャンル | Precision | Recall | F1 |
|----------|-----------|--------|------|
| ai_data | 0.7209 | 0.9688 | 0.8267 |
| education | 0.7500 | 0.9375 | 0.8333 |
| consumer_tech | 0.7250 | 0.9062 | 0.8056 |
| cybersecurity | 0.7368 | 0.8750 | 0.8000 |

#### Strict マルチラベル評価での課題

以下のジャンルは Strict 評価で **F1 = 0**（ほぼ検出不能）:

| ジャンル | Precision | Recall | F1 | 原因推定 |
|----------|-----------|--------|------|----------|
| film_tv | 0.0000 | 0.0000 | 0.0000 | 閾値が高すぎる |
| society_demographics | 0.0000 | 0.0000 | 0.0000 | 閾値が高すぎる |

以下のジャンルは **高Precision・低Recall** パターン:

| ジャンル | Precision | Recall | F1 | 分析 |
|----------|-----------|--------|------|------|
| diplomacy_security | 1.0000 | 0.0312 | 0.0606 | 当たるときは正しいが、ほとんど当たらない |
| law_crime | 1.0000 | 0.0625 | 0.1176 | 同上 |
| economics_macro | 1.0000 | 0.1250 | 0.2222 | 同上 |
| health_medicine | 1.0000 | 0.1250 | 0.2222 | 同上 |

---

### 英語モデル (EN)

#### 基本メトリクス

| 指標 | Top-1 | Strict |
|------|-------|--------|
| Accuracy | 0.9292 | 0.2594 |
| Macro F1 | 0.9222 | 0.2883 |
| Micro F1 | 0.9562 | 0.3981 |
| サンプル数 | 960 | 960 |

#### ジャンル別パフォーマンス（Top-1）

**高パフォーマンス（F1 = 1.0）**:
- `consumer_tech`
- `culture_arts`
- `cybersecurity`
- `law_crime`

**低パフォーマンス**:
| ジャンル | Precision | Recall | F1 |
|----------|-----------|--------|------|
| life_science | 1.0000 | 0.5000 | 0.6667 |
| sports | 0.9333 | 0.9655 | 0.9492 |

#### Strict マルチラベル評価での課題

以下のジャンルは Strict 評価で **F1 = 0**:

| ジャンル | Support |
|----------|---------|
| consumer_products | 33 |
| economics_macro | 33 |
| food_cuisine | 35 |
| games_esports | 31 |
| home_living | 33 |
| law_crime | 34 |
| life_science | 34 |
| markets_finance | 31 |
| society_demographics | 28 |
| software_dev | 29 |
| startups_innovation | 30 |
| travel_places | 32 |

---

## 混同傾向の分析

### 意味的に近いジャンル間の混同

1. **Economics & Macro ⇔ Markets & Finance**
   - マクロ経済記事が金融・市場カテゴリに分類される傾向
   - 投資・市場動向とマクロ経済政策の記事が語彙的に近い

2. **社会・治安系カテゴリの混同**
   - `diplomacy_security`, `law_crime` が政治・サイバーセキュリティに吸収される
   - 安全保障・治安に関する記事は複数のトピックと重なりやすい

3. **Games & Esports ⇔ Sports**
   - 日本語モデルで混同が見られる
   - eスポーツと従来のスポーツの境界が曖昧

---

## 提言

### 1. マルチラベル閾値の再設計

**課題**: 一律閾値0.5では多数のジャンルがF1=0

**対策**:
- ジャンルごとのスコア分布を可視化
- **ジャンル別閾値** の設定
- **Top-k ラベル選択**（スコア上位3〜5件を出力）

### 2. 長尾ジャンル向けの補強学習

**対象ジャンル**:
- `film_tv`, `society_demographics`, `economics_macro`, `life_science`, `markets_finance`

**対策**:
- 追加データ収集
- ハードネガティブマイニング

### 3. 階層的分類の導入検討

**アプローチ**:
1. まず「マクロカテゴリ」（経済・金融 / 社会・政治 / 科学・技術 / ライフスタイル）を予測
2. その下で細分類を行う

**効果**: 意味的に近いジャンル間の混同を抑制

### 4. アノテーション品質の確認

- 誤分類サンプルの可視化・レビュー
- ラベル定義の明文化（特に「経済 vs 金融」「社会 vs 教育」など）

---

## ジャンル別パフォーマンス詳細（日本語・Strict）

| ジャンル | Precision | Recall | F1 | Support |
|----------|-----------|--------|------|---------|
| ai_data | 0.5000 | 0.8438 | 0.6279 | 32 |
| climate_environment | 0.9000 | 0.2812 | 0.4286 | 32 |
| consumer_products | 1.0000 | 0.2188 | 0.3590 | 32 |
| consumer_tech | 0.7250 | 0.9062 | 0.8056 | 32 |
| culture_arts | 1.0000 | 0.3125 | 0.4762 | 32 |
| cybersecurity | 0.5660 | 0.9375 | 0.7059 | 32 |
| diplomacy_security | 1.0000 | 0.0312 | 0.0606 | 32 |
| economics_macro | 1.0000 | 0.1250 | 0.2222 | 32 |
| education | 0.3614 | 0.9375 | 0.5217 | 32 |
| energy_transition | 1.0000 | 0.3125 | 0.4762 | 32 |
| film_tv | 0.0000 | 0.0000 | 0.0000 | 32 |
| food_cuisine | 1.0000 | 0.5312 | 0.6939 | 32 |
| games_esports | 1.0000 | 0.5312 | 0.6939 | 32 |
| health_medicine | 1.0000 | 0.1250 | 0.2222 | 32 |
| home_living | 1.0000 | 0.1562 | 0.2703 | 32 |
| industry_logistics | 1.0000 | 0.2500 | 0.4000 | 32 |
| internet_platforms | 1.0000 | 0.2188 | 0.3590 | 32 |
| labor_workplace | 0.8182 | 0.2812 | 0.4186 | 32 |
| law_crime | 1.0000 | 0.0625 | 0.1176 | 32 |
| life_science | 0.9412 | 0.5000 | 0.6531 | 32 |
| markets_finance | 0.4098 | 0.7812 | 0.5376 | 32 |
| mobility_automotive | 0.6531 | 1.0000 | 0.7901 | 32 |
| music_audio | 0.8056 | 0.9062 | 0.8529 | 32 |
| politics_government | 0.5918 | 0.9062 | 0.7160 | 32 |
| society_demographics | 0.0000 | 0.0000 | 0.0000 | 32 |
| software_dev | 0.7619 | 0.5000 | 0.6038 | 32 |
| space_astronomy | 1.0000 | 0.3750 | 0.5455 | 32 |
| sports | 0.6667 | 0.8125 | 0.7324 | 32 |
| startups_innovation | 1.0000 | 0.3125 | 0.4762 | 32 |
| travel_places | 0.7442 | 1.0000 | 0.8533 | 32 |
