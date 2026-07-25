# Stack Operations

開発者ローカルにおける Alt スタックの起動・停止・再構築・診断の運用境界。
compose/*.yaml が定義する 80+ サービスを、人間が扱える粒度 (Stack) で操作する語彙を固定する。
(2026-07-25 grill にて定義開始)

## Language

**Stack**:
compose ファイル名 stem (`db.yaml` → `db`) と 1:1 対応する、起動・停止の論理単位。
Stack の存在とサービス構成は compose YAML から導出され、手書きの複製を持たない。
_Avoid_: グループ、プロファイル (Compose profiles とは別物)

**Stack Semantics (スタック意味論)**:
YAML から導出できない Stack の性質 — Stack 間依存・GPU 要求・起動タイムアウト・optional 性 — のみを宣言する層。
サービス一覧はここに書かない (導出と宣言の二重管理がドリフトの根本原因)。
_Avoid_: レジストリ (旧ハードコード実装を想起させる)

**altctl**:
開発者ローカル専用のスタックライフサイクル CLI。up / down / rebuild / doctor が責務。
デプロイは責務外。
_Avoid_: デプロイツール

**Ready**:
サービスが利用可能とみなされる状態。healthcheck を持つサービスは healthy、
持たないサービス (distroless 等) は running をもって Ready とする。
`altctl up` の「成功」は対象全サービスの Ready 到達を意味し、それ以外は成功と報告しない。
_Avoid_: started, up (コンテナ起動直後の状態と混同するため)

**Doctor**:
スタックの読み取り専用診断。状態を一切変更せず、unhealthy / 欠落サービスごとに
原因候補と対処コマンド (処方箋) を提示する。
_Avoid_: heal, repair (自動修復を想起させる)

**c2quay**:
Pact ゲート付きデプロイヤ。本番ロールアウトの唯一の正規経路。
_Avoid_: altctl deploy (削除済み — 2026-07-25)
