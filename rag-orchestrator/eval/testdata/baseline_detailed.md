# Augur Baseline Eval Report

**Timestamp:** 2026-04-08T10:32:58Z

**Cases:** 12 | **Pass:** 4 | **Fail:** 8 | **Pass Rate:** 33%

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
| Intent Accuracy | 0.714 |
| Clarification Precision | 0.000 |
| Follow-up Resolution Rate | 0.500 |

### Generation

| Metric | Value |
|--------|-------|
| Mean Faithfulness | 0.167 |
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
| Answer Length | 561 runes |
| Citations | 6 |
| Fallback | false |
| Retrieved Titles | US interest in electric vehicles surges as gas prices jump amid Iran war | Business | The Guardian; Trump says he’s been assured Tehran has stopped killing protesters as Iran reopens its airspace – live | Iran | The Guardian; Iranian airspace reopens – as it happened | Iran | The Guardian; 緊迫のイラン情勢 日々の暮らしや経済に影響広がる; 原油安定供給に懸念 関連製品の値上げや火力発電設備の停止も; 「ナフサ」供給懸念 関連製品の減産や値上げの動きが広がる; 「エチレン」国内生産に影響 イラン情勢 原油の安定供給懸念で |
| Cited Titles | US interest in electric vehicles surges as gas prices jump amid Iran war | Business | The Guardian; 原油安定供給に懸念 関連製品の値上げや火力発電設備の停止も; 「ナフサ」供給懸念 関連製品の減産や値上げの動きが広がる; 「エチレン」国内生産に影響 イラン情勢 原油の安定供給懸念で; 緊迫のイラン情勢 日々の暮らしや経済に影響広がる; Trump says he’s been assured Tehran has stopped killing protesters as Iran reopens its airspace – live | Iran | The Guardian |

**Failures:**

- answer length: got 561 runes, want >= 800

<details><summary>Answer (preview)</summary>

```
**直接的要因**\n*   米国とイスラエルによるイランへの爆撃[1]が引き金となり、原油価格が上昇した[1]。\n*   この紛争により、世界の原油の約5分の1が通過する重要な水路であるホルムズ海峡がイランによって閉鎖された懸念が生じた[1]。\n*   イラン情勢の緊迫化に伴い、原油の安定供給への懸念が生じている[5, 6, 7]。\n\n**構造的背景**\n*   イラン情勢の緊迫化が、原油価格の高騰を引き起こし、レギュラーガソリンの小売価格が過去最高水準に達するなどの影響を及ぼしている[4]。\n*   原油の安定供給への懸念は、石油化学製品の値上げや、重油を燃料とする火力発電設備の稼働の一時停止といった広範な影響を企業活動にも及ぼしている[5]。\n*   イラン情勢の緊迫化は、石油製品の「ナフサ」の供給懸念を引き起こし、メーカー間で関連製品の減産や値上げの動きを広げている[6]。\n*   原油を原料とする企業の間では、生産活動への影響が出ている[7]。\n\n**不確実性**\n*   イランの空域が再開したことや、テヘランが抗議デモの鎮圧を停止したとの発言[2]があ...
```

</details>

---

### PASS `iran-follow-up-developments`

**Query:** では、それに関連するイランの動向は？

**Conversation History:**

- **user:** 最近の石油危機の真因は？
- **assistant:** 最近の石油危機は、複数の産油国の減産方針と地政学的緊張の高まりが複合的に作用して発生しました。

**Tags:** follow-up, cjk, coreference, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 414 runes |
| Citations | 5 |
| Fallback | false |
| Retrieved Titles | US Coast Guard pursuing another oil tanker off coast of Venezuela | Trump administration | The Guardian; Globalisation is under threat from Iran war – and Britain is uniquely vulnerable; Middle East war creating ‘largest supply disruption in the history of oil markets’; ‘The stakes are enormous’: how a prolonged Iran war could shock the global economy; Trump promises oil companies ‘total safety’ in Venezuela as he urges them to invest billions | Donald Trump | The Guardian; Trump indicates that Venezuela is responding to demands for ‘total access’ for US oil companies – US politics live | US news | The Guardian; Venezuela condemns ‘warmongering threats’ as Trump orders oil blockade – US politics live | Trump administration | The Guardian |
| Cited Titles | Middle East war creating ‘largest supply disruption in the history of oil markets’; Globalisation is under threat from Iran war – and Britain is uniquely vulnerable; ‘The stakes are enormous’: how a prolonged Iran war could shock the global economy; US Coast Guard pursuing another oil tanker off coast of Venezuela | Trump administration | The Guardian; Venezuela condemns ‘warmongering threats’ as Trump orders oil blockade – US politics live | Trump administration | The Guardian |

