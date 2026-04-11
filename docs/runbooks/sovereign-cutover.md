# Knowledge Sovereign Cutover Runbook

## 参照

- [[000532]] Knowledge Sovereign を durable knowledge state の owner として新設する
- `docs/plan/involve/cutover_readiness_checklist.md`
- `docs/plan/involve/sovereign_db_ownership.md`

## 前提条件

`CutoverReadinessUsecase` が `OverallReady == true` を返すこと。

```bash
altctl sovereign cutover-readiness
```

## Cutover 手順

### Step 1: 最終 replay catch-up

```bash
knowledge-sovereign/cmd/migrate catch-up \
  --source-db=alt-db \
  --target-db=knowledge-sovereign-db
```

### Step 2: Shadow compare

```bash
knowledge-sovereign/cmd/migrate verify \
  --source-db=alt-db \
  --target-db=knowledge-sovereign-db \
  --threshold-item-drift=0.05
```

全指標がしきい値内であることを確認。

### Step 3: Freeze（event append 一時停止）

```bash
altctl sovereign freeze --confirm
```

alt-backend の Sovereign write path を一時停止。Home API は read-only で継続動作。

### Step 4: 最終 catch-up（freeze 中）

```bash
knowledge-sovereign/cmd/migrate catch-up \
  --source-db=alt-db \
  --target-db=knowledge-sovereign-db
```

freeze 中の差分は数秒分のみ。

### Step 5: Active writer 切り替え

```bash
# 環境変数を切り替え
SOVEREIGN_WRITER_ACTIVE=true

# alt-backend を再起動
docker compose -f compose/compose.yaml -p alt restart alt-backend
```

### Step 6: Post-cutover compare

```bash
knowledge-sovereign/cmd/migrate verify \
  --source-db=alt-db \
  --target-db=knowledge-sovereign-db \
  --threshold-item-drift=0
```

### Step 7: Freeze 解除

```bash
altctl sovereign unfreeze --confirm
```

### Step 8: Rollback 窓（30分〜1時間）

monitoring で以下を監視:
- `alt.sovereign.mutation.duration_ms` — レイテンシ劣化がないこと
- `alt.sovereign.reconciliation.mismatch_total` — mismatch が 0 であること
- Home API / Recall / Lens の正常動作

### Step 9: Rollback 窓終了

問題がなければ旧 writer を無効化。

## Rollback 手順

### Rollback 窓内

```bash
# 1. writer を旧パスに戻す
SOVEREIGN_WRITER_ACTIVE=false
docker compose -f compose/compose.yaml -p alt restart alt-backend

# 2. knowledge-sovereign service を停止
docker compose -f compose/compose.yaml -f compose/sovereign.yaml -p alt stop knowledge-sovereign

# 3. 差分を alt-db に replay（必要な場合）
knowledge-sovereign/cmd/migrate reverse-catch-up \
  --source-db=knowledge-sovereign-db \
  --target-db=alt-db
```

### Rollback 窓終了後

- dual-path コードが削除済みの場合、git revert が必要
- DB state は knowledge-sovereign-db に authority があるため、alt-db への逆 replay が必要
- このシナリオは最悪ケース。発生確率は極めて低い
