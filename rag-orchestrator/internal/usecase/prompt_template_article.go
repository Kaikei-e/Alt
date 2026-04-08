package usecase

import "strings"

// articleScopedTemplate dispatches to sub-intent-specific prompt structures.
// Each sub-intent produces a focused system prompt (~10-15 lines of instruction).
type articleScopedTemplate struct{}

func (t *articleScopedTemplate) buildSystem(input PromptInput) string {
	var sb strings.Builder
	sb.WriteString(preamble())

	switch input.SubIntentType {
	case SubIntentCritique:
		t.writeCritique(&sb)
	case SubIntentOpinion:
		t.writeOpinion(&sb)
	case SubIntentImplication:
		t.writeImplication(&sb)
	case SubIntentDetail:
		t.writeDetail(&sb)
	case SubIntentRelatedArticles:
		t.writeRelatedArticles(&sb)
	case SubIntentEvidence:
		t.writeEvidence(&sb)
	case SubIntentSummaryRefresh:
		t.writeSummaryRefresh(&sb)
	default:
		t.writeDefault(&sb)
	}

	sb.WriteString(outputFormatBrief())
	t.writeFewShot(&sb, input.SubIntentType)
	if input.LowConfidence {
		sb.WriteString(lowConfidenceNote())
	}
	sb.WriteString(sandwich())
	return sb.String()
}

func (t *articleScopedTemplate) buildUser(input PromptInput) string {
	return buildUserMessage(input)
}

func (t *articleScopedTemplate) estimateSystemTokens() int {
	return estimateTokens(t.buildSystem(PromptInput{}))
}

// --- Sub-intent writers ---

func (t *articleScopedTemplate) writeCritique(sb *strings.Builder) {
	sb.WriteString("## 批判的分析\n")
	sb.WriteString("記事の主張に対する反論・弱点・限界を分析すること。\n")
	sb.WriteString("1. **主張の要約**: 記事の中心的な主張を1-2文で提示\n")
	sb.WriteString("2. **反論・弱点**: 考えられる反論、データの限界、論理的な穴 [引用付き]\n")
	sb.WriteString("3. **見落としている視点**: 記事が扱っていない重要な観点\n\n")
	sb.WriteString("記事の内容を要約するのではなく、批判的に分析すること。\n")
	sb.WriteString("反対の立場からの視点を積極的に提示すること。\n\n")
}

func (t *articleScopedTemplate) writeOpinion(sb *strings.Builder) {
	sb.WriteString("## 評価・意見\n")
	sb.WriteString("記事の内容に対する分析的な評価を提示すること。\n")
	sb.WriteString("1. **評価**: コンテキストに基づく総合的な評価\n")
	sb.WriteString("2. **長所**: 記事の強み・価値ある点 [引用付き]\n")
	sb.WriteString("3. **短所**: 改善の余地がある点 [引用付き]\n\n")
	sb.WriteString("要約ではなく、分析と評価に集中すること。\n\n")
}

func (t *articleScopedTemplate) writeImplication(sb *strings.Builder) {
	sb.WriteString("## 影響・意味合い分析\n")
	sb.WriteString("記事の内容がもたらす影響と意味合いを分析すること。\n")
	sb.WriteString("1. **短期的影響**: 直近に予想される影響 [引用付き]\n")
	sb.WriteString("2. **長期的影響**: 中長期的な構造変化の可能性 [引用付き]\n")
	sb.WriteString("3. **波及範囲**: 影響を受ける領域・関係者\n\n")
	sb.WriteString("記事の要約ではなく、その影響と意味合いの分析に集中すること。\n\n")
}

func (t *articleScopedTemplate) writeDetail(sb *strings.Builder) {
	sb.WriteString("## 技術的詳細\n")
	sb.WriteString("記事の技術的な詳細・メカニズム・具体例に集中すること。\n")
	sb.WriteString("1. **メカニズム**: 仕組み・手順・プロセスの説明 [引用付き]\n")
	sb.WriteString("2. **具体的データ**: 数値・日付・固有名詞を正確に引用 [引用付き]\n")
	sb.WriteString("3. **技術的根拠**: 技術的な裏付け・原理\n\n")
	sb.WriteString("概要の再説明は不要。技術的な詳細に集中すること。\n\n")
}