<details><summary>Answer (preview)</summary>

```
**イラン関連の地政学的動向と原油市場への影響**\n\n*   **全体的な懸念**：中東での戦争が「石油市場史上最大の供給途絶」を引き起こしていると報じられています[3]。\n*   **広範囲な影響**：米国とイスラエル間の攻撃から生じる経済的な波及効果は、エネルギー価格や食料品などあらゆるものに影響を及ぼすと指摘されています[2]。\n*   **長期的な懸念**：イランでの長期的な戦争は、過去の世界的な経済危機に似たものとなり得るとされ、特に「急騰する原油・ガス価格」は経済的な問題の前兆とされています[4]。\n*   **価格への影響**：これらの要因から、地政学的な緊張が高まるにつれて原油価格が上昇する可能性が示唆されています[1]。\n\nなお、原油価格は、ウクライナ和平交渉の進展により過去に数年ぶりの安値に達したものの、ベネズエラへの供給リスクが再び価格を押し上げている状況が確認されています[7]。
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
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | retrieval quality insufficient: context relevance too low |

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
| Answer Length | 136 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | Apple better positioned as 2026 notebook shipments face decline: report - 9to5Mac; After the Galaxy S26, I'm really worried about the Galaxy Z Flip 8 - Android Authority; I can't believe Apple wanted the iPhone 16e to compete with Android; Forget Shure: I recommend this flexible mic for podcasts and meetings, especially at its price | ZDNET; My 7 wishes for Android phones in 2026 - Android Authority; 2026 is already shaping up to be a bad year to buy a new smartphone - Android Authority; What does the RAM shortage mean for smartphones in 2026? |

**Failures:**

- answer length: got 136 runes, want >= 200
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
**主張**\n全固体電池によるバッテリー寿命の延長が科学的根拠に基づいているか\n\n**根拠**\n提供されたコンテキスト情報には、全固体電池によるバッテリー寿命の延長に関する記述や、それを裏付ける科学的根拠についての言及がありません。\n\n**判定**\n判定不能
```

</details>

---

### FAIL `oil-crisis-root-cause`

**Query:** 最近の石油危機の真因は？

**Tags:** causal, cjk, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 565 runes |
| Citations | 7 |
| Fallback | false |
| Retrieved Titles | Middle East war creating ‘largest supply disruption in the history of oil markets’; Fuel rations and no air con: south-east Asian nations race to conserve energy; Aramco warns of oil market ‘catastrophe’ unless strait of Hormuz reopens soon; Reader callout: are you feeling the petrol pinch in Australia, and how is it affecting you?; How high could oil go, and what might the global economic fallout be?; CMA puts UK fuel retailers ‘on notice’ over profiteering from Iran war; War in Iran Spiked Oil Prices. Trump Will Decide How High They Go |
| Cited Titles | Middle East war creating ‘largest supply disruption in the history of oil markets’; Aramco warns of oil market ‘catastrophe’ unless strait of Hormuz reopens soon; Reader callout: are you feeling the petrol pinch in Australia, and how is it affecting you?; Fuel rations and no air con: south-east Asian nations race to conserve energy; How high could oil go, and what might the global economic fallout be?; War in Iran Spiked Oil Prices. Trump Will Decide How High They Go; CMA puts UK fuel retailers ‘on notice’ over profiteering from Iran war |

**Failures:**

- answer length: got 565 runes, want >= 600

<details><summary>Answer (preview)</summary>

