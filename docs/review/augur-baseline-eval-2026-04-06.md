# Augur Baseline Eval Report

**Timestamp:** 2026-04-06T09:14:06Z

**Cases:** 12 | **Pass:** 0 | **Fail:** 12 | **Pass Rate:** 0%

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
| Intent Accuracy | 0.000 |
| Clarification Precision | 0.000 |
| Follow-up Resolution Rate | 0.000 |

### Generation

| Metric | Value |
|--------|-------|
| Mean Faithfulness | 0.500 |
| Mean Citation Correctness | 0.000 |
| Unsupported Claim Rate | 0.000 |
| Fallback Rate | 0.333 |

---

## Per-Case Results

### FAIL `iran-oil-crisis-causal`

**Query:** イランの石油危機はなぜ起きた？

**Tags:** causal, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 1237 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | 原油価格上昇 誤情報や偽画像などSNSで拡散 冷静に対応を; Airlines Are Already Preparing for an Oil Crisis | WIRED; Oil prices hit seven-month highs as tensions rise before US-Iran talks; 米 エネルギー長官 来日とりやめ 原油安定供給懸念が背景か; Iran threatens Gulf energy facilities after Israeli attack on its largest gasfield; Iran protesters tell of brutal police response as regime lashes out | Iran | The Guardian; 自民 イラン情勢長期化見据え 石油業界などの団体から聴き取り |

**Failures:**

- intent: got "", want "causal_explanation"
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
イランをめぐる情勢の緊迫化が、原油価格の高騰を引き起こし、石油危機的な懸念を生じさせていることが主な原因です。特に、イランによるエネルギー施設への報復的脅威や、ホルムズ海峡を巡る緊張が市場の懸念を煽っています。この状況を受け、原油の安定供給に対する懸念が顕在化し、関連する政府機関の動きや情報拡散といった二次的な影響も生じています。

## 詳細
イランを巡る石油危機的な状況は、複数の要因が複合的に作用した結果と考えられます。これらを「直接的要因」「構造的背景」「不確実性」の観点から分析します。

