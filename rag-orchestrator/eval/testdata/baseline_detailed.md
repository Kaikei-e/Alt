# Augur Baseline Eval Report

**Timestamp:** 2026-04-11T02:21:52Z

**Cases:** 12 | **Pass:** 2 | **Fail:** 10 | **Pass Rate:** 17%

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
| Follow-up Resolution Rate | 0.000 |

### Generation

| Metric | Value |
|--------|-------|
| Mean Faithfulness | 0.000 |
| Mean Citation Correctness | 0.000 |
| Unsupported Claim Rate | 0.000 |
| Fallback Rate | 0.583 |

---

## Per-Case Results

### FAIL `iran-oil-crisis-causal`

**Query:** イランの石油危機はなぜ起きた？

**Tags:** causal, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 444 runes |
| Citations | 6 |
| Fallback | false |
| Retrieved Titles | Iran war energy crisis equal to 70s twin oil shocks and Ukraine invasion fallout, says IEA chief | First Thing; Iran war energy crisis equal to 70s twin oil shocks and fallout from Ukraine war, says IEA chief; Iran war energy crisis equal to 70s twin oil shocks and fallout from Ukraine war, says IEA chief; How high could oil go, and what might the global economic fallout be?; UK letting US use its bases is ‘participation in aggression’, Iran’s foreign minister tells Yvette Cooper – UK politics live | Politics | The Guardian; Iran hits Israeli town housing nuclear facility in retaliation for Natanz strike; Trump calls on UK to send warships to keep strait of Hormuz open |
| Cited Titles | UK letting US use its bases is ‘participation in aggression’, Iran’s foreign minister tells Yvette Cooper – UK politics live | Politics | The Guardian; Iran hits Israeli town housing nuclear facility in retaliation for Natanz strike; How high could oil go, and what might the global economic fallout be?; Iran war energy crisis equal to 70s twin oil shocks and Ukraine invasion fallout, says IEA chief | First Thing; Iran war energy crisis equal to 70s twin oil shocks and fallout from Ukraine war, says IEA chief; Iran war energy crisis equal to 70s twin oil shocks and fallout from Ukraine war, says IEA chief |

**Failures:**

- answer length: got 444 runes, want >= 800
- expected entity "石油" not found in answer

<details><summary>Answer (preview)</summary>

```
**直接的要因**\n*   イランによるホルムズ海峡の封鎖や、湾岸地域におけるエネルギー施設への攻撃が懸念を高めている[5]。\n*   イランが、平時において世界の原油貿易の5分の1が利用する重要な水路を封鎖した[6]。\n*   米国とイスラエル間の戦争がイランに関連していることにより、原油価格が1バレル100ドルを超えて急騰し、世界経済への懸念を引き起こしている[4]。\n\n**構造的背景**\n*   国際エネルギー機関（IEA）の責任者は、イランを原因とする世界的なエネルギー危機が、1970年代の二度のオイルショックとロシアによるウクライナ侵攻の余波を合わせたものに匹敵すると述べている[1, 2]。\n*   イランの国務大臣は、湾岸地域におけるエネルギー供給の安全保障に対する懸念が高まっていると指摘している[5]。\n\n**不確実性**\n*   イランの爆撃やホルムズ海峡の閉鎖が、当初、世界的な指導者たちによって理解されていなかったと指摘されている[3]。
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
| Answer Length | 1088 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | answer quality insufficient: short answer with quality issues |
| Retrieved Titles | Middle East war creating ‘largest supply disruption in the history of oil markets’; ‘This is round two’: Attacks on Iran have broad support among unsurprised Israelis; Yet another mid-talks attack jeopardises chances of Iran taking Trump seriously; US and Israel launch joint attack on Iran as Trump urges regime change; A visual guide to US-Israeli strikes on Iran – and Tehran’s response; Countries must seek energy independence through renewables and nuclear, says John Kerry; Iran hits Israeli town housing nuclear facility in retaliation for Natanz strike |

**Failures:**

- citations required but none provided

<details><summary>Answer (preview)</summary>

```
**石油危機におけるイランの動向と市場への影響に関する深掘り分析**\n\n提供されたコンテキスト情報に基づくと、最近の石油市場の動向は、イランを巻き込んだ紛争とそれに伴う供給途絶が主要因となっています。この状況は、エネルギー市場にとって「史上最大の供給途絶」を引き起こしていると警告されています[1]。\n\n**1. 供給途絶の具体的なメカニズムと影響**\n\n*   **海路の遮断:** イランをめぐる戦争が、タンカーによる原油の輸送を妨害しており、これが市場に大きな影響を与えています[1]。さらに、イランは、平時には世界の原油貿易の5分の1が利用される重要な水路を封鎖したと報告されています[7]。\n*   **市場への警告:** 世界のエネルギー監視機関は、この状況を「史上最大の供給途絶」と警告しています[1]。\n\n**2. 地政学的緊張の構造とイランの関与**\n\n*   **軍事的な対立:** コンテキストには、米国とイスラエルによるイランへの攻撃、およびそれに対するイランの報復行動に関する複数の報道が含まれています[2, 3, 4, 5]。例えば、イランは、ナタン...
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
| Answer Length | 985 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | answer quality insufficient: short answer with quality issues |
| Retrieved Titles | ‘We excel at every phase of AI’: Nvidia CEO quells Wall Street fears of AI bubble amid market selloff; Japan Is Building a 1.4nm AI Chip. No, That's Not a Typo.; I spoke to Arm to find out why your Android phone needs all that AI power; 高火力 PHY B200で、H100の4倍の費用対効果を得る方法 | さくらのナレッジ; The Missing Guardrail in AI Coding: Protecting Architecture - DEV Community; Why Connecting AI to Real Systems Is Still Hard - DEV Community; Your Microservices Are Holding Your AI Back. Here's What We Replaced Them With. |