```
**直接的要因**\nイランでの戦争が、タンカーによる原油の輸送を妨げているため、石油市場は「史上最大の供給途絶」に直面していると世界エネルギー監視機関が警告している[1]。また、アラブ首長国連邦の国営石油会社は、この危機を地域で最も大きいものだと呼び、ホルムズ海峡がまもなく再開されない限り「カタストロジ」になると警告している[3]。\n\n**構造的背景**\n*   **地域的なエネルギー依存と影響**: アジアは、海峡を通過する輸入エネルギーに大きく依存しており、この危機の影響を深く受けている[4]。\n*   **各国政府の対応**: 中東の石油に大きく依存する国々では、高騰するコストから国民を守るため、燃料配給や空調の制限といったエネルギー節約策を導入している[2]。\n*   **価格への懸念**: この紛争は原油価格を押し上げており、消費者に深刻な影響を及ぼす可能性があり、原油価格が2008年の記録高を上回る可能性についての議論がある[5]。\n\n**不確実性**\n*   価格上昇の度合いについて、トランプ氏がそれらをどこまで引き上げるかを決定するという言及がある[7...
```

</details>

---

### PASS `simple-temporal-query`

**Query:** 今週のAI関連ニュースは？

**Tags:** temporal, cjk

| Field | Value |
|-------|-------|
| Answer Length | 660 runes |
| Citations | 7 |
| Fallback | false |
| Retrieved Titles | Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Anthropic Said No to the Pentagon. Meta Can't Beat Google. And NVIDIA Owns Everything. - DEV Community; Anthropic Said No to the Pentagon. Meta Can't Beat Google. And NVIDIA Owns Everything. - DEV Community; Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community |
| Cited Titles | Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Anthropic Said No to the Pentagon. Meta Can't Beat Google. And NVIDIA Owns Everything. - DEV Community; Anthropic Said No to the Pentagon. Meta Can't Beat Google. And NVIDIA Owns Everything. - DEV Community; Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Atlassian Just Laid Off 1,600 People to Fund AI. We Need to Talk About This. - DEV Community |

<details><summary>Answer (preview)</summary>

```
**2026年4月3日週のAI関連の最新動向**\n\nこの週のAI関連のニュースは、単なる技術の話から、政治的、経済的、そして実存的な問題へと移行したという点で特徴づけられます[3, 4]。\n\n**主要な出来事と動向**\n\n*   **企業によるAIへの大規模投資と人員削減:** Atlassian社が「AIの資金調達」のために1,600人の人員削減を行ったことが報じられています[1, 5]。\n*   **軍事利用の進展:** 米軍が実戦においてAIを使用していることが確認されました[1]。\n*   **AIエージェントの急速な展開:** 自律的なAIエージェントが本番システムに積極的に展開されており、安全対策が追いついていない状況が指摘されています[2]。\n*   **技術開発の多様化:** ロボティクスチームが、物理的なAIシステムを完全に合成データのみで訓練したという論文を発表しました[1]。\n*   **業界の構造的変化:** Anthropic社が国防総省（Pentagon）を断ったこと、Meta社がGoogle社に勝てない可能性、そしてNVIDIAが全てを...
```

</details>

---

### PASS `comparison-query`

**Query:** GPT-4oとClaude 3.5の違いは？

**Tags:** comparison, cjk

| Field | Value |
|-------|-------|
| Answer Length | 1231 runes |
| Citations | 7 |
| Fallback | false |
| Retrieved Titles | Claude Sonnet 4.5 Code Review Benchmark; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026; Claude vs ChatGPT for Coding: The Real Differences in 2026; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026; ChatGPT vs Claude for Coding: Which AI Is Better for Developers in 2025? |
| Cited Titles | Claude Sonnet 4.5 Code Review Benchmark; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026; Claude vs ChatGPT for Coding: The Real Differences in 2026; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026; ChatGPT vs Claude for Coding: Which AI Is Better for Developers in 2025? |

<details><summary>Answer (preview)</summary>

```
**共通点**\n*   **コードレビュー性能**: Claude Sonnet 4.5とGPT-4oは、全体的なコードレビューにおいて比較可能な性能を示す[1]。\n*   **複雑なタスク**: 両モデルとも、複雑なレビュータスクにおいては、より小規模なモデルよりも著しく優れた性能を示す[1]。\n*   **コード生成**: Claude（Opus/Sonnet）は、初回の試みでより良いコードを書く傾向がある[3]。\n\n**相違点**\n*   **コードレビューの質（バグ検出と提案）**: Claudeは、ニュアンスに富んだバグ検出に優れており、より実行可能な提案を生成する[1]。一方、GPT-4oはReact固有のXSS検出においてClaudeを上回り、フロントエンドのセキュリティパターンに関する訓練がClaudeに若干少ない可能性が示唆されている[2]。\n*   **速度**: GPT-4oは、Claudeよりも高速であると指摘されている[1, 3]。GPT-4oは「目に見えて」速い[3]。\n*   **精度と再現性**: GPT-4oは標準のClaude Son...
```

