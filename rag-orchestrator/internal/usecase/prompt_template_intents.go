package usecase

import "strings"

// --- General Template ---

type generalTemplate struct{}

func (t *generalTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## 回答構造\n")
	sb.WriteString("1. **概要**: 結論と全体像を2-3文で説明\n")
	sb.WriteString("2. **詳細**: 具体的な事実・データ・事例（引用[番号]付き）\n")
	sb.WriteString("3. **まとめ**: 重要ポイントと今後の展望\n\n")
	sb.WriteString(lengthFloor(600))

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *generalTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>EUのAI規制法案の影響は？</query>\n")
	sb.WriteString("<answer>{\"answer\":\"## 概要\\nEUのAI規制法案はリスクに応じてAIシステムを分類し、高リスク用途に事前の適合評価と透明性義務を課す枠組みである[1]。\\n\\n## 詳細\\n禁止用途・高リスク・限定リスク・最小リスクの4段階で義務が変わり、採用や与信の判定に使う高リスク用途には文書化とログ保持が求められる[1]。違反時には全世界売上高に連動した制裁金が定められており、域外の企業も域内で提供する場合は対象になる[2]。\\n\\n## まとめ\\n影響が大きいのは高リスク領域の事業者で、適合評価の体制整備が当面の課題になる[1][2]。\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"法案の枠組み\"},{\"chunk_id\":\"2\",\"reason\":\"制裁と適用範囲\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *generalTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *generalTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- Causal Template ---

type causalTemplate struct{}

func (t *causalTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## 因果分析\n")
	sb.WriteString("回答は以下の3構成要素で構造化すること:\n")
	sb.WriteString("1. **直接的要因**: トリガーとなった出来事 [引用付き]\n")
	sb.WriteString("2. **構造的背景**: 長期的な要因・制度的背景 [引用付き]\n")
	sb.WriteString("3. **不確実性**: 根拠不十分な点、見解が分かれる点\n\n")
	sb.WriteString("複数の要因を分離して記述し、単一原因に帰結させないこと。\n")
	sb.WriteString("ソースが収束しない場合は「見解が分かれている」と明記すること。\n")
	sb.WriteString(lengthFloor(800))

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *causalTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>なぜ石油危機が起きた？</query>\n")
	sb.WriteString("<answer>{\"answer\":\"**直接的要因**\\n同年10月の戦争を受けて産油国側が減産と輸出制限を決定したことが引き金となった[1]。公示価格は数か月で数倍に引き上げられ、輸入国は調達先の確保と配給に追われた[1]。\\n\\n**構造的背景**\\n先進国のエネルギー消費が中東産原油に集中し、代替供給源や備蓄制度が整っていなかった[2]。産油国では資源国有化の流れが強まり、価格決定権が消費国側から移りつつあった[2]。\\n\\n**不確実性**\\n減産の政治的意図と経済的意図のどちらが主因かは論者によって評価が分かれている[2]。\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"直接要因\"},{\"chunk_id\":\"2\",\"reason\":\"構造的背景\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *causalTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *causalTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- Synthesis Template ---

type synthesisTemplate struct{}

func (t *synthesisTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## 概念的合成\n")
	sb.WriteString("ユーザーは広範なテーマの包括的な理解を求めています。\n")
	sb.WriteString("回答は以下の構造で作成すること:\n")
	sb.WriteString("1. **導入**: テーマの概要と主要な側面を2-3文で提示\n")
	sb.WriteString("2. **多面的分析**: 3つ以上の異なる側面から論じること（各側面に引用[番号]付き）\n")
	sb.WriteString("3. **相互関係**: 側面間のつながりや影響関係\n")
	sb.WriteString("4. **現状と展望**: 最新の動向と今後の方向性\n\n")
	sb.WriteString("1つの側面に偏らずバランスよく複数の視点を提供すること。\n")
	sb.WriteString("回答は1200文字以上で、具体的な事実・データ・事例を含むこと。\n\n")

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *synthesisTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>AIと社会の関係について</query>\n")
	sb.WriteString("<answer>{\"answer\":\"**導入**\\nAIは社会のあらゆる側面に...[1]\\n\\n**多面的分析**\\n**経済的影響**\\n...[2]\\n\\n**倫理的課題**\\n...[3]\\n\\n**相互関係**\\n...[1][2]\\n\\n**現状と展望**\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"概要\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *synthesisTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *synthesisTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- Comparison Template ---

type comparisonTemplate struct{}

func (t *comparisonTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## 比較分析\n")
	sb.WriteString("両者を公平に比較し、以下の構造で回答すること:\n")
	sb.WriteString("1. **共通点**: 両者に共通する要素\n")
	sb.WriteString("2. **相違点**: 各項目の違いを対比して記述\n")
	sb.WriteString("3. **評価**: 長所・短所を併記し、一方に偏らないこと\n\n")
	sb.WriteString(lengthFloor(600))

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *comparisonTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>AとBの違いは？</query>\n")
	sb.WriteString("<answer>{\"answer\":\"**共通点**\\n両者ともクラウド上で動作し、利用量に応じた従量課金を採用している[1]。初期費用が不要で、小規模から試せる点も同じである[1]。\\n\\n**相違点**\\nAはマネージド運用で設定項目が少なく、小規模チームでも短期間で立ち上げられる[1]。一方Bは細かなチューニングが可能で、大規模データでは処理コストを抑えやすい[2]。サポートはAが標準で含まれるのに対し、Bは上位プランに限られる[2]。\\n\\n**評価**\\n運用負荷を下げたいならA、コストと制御性を優先するならBが向くが、Bは学習コストを見込む必要がある[1][2]。\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"比較対象A\"},{\"chunk_id\":\"2\",\"reason\":\"比較対象B\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *comparisonTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *comparisonTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- Temporal Template ---

type temporalTemplate struct{}

func (t *temporalTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## 時系列分析\n")
	sb.WriteString("最新の情報を優先して回答すること。\n")
	sb.WriteString("日付・時期を明記し、時系列順に整理すること。\n")
	sb.WriteString("主要な転換点を特定し、各段階の因果関係を記述すること。\n")
	sb.WriteString(lengthFloor(600))

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *temporalTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>最新の動向は？</query>\n")
	sb.WriteString("<answer>{\"answer\":\"**直近の動向**\\n2026年3月に改正案が可決され、施行日は同年10月と定められた[1]。事業者向けのガイドラインも同じ月に公表された[1]。\\n\\n**経緯**\\n2025年前半に有識者会議が論点を整理し、同年秋に法案が提出された[2]。審議では中小事業者の負担が争点となり、経過措置が追加された[2]。\\n\\n**今後の見通し**\\n施行から1年で運用状況の見直しが予定されており、対象範囲の拡大が議論される見込みである[1]。\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"最新情報\"},{\"chunk_id\":\"2\",\"reason\":\"経緯\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *temporalTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *temporalTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- FactCheck Template ---

type factCheckTemplate struct{}

func (t *factCheckTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## ファクトチェック\n")
	sb.WriteString("「主張」「根拠」「判定」の3段構成で回答すること。\n")
	sb.WriteString("1. **主張**: 検証対象の主張を明記\n")
	sb.WriteString("2. **根拠**: コンテキストから裏付ける/反証する情報 [引用付き]\n")
	sb.WriteString("3. **判定**: 「支持される」「一部支持」「反証される」「判定不能」のいずれか\n\n")

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *factCheckTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>この主張は正しい？</query>\n")
	sb.WriteString("<answer>{\"answer\":\"**主張**\\n...[1]\\n\\n**根拠**\\n...[1][2]\\n\\n**判定**\\n一部支持される。...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"根拠\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *factCheckTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *factCheckTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- DeepDive Template ---

type deepDiveTemplate struct{}

func (t *deepDiveTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	sb.WriteString("## 深掘り分析\n")
	sb.WriteString("背景・詳細・影響を包括的に解説すること。\n")
	sb.WriteString("基本概念から応用まで段階的に説明すること。\n")
	sb.WriteString("回答は800文字以上で、具体的な事実・データを含むこと。\n\n")

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *deepDiveTemplate) writeFewShot(sb *strings.Builder) {
	sb.WriteString("<example>\n<query>詳しく教えて</query>\n")
	sb.WriteString("<answer>{\"answer\":\"**基本概念**\\n...[1]\\n\\n**技術的詳細**\\n...[2]\\n\\n**影響と応用**\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"基本情報\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
}

func (t *deepDiveTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *deepDiveTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}