### 1. 直接的要因：地政学的緊張の激化と報復の連鎖
最も直接的な引き金となったのは、イランをめぐる軍事的緊張の高まりと、それによるエネルギーインフラへの脅威です。具体的には、イスラエルによるイラン最大のガス田への攻撃を受け、イランが湾岸地域全体のエネルギー施設への攻撃を脅迫したことが挙げられます[5]。この事態は、原油価格を急騰させる要因となり、トレーダーの反応を引き起こしました[3]。また、この緊張の高まりは、ホルムズ海峡という重要な航路における封鎖の懸念を高め、原油価格を...
```

</details>

---

### FAIL `iran-follow-up-developments`

**Query:** では、それに関連するイランの動向は？

**Conversation History:**

- **user:** 最近の石油危機の真因は？
- **assistant:** 最近の石油危機は、複数の産油国の減産方針と地政学的緊張の高まりが複合的に作用して発生しました。

**Tags:** follow-up, cjk, coreference, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 123 runes |
| Citations | 1 |
| Fallback | false |
| Retrieved Titles | Here’s Every Country Directly Impacted by the War on Iran | WIRED; Spain denies US permission to use jointly operated bases to attack Iran; Dubai’s famous Fairmont hotel in flames after Iranian air strike; Iran unleashes hundreds of drones aimed at targets across Middle East; Monday briefing: ​How are Iranians abroad grappling with loss and uncertainty from afar?; Israel-US war on Iran live: fire at US embassy in Riyadh after drone strike; state department urges all Americans to leave Middle East; Iran crisis live: UN says ‘cycle of horrific violence’ in Iran must as official says 2,000 killed in crackdown | Iran | The Guardian |
| Cited Titles | Iran crisis live: UN says ‘cycle of horrific violence’ in Iran must as official says 2,000 killed in crackdown | Iran | The Guardian |

**Failures:**

- answer length: got 123 runes, want >= 300

<details><summary>Answer (preview)</summary>

```
イラン国内では、何が起きているかという点で、抗議活動が起きていることや、インターネットが108時間以上停止しているという状況が報じられています。また、一部の報道では、イラン政府が「最終的な数日、数週間」に差し掛かっているという指摘もされています。
```

</details>

---

### FAIL `ambiguous-more-detail`

**Query:** もっと詳しく

**Conversation History:**

- **user:** 最近のAIチップ開発の動向は？
- **assistant:** NVIDIAのBlackwellアーキテクチャが発表され、推論性能が大幅に向上しました。

**Tags:** follow-up, ambiguous, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 104 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | 提供されたコンテキストは、AIチップの最新の具体的な動向（例：電力効率の改善や特定アーキテクチャの詳細）について言及していません。 |
| Retrieved Titles | 【ハンズオン】Vertex AI RAG Engine - ローカル開発から Agent Engine 環境まで - Yapodu Tech Blog; Flutter Anti-Pattern: How setState() Turns Your App Into a Slideshow; GitHub Spec Kit入門｜AIコーディングエージェントで仕様駆動開発を実践する | SIOS Tech Lab; Writing an Infix Expression Evaluator in C++ - DEV Community; Writing an Infix Expression Evaluator in C++ - DEV Community; Writing an Infix Expression Evaluator in C++ - DEV Community; Writing an Infix Expression Evaluator in C++ - DEV Community |

**Failures:**

- expected clarification but none was asked

<details><summary>Answer (preview)</summary>

```
現在、AIチップの開発は、特定のタスク（例：推論、トレーニング）に特化し、電力効率と性能を最大化する方向に進んでいます。例えば、特定のモデルやワークロードに最適化されたアクセラレータの設計が進められています。
```

</details>

---

### FAIL `ambiguous-is-that-true`

**Query:** それって本当？

**Conversation History:**

- **user:** EVバッテリーの寿命は延びている？
- **assistant:** 最新の全固体電池技術により、バッテリー寿命は従来比2倍に延びています。

**Tags:** follow-up, fact-check, coreference

| Field | Value |
|-------|-------|
| Answer Length | 78 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | コンテキスト全体が2026年のトレンドや技術的なトピックに関するものであり、バッテリー技術の進化に関する具体的な検証データが含まれていないため。 |
| Retrieved Titles | Exploring the Future of NLP: Trends, Techniques, and Tools in 2026; 2025: A Very Special Year in Review | Codrops; AI Technology Trends 2026: Latest Developments and Future Directions - DEV Community; Light Of The Ordinary, Macros And Photography Discussions - The Weekly Light Stalking Community Roundup | Light Stalking; Cocktail of the week: Huŏ’s Szechuan sizzle – recipe | The good mixer; 100 Charges, Zero AI Logs: Building Cryptographic Account Enforcement and Law Enforcement Referral Provenance with CAP-SRP v1.1 - DEV Community; Buy these 5 cheap Android phones before the RAM shortage ruins them |

**Failures:**

- intent: got "", want "fact_check"
- answer length: got 78 runes, want >= 200
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
提供されたコンテキストには、EVバッテリーの寿命に関する具体的な情報や、前回の回答（全固体電池による寿命の延長）を裏付ける証拠となる記述は含まれていません。
```

</details>

---

### FAIL `oil-crisis-root-cause`

**Query:** 最近の石油危機の真因は？

**Tags:** causal, cjk, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | retrieval quality insufficient: context relevance too low |

**Failures:**

- intent: got "", want "causal_explanation"
- answer length: got 0 runes, want >= 600
- citations required but none provided
- expected entity "石油" not found in answer

---

### FAIL `simple-temporal-query`

**Query:** 今週のAI関連ニュースは？

**Tags:** temporal, cjk

