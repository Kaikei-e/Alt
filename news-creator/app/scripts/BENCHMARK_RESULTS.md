# News-Creator Summary ベンチマーク結果レポート

**計測日時**: 2026-01-02
**環境**: RTX 4060 8GB VRAM, Gemma3:4b Q4_K_M, Ollama

---

## エグゼクティブサマリー

**結論**: 現在の実装は RTX 4060 8GB で**十分最適化されている**。

- 16K contextモデルはすべての目標値をクリア
- 80K contextは8GB VRAMでは実用的でない (Map-Reduce戦略が正解)
- 主要な改善余地は限定的

---

## ベースライン計測結果

| ケース | 入力サイズ | プロンプトトークン | TTFT P50 | Decode tok/s | Total Latency | 評価 |
|--------|-----------|-------------------|----------|--------------|---------------|------|
| small | 484 chars | ~288 | 0.25s | 61.4 | 5.86s | ✅ PASS |
| medium | 1,966 chars | ~658 | 0.56s | 60.5 | 8.34s | ✅ PASS |
| large | 7,085 chars | ~1,938 | 1.23s | 51.3 | 10.96s | ✅ PASS |
| xl (80K) | - | - | - | - | - | ⚠️ VRAM制約 |

### 目標値 vs 実測値

| メトリクス | 目標値 | small | medium | large | 判定 |
|-----------|--------|-------|--------|-------|------|
| TTFT | <2s | 0.25s | 0.56s | 1.23s | ✅ ALL PASS |
| Decode | >40 tok/s | 61.4 | 60.5 | 51.3 | ✅ ALL PASS |
| Prefill | >500 tok/s | 20,346 | 3,990 | 4,046 | ✅ ALL PASS |

---

## ボトルネック分析

### 1. TTFT (Time To First Token)

**パターン**: プロンプトサイズに比例して増加
```
small  (288 tokens):  0.25s - モデルがVRAMにホット、minimal load_duration
medium (658 tokens):  0.56s - プロンプト処理時間増加
large  (1,938 tokens): 1.23s - 目標内に収まる
```

**分析**:
- `load_duration`は最小 (Warmup + keep_alive戦略が効果的)
- `prompt_eval_duration`がTTFTの主因
- Prefill速度は十分高い (3,990-20,346 tok/s)

**結論**: **ボトルネックなし**

### 2. Decode速度

**パターン**: 安定した高速デコード
```
small:  61.4 tok/s
medium: 60.5 tok/s
large:  51.3 tok/s (若干低下)
```

**分析**:
- RTX 4060の期待値 40-50 tok/s を上回る
- 大規模入力でも50 tok/s以上を維持
- `num_batch=1024`の設定が効果的

**結論**: **ボトルネックなし**

### 3. VRAM制約

```
現在の使用量:
- gemma3-4b-16k: ~5.3GB VRAM (size_vram)
- 残り: ~2.9GB (8GB - 5.3GB)

80K context推定:
- KV cache増加: 16K→80K = 5倍
- 追加VRAM: 推定2-3GB+
- 合計: 7-8GB+ (8GB VRAMの限界)
```

**結論**: **80Kは実用的でない**。Map-Reduce戦略 (`HIERARCHICAL_THRESHOLD_CHARS`) の継続が正解。

---

## 改善提案 (優先度順)

### 高優先度: なし

現在の実装は目標値をすべてクリアしており、緊急の改善は不要。

### 中優先度

1. **80K contextの無効化または閾値引き下げ**
   - `HIERARCHICAL_THRESHOLD_CHARS`を150,000→100,000に変更
   - OOMリスクを完全に排除

2. **TTFT監視の追加**
   - `prompt_eval_duration`の閾値アラート (>3s で警告)
   - Grafana/Prometheusメトリクス連携

### 低優先度

1. **QATモデル検証**
   - `gemma-3-4b-it-qat-q4_0`で精度/速度トレードオフ検証
   - 現行Q4_K_Mで十分な性能のため優先度低

2. **Flash Attention有効化**
   - `OLLAMA_FLASH_ATTENTION=1`
   - 効果は限定的と予想 (すでに高速)

---

## 環境詳細

```yaml
GPU: NVIDIA GeForce RTX 4060
VRAM: 8188 MiB
Driver: 575.57.08
CUDA: 12.9

Model: gemma3:4b (Q4_K_M)
Context: 16K / 80K bucket system
Quantization: 4-bit (Q4_K_M)
Model Size: ~3.3GB

Ollama Config:
  num_batch: 1024
  temperature: 0.15
  top_p: 0.85
  repeat_penalty: 1.15
  keep_alive: 24h (16K), 15m (80K)
```

---

## 結論

RTX 4060 8GB + Gemma3:4b の組み合わせは、news-creator Summary機能に**最適**である。

- 16K contextですべての実用的なユースケースをカバー
- Decode速度60 tok/sはGemma3:4bの理論値に近い
- TTFTは十分低く、ユーザー体験を損なわない
- 現在の実装は**本番運用に適している**

改善を行う場合は、80K context無効化とTTFT監視追加を推奨。
