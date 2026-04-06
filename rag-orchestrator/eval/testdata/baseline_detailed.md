# Augur Baseline Eval Report

**Timestamp:** 2026-04-06T09:52:39Z

**Cases:** 12 | **Pass:** 1 | **Fail:** 11 | **Pass Rate:** 8%

---

## Aggregate Metrics

### Retrieval

| Metric | Value |
|--------|-------|
| Mean Recall@20 | 0.000 |
| Mean nDCG@10 | 0.000 |
| Mean Top-1 Precision | 0.000 |
| BM25 Zero Rate | 1.000 |

### Planning

| Metric | Value |
|--------|-------|
| Intent Accuracy | 0.286 |
| Clarification Precision | 0.000 |
| Follow-up Resolution Rate | 0.000 |

### Generation

| Metric | Value |
|--------|-------|
| Mean Faithfulness | 0.000 |
| Mean Citation Correctness | 0.000 |
| Unsupported Claim Rate | 0.000 |
| Fallback Rate | 0.000 |

---

## Per-Case Results

### FAIL `iran-oil-crisis-causal`

**Query:** イランの石油危機はなぜ起きた？

**Tags:** causal, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- min relevant contexts: got 0, want >= 2
- intent: got "", want "causal_explanation"
- answer length: got 0 runes, want >= 800
- citations required but none provided
- expected entity "イラン" not found in answer
- expected entity "石油" not found in answer

---

### FAIL `iran-follow-up-developments`

**Query:** では、それに関連するイランの動向は？

**Conversation History:**

- **user:** 最近の石油危機の真因は？
- **assistant:** 最近の石油危機は、複数の産油国の減産方針と地政学的緊張の高まりが複合的に作用して発生しました。

**Tags:** follow-up, cjk, coreference, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- answer length: got 0 runes, want >= 300
- citations required but none provided
- expected entity "イラン" not found in answer

---

### FAIL `ambiguous-more-detail`

**Query:** もっと詳しく

**Conversation History:**

- **user:** 最近のAIチップ開発の動向は？
- **assistant:** NVIDIAのBlackwellアーキテクチャが発表され、推論性能が大幅に向上しました。

**Tags:** follow-up, ambiguous, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- expected clarification but none was asked

---

### FAIL `ambiguous-is-that-true`

**Query:** それって本当？

**Conversation History:**

- **user:** EVバッテリーの寿命は延びている？
- **assistant:** 最新の全固体電池技術により、バッテリー寿命は従来比2倍に延びています。

**Tags:** follow-up, fact-check, coreference

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- intent: got "", want "fact_check"
- answer length: got 0 runes, want >= 200
- citations required but none provided

---

### PASS `oil-crisis-root-cause`

**Query:** 最近の石油危機の真因は？

**Tags:** causal, cjk, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 1512 runes |
| Citations | 6 |
| Fallback | false |
| Retrieved Titles | Oil prices hit seven-month highs as tensions rise before US-Iran talks; Trump’s re-election may have helped Albanese – but the US war in Iran is creating economic conundrums; Middle East crisis live: More than 100 children killed by Israeli strikes in Lebanon, health ministry says; How escalating Iran conflict is driving up oil and gas prices – a visual guide; Australia news live: PM confirms deal to keep Tomago aluminium smelter open; Chalmers clears way for Korean firm to up its stake in defence shipbuilder  | Australia news | The Guardian; Pessimism takes root in UK as shoppers struggle to afford essentials | UK cost of living crisis | The Guardian; Lloyd’s of London stresses it is still insuring shipping in strait of Hormuz |
| Cited Titles | Oil prices hit seven-month highs as tensions rise before US-Iran talks; Trump’s re-election may have helped Albanese – but the US war in Iran is creating economic conundrums; Middle East crisis live: More than 100 children killed by Israeli strikes in Lebanon, health ministry says; How escalating Iran conflict is driving up oil and gas prices – a visual guide; Pessimism takes root in UK as shoppers struggle to afford essentials | UK cost of living crisis | The Guardian; Lloyd’s of London stresses it is still insuring shipping in strait of Hormuz |

<details><summary>Answer (preview)</summary>

```
## 概要
最近の石油価格高騰やエネルギー危機を引き起こしている要因は、単一の原因ではなく、複数の要因が複雑に絡み合っていると分析されます。最も直接的なトリガーとしては、米国とイラン間の緊張の高まりが挙げられ、これが市場の懸念を煽っています。しかし、背景には、地域的な紛争の拡大や、特定の戦略的要衝での供給リスクが高まっている構造的な問題が存在します。特に、ホルムズ海峡での輸送の停滞や精製所への攻撃といった物理的な供給懸念が、価格上昇の主要因となっています。

## 詳細
最近のエネルギー市場の動向は、複数の要因から構成される複雑な因果関係を示しています。これらを「直接的要因」「構造的背景」「不確実性」の三つの側面から分析します。

### 1. 直接的要因：米イラン間の緊張の高まりと地域紛争の激化

石油価格が直近で7ヶ月ぶりの高水準に達した背景には、米国とイランの間で差し迫った緊張が高まっていることが直接的な引き金となっています[1]。この地域情勢の悪化は、市場参加者の間で懸念を引き起こし、価格を押し上げています[1]。さらに、米国による中東への介入（Middle East incu...
```

</details>

---