**Failures:**

- expected clarification but none was asked

<details><summary>Answer (preview)</summary>

```
**NVIDIA Blackwellアーキテクチャの概要と技術的進化**\n\n提供された情報に基づくと、NVIDIAのAIチップ開発において、Blackwellアーキテクチャが重要な進展を示していることがわかります。特に、このアーキテクチャは、前世代のHopperアーキテクチャ（NVIDIA H100/H200）からの「正統進化」として位置づけられています[4]。\n\n**技術的詳細と性能向上**\n\nBlackwellアーキテクチャの具体的な技術的詳細として、Hopper世代からの進化が挙げられています[4]。この進化により、B200プラン（正確にはSec.Fモデル）を利用することで、前世代のH100やH200と比較して、費用対効果の面で大幅な向上が図られていることが示唆されています[4]。\n\n**推論性能への影響**\n\nAIチップの需要は継続的に成長しており、AIが「至るところで、すべてを、一度に」実行するようになり、「AIはどこにでも、すべてを、同時に」なると述べられています[1]。この広範なAIの展開に伴い、チップの性能、特に推論（Inference）の領域での進...
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
| Answer Length | 171 runes |
| Citations | 1 |
| Fallback | false |
| Retrieved Titles | Here are six exciting Apple product launches to look forward to in 2026 - 9to5Mac; React in 2026: Start From Scratch the Right Way (+ Cheat Sheet); Is this the world’s first quantum battery? Australian scientists say so; The Future of Web Development: What's Actually Changing in 2026 (Not Just Hype) - DEV Community; OnePlus 16 leak hints at a new 'Pro' or 'Ultra' model; Why your iPhone isn't powered by a solid-state battery yet - even though it beats lithium-ion | ZDNET; The average smartphone will get more storage in 2026, despite price hikes |
| Cited Titles | Why your iPhone isn't powered by a solid-state battery yet - even though it beats lithium-ion | ZDNET |

**Failures:**

- answer length: got 171 runes, want >= 200

<details><summary>Answer (preview)</summary>

```
**主張**\n全固体電池のバッテリー寿命が2倍に延びているという主張の真偽について。\n\n**根拠**\nコンテキストには、全固体電池がリチウムイオンバッテリーよりも「より多くの充電サイクルを提供する」という記述はありますが、寿命が「2倍に延びている」という具体的な記述や数値は確認できませんでした。[6]\n\n**判定**\n判定不能
```

</details>

---

### FAIL `oil-crisis-root-cause`

**Query:** 最近の石油危機の真因は？

**Tags:** causal, cjk, known-failure-2026-03-28

| Field | Value |
|-------|-------|
| Answer Length | 400 runes |
| Citations | 4 |
| Fallback | false |
| Retrieved Titles | Cocktail of the week: Huŏ’s Szechuan sizzle – recipe | The good mixer; Trump’s re-election may have helped Albanese – but the US war in Iran is creating economic conundrums; AI Technology Trends 2026: Latest Developments and Future Directions - DEV Community; NY原油市場 先物価格 一時1バレル＝100ドル超に 事態悪化懸念; UK economy entering 2026 amid sharp private sector downturn, says CBI | Confederation of British Industry (CBI) | The Guardian; What to Do If ICE Invades Your Neighborhood | WIRED; How to Verify Information Online and Avoid Fake Content |
| Cited Titles | NY原油市場 先物価格 一時1バレル＝100ドル超に 事態悪化懸念; Trump’s re-election may have helped Albanese – but the US war in Iran is creating economic conundrums; Trump’s re-election may have helped Albanese – but the US war in Iran is creating economic conundrums; What to Do If ICE Invades Your Neighborhood | WIRED |

**Failures:**

- answer length: got 400 runes, want >= 600

<details><summary>Answer (preview)</summary>

```
**直接的要因**\n*   **原油供給をめぐる混乱の長期化懸念**: この懸念が、株価の下落や原油価格の変動を引き起こす要因として挙げられています[4]。\n*   **中東での事態**: 米国のイランでの戦争が、経済的な難題（conundrums）を生み出していることが指摘されています[2]。\n\n**構造的背景**\n*   **インフレの再燃**: 中東での紛争以前から要因となっていたインフレが、さらに激化するリスクを抱えていることが指摘されています[2]。\n*   **地政学的緊張**: 米国全土における政治的二極化と高まる緊張が、潜在的なリスクを評価する上での主要な背景として挙げられています[6]。\n\n**不確実性**\n見解が分かれている。提供された情報からは、石油危機を引き起こした単一の真因を特定することは難しく、複数の要因が絡み合っていることが示唆されています。
```

</details>

---

### PASS `simple-temporal-query`

**Query:** 今週のAI関連ニュースは？

**Tags:** temporal, cjk

| Field | Value |
|-------|-------|
| Answer Length | 735 runes |
| Citations | 7 |
| Fallback | false |
| Retrieved Titles | DORAレポートから考える：AIによる生産性を組織的な成果に広げる土台づくり - Visional Engineering Blog; Your Microservices Are Holding Your AI Back. Here's What We Replaced Them With.; Beyond the Cloud: Why the "Edge" is the New Frontier for Engineering - DEV Community; AI could finally pay off for businesses in 2026 - thanks to this, experts say | ZDNET; What the nation's strongest AI regulations change in 2026, according to legal experts | ZDNET; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Why Every AI Presentation Tool Gets It Wrong (And What a Show Really Needs) |
| Cited Titles | DORAレポートから考える：AIによる生産性を組織的な成果に広げる土台づくり - Visional Engineering Blog; Your Microservices Are Holding Your AI Back. Here's What We Replaced Them With.; Beyond the Cloud: Why the "Edge" is the New Frontier for Engineering - DEV Community; AI could finally pay off for businesses in 2026 - thanks to this, experts say | ZDNET; What the nation's strongest AI regulations change in 2026, according to legal experts | ZDNET; AI Agents Are Already Breaking Things — And We've Barely Started - DEV Community; Why Every AI Presentation Tool Gets It Wrong (And What a Show Really Needs) |

<details><summary>Answer (preview)</summary>

```
**直近の動向（2026年4月）**\n*   **AIの活用フェーズの変化と実用化**：2026年に入り、AIは新たな段階に入り、企業が技術を活用して成果を出すことが期待されています。特にAIエージェントやコマースの機会が鍵となると指摘されています[4]。\n*   **技術的なトレンド**：\n    *   「AIを導入するかどうか」という段階から、「導入したAIの価値をいかに最大限に引き出すか」へと関心がシフトしています[1]。\n    *   単に既存のマイクロサービスアーキテクチャにLLMの呼び出しを組み込むだけでは不十分であり、より高度なアプローチが求められています[2]。\n    *   「エッジ」が新たな最前線となっており、単に質問に答えるだけでなく行動を起こす「エージェンティックAI」の台頭が見られます。2026年の目標は「オンデバイスAI」であり、小型化・最適化されたLLMをローカルで実行することが重要視されています[3]。\n    *   AIエージェントがすでに問題を引き起こしている段階であり、これはまだ初期段階であると指摘されています[6]。\n*  ...
```

</details>

---

### PASS `comparison-query`

**Query:** GPT-4oとClaude 3.5の違いは？

**Tags:** comparison, cjk

| Field | Value |
|-------|-------|
| Answer Length | 1109 runes |
| Citations | 7 |
| Fallback | false |
| Retrieved Titles | Claude Sonnet 4.5 Code Review Benchmark; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026; Building an AI-Powered Telegram Bot with Claude API and n8n: A Complete Guide; Claude vs ChatGPT for Coding: The Real Differences in 2026; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026 |
| Cited Titles | Claude Sonnet 4.5 Code Review Benchmark; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026; Building an AI-Powered Telegram Bot with Claude API and n8n: A Complete Guide; Claude vs ChatGPT for Coding: The Real Differences in 2026; Claude Sonnet 4.5 Code Review Benchmark; Claude vs ChatGPT for Coding: The Real Differences in 2026 |

<details><summary>Answer (preview)</summary>

```
**共通点**\n*   両モデルとも、複雑なレビュータスクにおいては、より小規模なモデルよりも高い性能を示す[1]。\n*   Claudeは、複雑な指示に従う際の推論エンジンとして、GPTよりもニュアンスに富んでいるという指摘がある[4]。\n\n**相違点**\n*   **コードレビューの総合性能**: Claude Sonnet 4.5とGPT-4oは、コードレビュー全体においては比較可能な性能を示す[1]。\n*   **バグ検出と提案の質**: Claudeは、ニュアンスのあるバグ検出に優れており、より実用的な提案を生成する傾向がある[1]。一方、GPT-4oは、React固有のXSS検出においてClaudeを上回る結果を示しており、Claudeの訓練がフロントエンドのセキュリティパターンにやや重点を置いていない可能性が示唆されている[2]。\n*   **速度**: GPT-4oは、全体的、および特に速度面でChatGPT（GPT-4o）が速いと指摘されている[3, 5]。Claude Sonnet 4.5は、標準モードではGPT-4oより約20%遅く、拡張思考を有効に...
```

</details>

---

### FAIL `synthesis-broad-topic`

**Query:** そもそもニューヨークと芸術のかかわりは？

**Tags:** synthesis, cjk, known-failure-2026-04-03

| Field | Value |
|-------|-------|
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | retrieval quality insufficient: context relevance too low |

**Failures:**

- intent: got "", want "synthesis"
- answer length: got 0 runes, want >= 800
- citations required but none provided

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
| Fallback | true |
| Fallback Reason | retrieval quality insufficient: context relevance too low |

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
| Answer Length | 0 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | retrieval quality insufficient: context relevance too low |

**Failures:**

- intent: got "", want "topic_deep_dive"
- answer length: got 0 runes, want >= 500
- citations required but none provided

---

### FAIL `follow-up-different-perspective`

**Query:** 別の観点では？

**Conversation History:**

- **user:** 生成AIの著作権問題は？
- **assistant:** 著作権法の観点からは、学習データの利用がフェアユースに該当するか議論が続いています。

**Tags:** follow-up, ambiguous

| Field | Value |
|-------|-------|
| Answer Length | 1534 runes |
| Citations | 0 |
| Fallback | true |
| Fallback Reason | answer quality insufficient: short answer with quality issues |
| Retrieved Titles | noteとGENIACが切り拓く、AI時代のコンテンツ流通革命──RAGデータベースで実現する公正な対価還元とは; Fighting Renewed Attempts to Make ISPs Copyright Cops: 2025 in Review | Electronic Frontier Foundation; The Anthropic-DOD Conflict: Privacy Protections Shouldn’t Depend On the Decisions of a Few Powerful People | Electronic Frontier Foundation; Anthropic Epic Fail: What Secrets Are Hiding in Claude Code Source?; 生成AIの気になるニュース・記事（11/9~11/16）｜分析屋; 【社内SE向け】Claude Code for VS Codeは企業で使える?IPAガイドラインとセキュリティ仕様を徹底調査; EFF to Court: Don’t Make Embedding Illegal | Electronic Frontier Foundation |

**Failures:**

- citations required but none provided

<details><summary>Answer (preview)</summary>

```
**導入**\n生成AIの著作権問題は、AIがインターネット上の膨大なデータ、すなわち著作権で保護されたコンテンツを学習・参照する過程で生じる、極めて多角的かつ複雑な法的・倫理的な論点群です。主要な争点は、学習利用が著作権法上の許容範囲内か否か、そして、もし利用された場合、権利者への対価をどのように還元すべきかという点に集約されます[1]。\n\n**多面的分析**\n\n**1. 学習データ利用の適法性（フェアユース論）**\n著作権保護されたコンテンツをAIが学習に利用する行為が、現行法上の「フェアユース（公正利用）」原則に該当するかどうかが、最も重要な法的な論点の一つです[5]。過去の技術革新、例えばインターネット検索エンジンやコピー機などに対しても、著作権所有者から権利侵害の主張がなされてきましたが[3][7]、AIの学習利用がこれにどう当てはまるかについて、今後の法的な議論が続いています[5]。また、AIが生成したコードをオープンソースプロジェクトにコミットする際にも、透明性や帰属の問題がコミュニティ内で議論されています[4]。\n\n**2. 権利者への対価還元と流通の公正...
```

</details>

---

## Analysis

### Systemic Issues

| Issue | Count | Impact |
|-------|-------|--------|
| Citation not returned in stream | 6/12 | done event の citations が空。rag-orchestrator → frontend の citation 伝搬に問題 |
| Intent not exposed in response | 2/12 | StreamChat が intent debug 情報を返していない |
| Answer too short | 6/12 | follow-up・topic-shift で retrieval が不十分 |
| Clarification not triggered | 1/12 | ConversationPlanner が曖昧クエリで clarification を返さない |