| Field | Value |
|-------|-------|
| Answer Length | 1645 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | As confusion reigns over Jony Ive's iO device, I remain an AI hardware skeptic - 9to5Mac; Anthropic Said No to the Pentagon. Meta Can't Beat Google. And NVIDIA Owns Everything. - DEV Community; Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community; OWASP Just Ranked Agent Identity Abuse as the #3 Risk in Agentic AI. Here Is Why.; Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Trump says he fired Anthropic ‘like dogs’ as negotiations with Pentagon reportedly restart |

**Failures:**

- intent: got "", want "temporal"
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
2026年4月3日の週のAI関連ニュースは、AI技術が単なるテクノロジーの話から、政治的、経済的、そして存在論的な問題へと移行している様子が浮き彫りになっています。特に、AIエージェントの急速な実運用への展開、企業によるAI資金調達のための人員削減、そしてAIの安全性のリスク指摘が目立ちました。

## 詳細
この週のAIニュースは、個々の出来事として見ても興味深いものが多いものの、それらを総合すると、業界が自律的なAIエージェントを本番システムに非常にアグレッシブに展開しており、そのスピードが安全対策（セーフティレール）の追いつきを上回っているという全体像が浮かび上がっています [6]。

**1. 業界の構造的変化と経済的影響**

*   **企業活動の側面**: この週には、AtlassianがAIの資金調達のために1,600人の人員削減を行ったという出来事がありました [3], [5]。この出来事と並行して、AIが経済的な側面を強く意識され始めたことが示唆されています [5]。
*   **政府・産業の関与**: 米軍が実際にAIを能動的な戦争（active wa...
```

</details>

---

### FAIL `comparison-query`

**Query:** GPT-4oとClaude 3.5の違いは？

**Tags:** comparison, cjk

| Field | Value |
|-------|-------|
| Answer Length | 765 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | 【ZabbixHA】MySQL高可用性構成 性能検証レポート – TechHarmony; インターフェースとしてのRSpecとAIで実現する、パフォーマンスチューニングの自動化 - Money Forward Developers Blog; Goでgo fmtしたくないコードを書いた（Go版Quine） - エムスリーテックブログ; エンジニアがチーム異動して変わったこと、取り組んだこと | MOBILUS TECH BLOG; Deliver Us from Evil | Daily Philosophy; 【ハンズオン】Vertex AI RAG Engine - ローカル開発から Agent Engine 環境まで - Yapodu Tech Blog; Your AI Agent Says 'Done' &mdash; How Do You Know It Actually Worked? |

**Failures:**

- intent: got "", want "comparison"
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
提供されたコンテキスト情報には、GPT-4oとClaude 3.5に関する直接的な比較データや詳細な記述は一切含まれていません。したがって、両モデルの違いについて包括的かつ詳細な比較を行うことは不可能です。

## 詳細
提供されたコンテキスト情報（[1]～[7]）は、主に以下のトピックに関するものであり、大規模言語モデル（LLM）の比較に関する記述は確認できません。

*   **データベースの可用性・性能検証**: ZabbixHAを用いたMySQLの高可用性構成の性能検証レポートが確認できます [1]。
*   **ソフトウェア開発・技術トピック**: パフォーマンスチューニングの自動化（RSpecとAIの利用）[2]、Go言語のコード生成（Quine）[3]、AI Agentの動作検証に関する言及 [7]、Vertex AI RAG Engineのハンズオン [6]などが含まれています。
*   **その他の記事**: エンジニアの異動に関するブログ記事 [4]や、哲学的な考察記事 [5]など、技術比較とは無関係なコンテンツも含まれています。

結論として、GPT-...
```

</details>

---

### FAIL `synthesis-broad-topic`

**Query:** そもそもニューヨークと芸術のかかわりは？

