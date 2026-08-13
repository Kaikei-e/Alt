# Kawasima Immutable Data Model — Working Reference

イミュータブルデータモデル (kawasima 提唱) の核を、Alt の event-sourced コードレビューで使う
観点に翻訳する。SKILL.md ワークフロー Step 4 の「resource / event 切り分け」裏取り用。

> 出典: <https://scrapbox.io/kawasima/イミュータブルデータモデル>
> Slideshare: <https://www.slideshare.net/kawasima/ss-40471672>

## Contents

- [中核となる 3 つの主張](#中核となる-3-つの主張)
- [レビューでの使い方](#レビューでの使い方)
- [Cross-entity (R-R / E-E) パターン](#cross-entity-r-r--e-e-パターン)
- [命名アンチパターン](#命名アンチパターン)
- [Alt のレビューにそのまま使うチェック](#alt-のレビューにそのまま使うチェック)

## 中核となる 3 つの主張

1. **Resource と Event を分ける** — Resource は名詞由来で属性に **日時を持たない**。
   Event は動詞由来で属性に **日時を持つ**。業務の記録は event である。
2. **UPDATE を増やすな** — CRUD のうち UPDATE がモデルを最も複雑にする。更新を極限まで削れば、
   拡張に開いて修正に閉じたモデルになる。更新したくなったら、まだ抽出できていない event を疑う。
3. **One fact in one place** — event エンティティに意味の異なる時刻を複数混ぜない。
   event の payload は 1 業務事実。

分類に迷ったら「マスタ / トランザクション」ではなく **日時属性を本質的に持つか** で判定する。
前者は定義が曖昧で議論の種になるだけ、というのが kawasima の指摘。

## レビューでの使い方

### "updated_at を増やしたい" → hidden event のサイン

resource に `updated_at` を生やしたくなる動機は、ほぼ常に**観測されていない event** がそこにある。
例えば `member.updated_at` には、実際には複数の異なる業務遷移が混入している:

- ユーザ自身による情報変更 / オペレータによる強制退会 / 誤退会の復会対応 / メールアドレス確認完了

これらを `MemberInfoChangedByUser` / `MemberForcedDeactivated` / `MemberReactivated` /
`MemberEmailConfirmed` のような独立 event に分解すると、resource 側の `updated_at` は不要になり、
event log が真実源泉になる。

### "XxxUpdated" event を作りたい → 意味の混入

event 名の `Updated` は意味の異なる変更を 1 event に押し込む。「何が起きたか」を述語で書く。

- 悪い: `MemberUpdated`
- 良い: `MemberAddressChanged`, `MemberMembershipUpgraded`, `MemberContactPreferenceChanged`

### Resource はスナップショット

> リソースは、イベントによって引き起こされる属性の変化の一時点でのスナップショット

業務的に計画された更新がない、または更新の event を trace する必要がない場合に限り、
snapshot のみの resource として定義してよい。それ以外は event を抽出して append-only に倒す。

## Cross-entity (R-R / E-E) パターン

- **R-R 交差**: resource 同士を直接つながない。`社員` と `部門` の関係は `配属` (event: 配属日,
  配属理由) によって発生する。「現在の所属部門」は event log から派生する disposable な view。
- **E-E 交差**: event 同士をまとめる関係も別 event で表現する（複数の `受注` を束ねる `請求対応`
  など）。このとき時系列の逆転が起きないよう、順序の不変条件を保つ。

## 命名アンチパターン

kawasima が明示的に避けるべきとする語: 「情報」「データ」「処理」「〜物」「マスタ」「記録」
「管理」、および母音削除など短縮による劣悪な英名。resource か event か判別できなくなる。

## Alt のレビューにそのまま使うチェック

- [ ] resource テーブルに `updated_at` が増えていないか
- [ ] hidden event が抽出されないまま resource を上書きしていないか
- [ ] event 名が `XxxUpdated` のような generic 名になっていないか
- [ ] event payload に複数の意味の異なる業務時刻が同居していないか
- [ ] 2 つの resource を直接 join していて、それを発生させた event がモデル化されていない箇所はないか
- [ ] 命名に「情報」「データ」「処理」「管理」が混じっていないか

[← back to SKILL.md](../SKILL.md)
