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
	sb.WriteString("<answer>{\"answer\":\"## 概要\\nEUのAI規制法案は...[1]\\n\\n## 詳細\\n...[2]\\n\\n## まとめ\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"法案経緯\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
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
	sb.WriteString("ソースが収束しない場合は「見解が分かれている」と明記すること。\n\n")

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
	sb.WriteString("<answer>{\"answer\":\"**直接的要因**\\n中東の地政学的緊張が...[1]\\n\\n**構造的背景**\\n長期的な依存構造...[2]\\n\\n**不確実性**\\n一部の分析では...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"直接要因\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
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
	sb.WriteString("<answer>{\"answer\":\"**共通点**\\n両者とも...[1]\\n\\n**相違点**\\nAは...[1]、一方Bは...[2]\\n\\n**評価**\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"比較対象A\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
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
	sb.WriteString("主要な転換点を特定し、各段階の因果関係を記述すること。\n\n")

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
	sb.WriteString("<answer>{\"answer\":\"**直近の動向**\\n2026年3月に...[1]\\n\\n**経緯**\\n2025年から...[2]\\n\\n**今後の見通し**\\n...\",")
	sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"最新情報\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
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