**Tags:** synthesis, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 1947 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | High and low: the spontaneous joy and drama of New York City – in pictures; Meet the Speakerhead Wiring the Art World for Sound; Venezuelan Artists Speak Out; A View From the Easel; Hard Truths: Can an Artist Dally With Dubious Funding and Come Out Clean on the Other Side?; Required Reading; A View From the Easel |

**Failures:**

- intent: got "", want "synthesis"
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
ニューヨークと芸術の関係性は、単なる文化的な共存以上の、ダイナミックで多面的な相互作用を特徴としています。それは、芸術活動の場、経済的な側面、そして都市の社会的な変遷と密接に結びついています。この関係性は、アーティストたちの活動拠点、展示の機会、そして都市の景観そのものに影響を与え、時に「生への戦い」とも言えるほどの切実な側面を持つことが示唆されています。

## 詳細

### **芸術活動の物理的・コミュニティ的基盤の提供**

ニューヨークは、芸術家たちにとって物理的な活動の場を提供しています。具体的には、居住や創作活動を支援する制度が存在し、例えば、ベネズエラ人アーティストに対してニューヨーク市内で年間無償のスタジオスペースを提供するレジデンシープログラムが実施されています[3]。また、特定の地域やコミュニティレベルでも、芸術活動を支える基盤が見られます。例えば、ウッドストックやハドソンバレーといった地域では、アーティストのアソシエーション（例：Woodstock Artists Association and Museum）が設立され、地域に根差した芸術コミュニテ...
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
| Answer Length | 38 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | 提供されたコンテキストは、AIエージェントやプログラミングに関する技術記事であり、半導体市場の動向に関する情報は含まれていないため。 |
| Retrieved Titles | 【ハンズオン】Vertex AI RAG Engine - ローカル開発から Agent Engine 環境まで - Yapodu Tech Blog; Flutter Anti-Pattern: How setState() Turns Your App Into a Slideshow; GitHub Spec Kit入門｜AIコーディングエージェントで仕様駆動開発を実践する | SIOS Tech Lab; Writing an Infix Expression Evaluator in C++ - DEV Community; Writing an Infix Expression Evaluator in C++ - DEV Community; Writing an Infix Expression Evaluator in C++ - DEV Community; Writing an Infix Expression Evaluator in C++ - DEV Community |

**Failures:**

- answer length: got 38 runes, want >= 200
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
提供されたコンテキストには、半導体市場の動向に関する情報は含まれていません。
```

</details>

---

### FAIL `cjk-short-query-bm25`

**Query:** 量子コンピュータ 実用化

**Tags:** cjk, bm25, short-query

| Field | Value |
|-------|-------|
| Answer Length | 1580 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | SC25に参加・展示しました#2 ～量子コンピュータと、そのアプリケーションに関する展示 - fltech - 富士通研究所の技術ブログ; Boosting one protein helps the brain protect itself from Alzheimer’s; From RSA to ECC: The Impact of Quantum Computing on Modern Cryptography; Quantum Computing Will Break AI Privacy — And Nobody in the Industry Is Ready - DEV Community; AI Technology Trends 2026: Latest Developments and Future Directions - DEV Community; 前略、母さん、PQC移行が心配です / 開発者向けブログ・イベント | GMO Developers; Mastering Quantum Logic with Quantum Studio |

**Failures:**

- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
量子コンピュータの実用化は、特定用途での計算アクセラレータとしての期待が高まっているものの、現時点では実用的なシステム構築にはさらなる研究開発が必要な段階です。特に、従来の暗号システムに対する脅威が指摘されており、これに対抗する「耐量子計算機暗号（PQC）」の導入が喫緊の課題となっています。また、量子機械学習などの応用分野での進展が見られますが、具体的な実証段階へ進むためには、スーパーコンピュータとの連携といったシステムレベルでの検証が重要視されています。

## 詳細
### 1. 実用化の現状と技術的な課題
量子コンピュータは、特定用途における計算アクセラレータとして期待されていますが、具体的にどのような用途で使えるのかという点については、まだ言及が難しい状況です[1]。現在、注目が集まっているのは、量子コンピュータ自体というよりは、「スーパーコンピュータと量子コンピュータをどのように連携させれば実用的なシステムとなるのか」という点に焦点が当たっています[1]。実際に動作するシステムを構築し、実証できる段階に到達するためには、地道な研究開発が求められています[1]。

...
```