func (t *articleScopedTemplate) writeRelatedArticles(sb *strings.Builder) {
	sb.WriteString("## 関連記事リスト\n")
	sb.WriteString("コンテキスト内の関連記事をランク付きリストで返すこと。\n")
	sb.WriteString("- 各記事に関連度と理由を1文で付記すること [引用付き]\n")
	sb.WriteString("- 散文形式ではなくリスト形式で回答すること\n")
	sb.WriteString("- テーマ・トピック・時期の類似性で関連度を判断すること\n\n")
}

func (t *articleScopedTemplate) writeEvidence(sb *strings.Builder) {
	sb.WriteString("## 根拠・エビデンス\n")
	sb.WriteString("記事の主張を裏付ける具体的な根拠を提示すること。\n")
	sb.WriteString("1. **直接的根拠**: 主張を直接裏付けるパッセージ [引用付き]\n")
	sb.WriteString("2. **間接的根拠**: 状況的に支持する情報 [引用付き]\n")
	sb.WriteString("3. **根拠の強さ**: 各エビデンスの信頼度を明示\n\n")
	sb.WriteString("記事の要約ではなく、根拠となるパッセージに集中すること。\n\n")
}

func (t *articleScopedTemplate) writeSummaryRefresh(sb *strings.Builder) {
	sb.WriteString("## 記事要約\n")
	sb.WriteString("記事の要点を簡潔にまとめること。\n")
	sb.WriteString("- 重要ポイントを5-7項目に絞ること [引用付き]\n")
	sb.WriteString("- 前回の回答との重複は許容する\n")
	sb.WriteString("- 簡潔さを優先し、冗長にならないこと\n\n")
}

func (t *articleScopedTemplate) writeDefault(sb *strings.Builder) {
	sb.WriteString("## 記事分析\n")
	sb.WriteString("記事の要点と重要な情報を整理して回答すること。\n")
	sb.WriteString("1. **要点**: 記事の主要な主張・発見を簡潔に提示 [引用付き]\n")
	sb.WriteString("2. **背景**: 関連する文脈情報 [引用付き]\n")
	sb.WriteString("3. **注目点**: 特に重要な示唆や今後の展望\n\n")
}

// --- Few-shot examples ---

func (t *articleScopedTemplate) writeFewShot(sb *strings.Builder, subIntent SubIntentType) {
	switch subIntent {
	case SubIntentCritique:
		sb.WriteString("<example>\n<query>この記事の弱点は？</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**主張の要約**\\n記事は...[1]\\n\\n**反論・弱点**\\n1. データが限定的で...[1]\\n2. 反対意見が...[2]\\n\\n**見落としている視点**\\n...\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"主張の根拠\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	case SubIntentOpinion:
		sb.WriteString("<example>\n<query>この記事をどう評価する？</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**評価**\\n全体として...[1]\\n\\n**長所**\\n...[1]\\n\\n**短所**\\n...[2]\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"評価根拠\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	case SubIntentImplication:
		sb.WriteString("<example>\n<query>この記事の影響は？</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**短期的影響**\\n...[1]\\n\\n**長期的影響**\\n...[2]\\n\\n**波及範囲**\\n...\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"影響分析\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	case SubIntentDetail:
		sb.WriteString("<example>\n<query>技術的な詳細は？</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**メカニズム**\\n...[1]\\n\\n**具体的データ**\\n...[2]\\n\\n**技術的根拠**\\n...\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"技術詳細\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	case SubIntentRelatedArticles:
		sb.WriteString("<example>\n<query>関連する記事はある？</query>\n")
		sb.WriteString("<answer>{\"answer\":\"1. **記事A** - テーマが類似...[1]\\n2. **記事B** - 同時期の動向...[2]\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"関連トピック\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	case SubIntentEvidence:
		sb.WriteString("<example>\n<query>根拠は？</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**直接的根拠**\\n...[1]\\n\\n**間接的根拠**\\n...[2]\\n\\n**根拠の強さ**\\n...\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"直接的根拠\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	case SubIntentSummaryRefresh:
		sb.WriteString("<example>\n<query>要約して</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**要点**\\n1. ...[1]\\n2. ...[2]\\n3. ...\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"要点\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	default:
		sb.WriteString("<example>\n<query>この記事について教えて</query>\n")
		sb.WriteString("<answer>{\"answer\":\"**要点**\\n記事は...[1]\\n\\n**背景**\\n...[2]\\n\\n**注目点**\\n...\",")
		sb.WriteString("\"citations\":[{\"chunk_id\":\"1\",\"reason\":\"記事要点\"}],\"fallback\":false,\"reason\":\"\"}</answer>\n</example>\n\n")
	}
}