</details>

---

### FAIL `synthesis-broad-topic`

**Query:** そもそもニューヨークと芸術のかかわりは？

**Tags:** synthesis, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 779 runes |
| Citations | 0 |
| Fallback | false |
| Retrieved Titles | Dear Zohran, Don’t Let Art Workers Down; The Commissioner We Need; NYC Deserves a Culture Commissioner Who Cares; Dear Zohran, Don’t Let Art Workers Down; Dear Zohran, Don’t Let Art Workers Down; Queer Arab Art in Manhattan; 15 Shows to See in New York City This April |

**Failures:**

- intent: got "general", want "synthesis"
- answer length: got 779 runes, want >= 800
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
## 概要\nニューヨーク市にとって、芸術は単なる贅沢品ではなく、市民のインフラストラクチャとして不可欠な要素です。芸術は、人々の意味づけ、帰属意識の構築、コミュニティの福祉の向上、そして経済的な機会創出に深く関わっています[3]。\n\n## 詳細\n*   **不可欠な側面**: ニューヨーク市において芸術は欠かせないものであり、この都市の国際的な地位を支える側面の一つと見なされています[2]。文化的な生活は、市民の生活基盤（civic infrastructure）であると位置づけられています[3]。\n*   **芸術の役割**: 芸術は、人々の運命を形作る力を表現する手段であり、地域社会のすべてのメンバーが芸術的・文化的な創造に参加し、手頃でローカルな芸術プログラムにアクセスできることが求められています[4]。\n*   **課題と視点**: 一方で、芸術分野においては、単なる官民連携（neoliberal model）への依存ではなく、より大きな公的支援と資金提供が必要であるとの指摘があります[5]。また、芸術はコミュニティの福祉向上や経済機会創出に貢献するという視点も強調...
```

</details>

---

### PASS `topic-shift-explicit`

**Query:** 別件だけど、最新の半導体市場の動向は？

**Conversation History:**

- **user:** EVバッテリーの技術は？
- **assistant:** 全固体電池の実用化が進んでいます。

**Tags:** follow-up, topic-shift

| Field | Value |
|-------|-------|
| Answer Length | 699 runes |
| Citations | 6 |
| Fallback | false |
| Retrieved Titles | ML Hit 99% Accuracy on Yield Prediction — The Factory Floor Ignored It; 3D Chip Stacking Has a Warpage Problem — GNNs and RTX 4060 Benchmarks Show Why; Defense Stocks 2026: Who's Getting Rich From the Iran War [Complete Guide] - DEV Community; Talent Oversupply: How Less Prestigious Firms Can Attract Top-Tier Candidates Amid Economic Shifts; Google parent Alphabet beats forecasts with first $100bn quarter; AI-Powered Detection Engineering: Transforming Security Operations from Reactive to Predictive - DEV Community; Why the Latent Space Needs a Librarian |
| Cited Titles | Defense Stocks 2026: Who's Getting Rich From the Iran War [Complete Guide] - DEV Community; 3D Chip Stacking Has a Warpage Problem — GNNs and RTX 4060 Benchmarks Show Why; ML Hit 99% Accuracy on Yield Prediction — The Factory Floor Ignored It; Google parent Alphabet beats forecasts with first $100bn quarter; AI-Powered Detection Engineering: Transforming Security Operations from Reactive to Predictive - DEV Community; Talent Oversupply: How Less Prestigious Firms Can Attract Top-Tier Candidates Amid Economic Shifts |

<details><summary>Answer (preview)</summary>

```
**直近の半導体市場の動向とトレンド**\n\n2026年4月3日時点の情報に基づくと、半導体産業においては以下の動向が確認できます。\n\n*   **需要と供給の側面（地政学リスクとサプライチェーン）**\n    *   イラン紛争の影響により、高度な電子システムに対する需要が高まっており、専門的なコンポーネントに対する需要が前例のないものとなっています。これらの部品の多くは限られたサプライヤーから供給されています[3]。\n    *   防衛関連企業の業績は、サプライチェーンの回復力と半導体の入手可能性にますます依存しています[3]。\n*   **技術的な課題と予測**\n    *   3Dチップスタッキングには、ウェーページの問題が存在することが指摘されています[2]。\n    *   LSTMモデルを用いたイベント介入に基づく半導体産業のトレンド予測に関する研究が行われています[1, 2]。\n*   **市場の競争とAI関連**\n    *   AIおよびクラウド市場全体での競争が激化しており、競合他社が積極的に価格引き下げや新しい生成AI機能の導入を行っていま...
```

</details>

---

### FAIL `cjk-short-query-bm25`

**Query:** 量子コンピュータ 実用化

**Tags:** cjk, bm25, short-query

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | retrieval quality insufficient: context relevance too low |

**Failures:**

- min relevant contexts: got 0, want >= 1
- citations required but none provided

---

### FAIL `deep-dive-technical`

**Query:** Transformerアーキテクチャのattention機構について詳しく

**Tags:** deep-dive, technical

| Field | Value |
|-------|-------|
| Answer Length | 1804 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | answer quality insufficient: short answer with quality issues |
| Retrieved Titles | Three Things Had to Align: The Real Story Behind the LLM Revolution; Attention再入門 is all you need; Building Transformer from Scratch - DEV Community; Attention Is All You Need — Explained Like You’re Building It From Scratch - DEV Community; Transformerアーキテクチャの変遷 ~Attention is All You Needからgpt-ossまで~; Attention Is All You Need — Explained Like You’re Building It From Scratch - DEV Community; Attention Is All You Need — Full Paper Breakdown - DEV Community |

**Failures:**

- intent: got "", want "topic_deep_dive"
- citations required but none provided

<details><summary>Answer (preview)</summary>

```
**TransformerアーキテクチャにおけるAttention機構の仕組み、原理、および役割の詳細**\n\nTransformerアーキテクチャの根幹をなすAttention機構は、これまでのAIモデルが抱えていた構造的な問題を解決し、現代のLLMブームを支える極めて重要な技術です[2, 7]。この機構の核心的なアイデアは、「シーケンス（系列）そのものが必要なのではなく、要素間の『関係性（relationships）』が必要である」という点に集約されます[4]。\n\n### 1. Attention機構が解決した従来の課題（原理的背景）\n\nAttention機構が登場する以前、RNN（リカレントニューラルネットワーク）やLSTMなどのモデルは、テキストを「単語ごとに、順番に（sequentially）」処理するという制約がありました[6]。この逐次処理（One word at a time）という性質が、以下の二つの大きな問題を引き起こしていました[6]：\n\n*   **学習速度の遅さ（並列性の欠如）**: 単語を順番に処理するため、計算の並列化が難しく、学習が遅くなる...
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
| Fallback | true |
| Fallback Reason | llm stream failed: Post "http://news-creator:11434/api/chat": dial tcp: lookup news-creator on 127.0.0.11:53: no such host |
| Retrieved Titles | I Tried the Notion MCP Challenge — Can I Control My AI Agent?” ⭐; DRYを極める：go generateという選択肢; PC未経験だった大学生が、半年で40万行のWebアプリを個人開発した話（と、その全ディレクトリ構造）; VueWeave: Vue 3用の現代的なノードベースフローエディタを作りました; Ignite 2025で見えた「自律エージェント時代」の設計原則──Agent 365・IQスタック・モデル不可知論が示す次のアーキテクチャ; GitHub Copilot カスタムエージェントのための agents.md 作成ベストプラクティス; 多重決済を防ごうとしてwith_lockでハマった話 | Rentio Tech Blogさんのフィード |

**Failures:**

- answer length: got 0 runes, want >= 300
- citations required but none provided

---

## Analysis

### Systemic Issues

| Issue | Count | Impact |
|-------|-------|--------|
| Citation not returned in stream | 5/12 | done event の citations が空。rag-orchestrator → frontend の citation 伝搬に問題 |
| Intent not exposed in response | 2/12 | StreamChat が intent debug 情報を返していない |
| Answer too short | 5/12 | follow-up・topic-shift で retrieval が不十分 |
| Clarification not triggered | 1/12 | ConversationPlanner が曖昧クエリで clarification を返さない |