</details>

---

### FAIL `deep-dive-technical`

**Query:** Transformerアーキテクチャのattention機構について詳しく

**Tags:** deep-dive, technical

| Field | Value |
|-------|-------|
| Answer Length | 1859 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | Attention再入門 is all you need; NTTとドコモが開発した「大規模行動モデル(LAM)」で1to1マーケティングを革新──テレマ受注率が最大2倍に向上 | ヘッドウォータースのフィード; Building Transformer from Scratch - DEV Community; Transformerアーキテクチャの変遷 ~Attention is All You Needからgpt-ossまで~; Transformerアーキテクチャの変遷 ~Attention is All You Needからgpt-ossまで~; Transformerアーキテクチャの変遷 ~Attention is All You Needからgpt-ossまで~; Attention Is All You Need — Explained Like You’re Building It From Scratch - DEV Community |

**Failures:**

- intent: got "", want "topic_deep_dive"
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要
TransformerアーキテクチャにおけるAttention機構は、単に「Attention is all you need」という論文で注目された技術というだけでなく、系列データ（文章や時系列など）から文脈を深く理解するための核心的なメカニズムです。これは、各単語（トークン）が他の単語全体を「見て」、どの単語にどれだけ「注目（フォーカス）」を当てるかを計算することで実現されます。この機構により、モデルは単なる順番処理を超え、単語間の関係性そのものを捉えることが可能になり、現在の大規模言語モデル（LLM）を支える基盤技術となっています。

## 詳細
### 1. Attention機構の基本的な概念と役割
Attention機構の核心的なアイデアは、シーケンスデータ（系列データ）を処理する際に、単に隣接する要素に依存するのではなく、文脈全体における各要素間の「関係性」を理解することにあります[7]。このメカニズムにより、モデルは「各単語が他の単語を見る」ことを可能にし、その「注目度」を計算します[3]。

**技術的な理解のポイント:**
*   **コアアイデア:**...
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
| Answer Length | 189 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | GitHub Copilot カスタムエージェントのための agents.md 作成ベストプラクティス; Cloudflare（クラウドフレア）とは？エラーコードの詳細やエラーを防ぐ方法を解説; 論理削除とDecoratorパターンで作る柔軟なリポジトリ設計; フリーランスエンジニアは「経験の切り売り」になりやすいのか？; A Case for Postmodernism | Daily Philosophy; 11 Best Lenses for Landscape Photography (in 2025); Quick crossword No 17,430 |

**Failures:**

- answer length: got 189 runes, want >= 300
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
生成AIの著作権問題の議論は、単に「学習データ」の利用に留まらず、AIが出力した成果物（生成物）の「著作物性」や「権利帰属」という側面に焦点を当てる視点も重要です。具体的には、AIが生成したものが著作権法上の保護対象となる「創作性」を有しているか、そしてその創作性が誰（AIの利用者か、開発者か、それともAI自身か）に帰属するのかという点が、法制度上の大きな論点となっています。
```

</details>

---

## Analysis

### Systemic Issues

| Issue | Count | Impact |
|-------|-------|--------|
| Citation not returned in stream | 10/12 | done event の citations が空。rag-orchestrator → frontend の citation 伝搬に問題 |
| Intent not exposed in response | 7/12 | StreamChat が intent debug 情報を返していない |
| Answer too short | 5/12 | follow-up・topic-shift で retrieval が不十分 |
| Clarification not triggered | 1/12 | ConversationPlanner が曖昧クエリで clarification を返さない |