### FAIL `simple-temporal-query`

**Query:** 今週のAI関連ニュースは？

**Tags:** temporal, cjk

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- intent: got "", want "temporal"
- answer length: got 0 runes, want >= 300
- citations required but none provided

---

### FAIL `comparison-query`

**Query:** GPT-4oとClaude 3.5の違いは？

**Tags:** comparison, cjk

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- intent: got "", want "comparison"
- answer length: got 0 runes, want >= 500
- citations required but none provided

---

### FAIL `synthesis-broad-topic`

**Query:** そもそもニューヨークと芸術のかかわりは？

**Tags:** synthesis, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 1540 runes |
| Citations | 7 |
| Fallback | false |
| Retrieved Titles | High and low: the spontaneous joy and drama of New York City – in pictures; Meet the Speakerhead Wiring the Art World for Sound; Venezuelan Artists Speak Out; A View From the Easel; Hard Truths: Can an Artist Dally With Dubious Funding and Come Out Clean on the Other Side?; Required Reading; A View From the Easel |
| Cited Titles | High and low: the spontaneous joy and drama of New York City – in pictures; Meet the Speakerhead Wiring the Art World for Sound; Venezuelan Artists Speak Out; A View From the Easel; Hard Truths: Can an Artist Dally With Dubious Funding and Come Out Clean on the Other Side?; Required Reading; A View From the Easel |

**Failures:**

- intent: got "general", want "synthesis"

<details><summary>Answer (preview)</summary>

```
## 概要
ニューヨークと芸術の関係は、その「無限の刺激」「歴史的背景」「多様なコミュニティの形成」によって特徴づけられています。この都市は、芸術家たちにとって常に刺激的で、時には生存をかけた闘いの場でありながら、才能を育み、展示の場を提供するダイナミックな環境となっています。

## 詳細
ニューヨークは、芸術家たちにとって「尽きることのない謎と視覚的な刺激」を提供する場所として描かれています[7]。この環境は、単なる創造の場に留まらず、歴史や新たな経験が積み重なる「レガシー」を形成しています[7]。

**1. 芸術活動とコミュニティの側面**

*   **コミュニティの結集と支援:** ニューヨークには、特定の芸術分野に特化したコミュニティが存在します。例えば、ある地域では「新世代のニューヨークを拠点とする彫刻家たち」に捧げられたグループショーが開かれた記録があります[2]。また、特定の地域（例：Woodstock、Ulster County、Hudson Valley）には、アーティストが集まる大きな芸術コミュニティが存在し、関連する活動が行われています[4]。
*   **...
```

</details>

---

### FAIL `topic-shift-explicit`

**Query:** 別件だけど、最新の半導体市場の動向は？

**Conversation History:**

- **user:** EVバッテリーの技術は？
- **assistant:** 全固体電池の実用化が進んでいます。

**Tags:** follow-up, topic-shift

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- answer length: got 0 runes, want >= 200
- citations required but none provided

---

### FAIL `cjk-short-query-bm25`

**Query:** 量子コンピュータ 実用化

**Tags:** cjk, bm25, short-query

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- min relevant contexts: got 0, want >= 1
- citations required but none provided

---

### FAIL `deep-dive-technical`

**Query:** Transformerアーキテクチャのattention機構について詳しく

**Tags:** deep-dive, technical

| Field | Value |
|-------|-------|
| Answer Length | 640 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
提供されたコンテキスト情報だけでは、ユーザーの質問に対する包括的かつ詳細な回答を生成することができません。回答に必要な具体的な情報（コンテキスト）が提供されていないため、分析や解説を行うことが不可能です。

## 詳細
本タスクでは、優秀なリサーチアナリストとして、提供されたコンテキスト情報を最大限に活用し、ユーザーの質問に対して包括的で詳細な回答を生成することが求められています。回答は、結論を最初に述べ、その後で根拠と詳細を説明するという構造に従う必要があります。また、回答は日本語で記述し、800文字以上を目指す必要があります。最も重要な点として、回答の根拠となる全ての情報は、提供されたコンテキスト情報のみに基づいて記述し、ソース引用（[番号]形式）を付与することが必須です。

しかしながら、今回のリクエストには、分析対象となる「Context」が空であるか、または提供されていない状態です。そのため、具体的な事実、データ、事例を引用することができず、回答の核となる部分を構成することができません。コンテキスト情報が欠如しているため、指定された品質基準（特に「コンテキストの情...
```

</details>

---

### FAIL `follow-up-different-perspective`

**Query:** 別の観点では？

**Conversation History:**

- **user:** 生成AIの著作権問題は？
- **assistant:** 著作権法の観点からは、学習データの利用がフェアユースに該当するか議論が続いています。

**Tags:** follow-up, ambiguous

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | false |

**Failures:**

- answer length: got 0 runes, want >= 300
- citations required but none provided

---

## Analysis

### Systemic Issues

| Issue | Count | Impact |
|-------|-------|--------|
| Citation not returned in stream | 9/12 | done event の citations が空。rag-orchestrator → frontend の citation 伝搬に問題 |
| Intent not exposed in response | 5/12 | StreamChat が intent debug 情報を返していない |
| Answer too short | 7/12 | follow-up・topic-shift で retrieval が不十分 |
| Clarification not triggered | 1/12 | ConversationPlanner が曖昧クエリで clarification を返さない |
