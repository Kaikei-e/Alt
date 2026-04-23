# Kawasima Immutable Data Model — Working Reference

イミュータブルデータモデル (kawasima 提唱) の核を、Alt の event-sourced
コードレビューで使う観点に翻訳する。原典は kawasima の Scrapbox / Slideshare
"イミュータブルデータモデル(入門編)"。

> 出典: <https://scrapbox.io/kawasima/イミュータブルデータモデル>
> Slideshare: <https://www.slideshare.net/kawasima/ss-40471672>

## 中核となる 3 つの主張

1. **Resource と Event を分ける**
   - Resource: 名詞由来。属性に **日時を持たない** エンティティ
   - Event: 動詞由来。属性に **日時を持つ** エンティティ
   - 業務の記録は event である

2. **UPDATE を増やすな**
   - CRUD のうち UPDATE がモデルを最も複雑にする
   - 更新を極限まで削れば、拡張に開いて修正に閉じたモデルになる
   - 更新したくなったら、まだ抽出できていない event を疑う

3. **Event は単一の業務時刻属性に寄せる ("One fact in one place")**
   - event エンティティに複数の意味の異なる時刻を混ぜない
   - event の payload は 1 業務事実

## レビューでの使い方

### "updated_at を増やしたい" → hidden event のサイン

resource に updated_at を生やしたくなる動機は、ほぼ常に
**観測されていない event** がそこにある。

例: `member` に `updated_at` を入れる → 実際には複数の異なる業務遷移が
混入している:

- ユーザ自身による情報変更
- オペレータによる強制退会
- 誤退会の復会対応
- メールアドレス確認完了

これらを `MemberInfoChangedByUser` / `MemberForcedDeactivated` /
`MemberReactivated` / `MemberEmailConfirmed` のような独立 event に分解する。
このとき resource 側の `updated_at` は不要になり、event log が真実源泉になる。

### "XxxUpdated" event を作りたい → 意味の混入

event 名に `Updated` を使うと、意味の異なる変更を 1 event に押し込みがち。
「何が起きたか」を述語で書く:

- 悪い: `MemberUpdated`
- 良い: `MemberAddressChanged`, `MemberMembershipUpgraded`,
  `MemberContactPreferenceChanged`

## Cross-entity (R-R / E-E) パターン

### R-R 交差: resource 同士をつなぐのは event

`社員` (resource) と `部門` (resource) を直接つないではいけない。
両者の関係性は `配属` (event) によって発生する。

```
社員 ─── 配属 (event: 配属日, 配属理由) ─── 部門
```

projection で「現在の所属部門」を出したい場合、それは event log から
派生する **disposable な view**。

### E-E 交差: event 同士をつなぐ独立した event

`受注` と `請求` のように、複数 event をまとめる関係も別 event で表現:

```
受注 (event)  ←─┐
                 ├── 請求対応 (event: 対応日)
受注 (event)  ←─┘
```

**重要**: 時系列の逆転が起きないように設計する。請求は受注より後の時刻、
など順序の不変条件を保つ。

## 命名アンチパターン

kawasima が明示的に避けるべきとしている語:

- 「情報」「データ」「処理」
- 「〜物」「マスタ」「記録」「管理」
- 母音削除など短縮による劣悪な英名

これらは意味を曖昧にし、resource か event か判別できなくする。

## マスタ / トランザクション分類との違い

kawasima は「マスタ / トランザクション」分類は定義が曖昧で議論の種になる、と
指摘する。一方「resource / event」は **日時属性を持つかどうか** という
明快な基準で分類できる。

レビューで「これはマスタっぽい」「トランザクション的だ」と言いたくなったら、
代わりに「日時を本質的に持つか」で判定する。

## Resource はスナップショット

> リソースは、イベントによって引き起こされる属性の変化の一時点での
> スナップショット

業務的に計画された更新がない、または更新の event を trace する必要がない
場合に限り、snapshot のみの resource として定義してよい。それ以外は
event を抽出して append-only に倒す。

## Alt のレビューにそのまま使うチェック

- [ ] resource テーブルに `updated_at` が増えていないか
- [ ] hidden event が抽出されないまま resource を上書きしていないか
- [ ] event 名が `XxxUpdated` のような generic 名になっていないか
- [ ] event payload に複数の意味の異なる業務時刻が同居していないか
- [ ] 2 つの resource を直接 join してしまい、それを発生させた event が
      モデル化されていない箇所はないか
- [ ] 「マスタ / トランザクション」で考えそうになったとき、
      日時属性で resource / event に再分類できるか
- [ ] 命名に「情報」「データ」「処理」「管理」が混じっていないか

## 参考文献

- 佐藤正美『データベース設計論 T字型ER』
- 羽生章洋『楽々ERDレッスン』
- kawasima のアンチパターン記事群 (Scrapbox)

[← back to SKILL.md](../SKILL.md)
