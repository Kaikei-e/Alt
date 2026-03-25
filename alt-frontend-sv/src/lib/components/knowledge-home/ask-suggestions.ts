/**
 * Context-aware question suggestion pool for AskSheet.
 *
 * 7 categories aligned with Agentic RAG intent types (ADR-568).
 * Questions are selected based on article tags to surface the most
 * relevant prompts per NNGroup best practices.
 */

export const suggestionPool: Record<string, string[]> = {
	summary: [
		"この記事の要点は？",
		"3行でまとめると？",
		"初心者にわかるように説明して",
	],
	deep_dive: [
		"ここでの新しい発見は？",
		"技術的な詳細をもっと教えて",
		"この手法の仕組みは？",
	],
	comparison: [
		"従来のアプローチとどう違う？",
		"競合技術と比較すると？",
		"メリットとデメリットは？",
	],
	impact: [
		"実務にどう活かせる？",
		"今後の業界への影響は？",
		"日本市場での意味は？",
	],
	critical: ["この主張の根拠は十分？", "反論はある？", "バイアスはないか？"],
	explore: [
		"次に何を読むべき？",
		"関連するトピックは？",
		"この分野の最新動向は？",
	],
	practical: [
		"これを試すには何が必要？",
		"今日からできることは？",
		"学習ロードマップは？",
	],
};

/** Maps article tags to preferred suggestion categories. */
const tagCategoryWeights: Record<string, string[]> = {
	security: ["critical", "comparison"],
	vulnerability: ["critical", "practical"],
	cybersecurity: ["critical", "comparison"],
	ai: ["deep_dive", "comparison"],
	machinelearning: ["deep_dive", "practical"],
	llm: ["deep_dive", "comparison"],
	deeplearning: ["deep_dive", "impact"],
	startup: ["impact", "practical"],
	business: ["impact", "practical"],
	finance: ["impact", "critical"],
	programming: ["comparison", "practical"],
	rust: ["comparison", "deep_dive"],
	go: ["comparison", "deep_dive"],
	python: ["practical", "deep_dive"],
	typescript: ["practical", "comparison"],
	cloud: ["comparison", "practical"],
	devops: ["practical", "impact"],
	blockchain: ["critical", "deep_dive"],
	quantum: ["deep_dive", "explore"],
	research: ["deep_dive", "critical"],
	opensource: ["comparison", "practical"],
};

const allCategories = Object.keys(suggestionPool);

/**
 * Pick 3 context-aware question suggestions based on article tags.
 *
 * 1. Resolve tag → preferred categories (weighted)
 * 2. Select 3 distinct categories (preferred ones first, then random fill)
 * 3. Pick 1 random question per category
 */
export function pickSuggestions(tags: string[] | undefined): string[] {
	const preferred = resolvePreferredCategories(tags);
	const selected = selectCategories(preferred, 3);
	return selected.map((cat) => pickRandom(suggestionPool[cat]));
}

function resolvePreferredCategories(tags: string[] | undefined): string[] {
	if (!tags || tags.length === 0) return [];

	const seen = new Set<string>();
	const result: string[] = [];

	for (const tag of tags) {
		const normalized = tag.toLowerCase().replace(/[^a-z]/g, "");
		const weights = tagCategoryWeights[normalized];
		if (weights) {
			for (const cat of weights) {
				if (!seen.has(cat)) {
					seen.add(cat);
					result.push(cat);
				}
			}
		}
	}

	return result;
}

function selectCategories(preferred: string[], count: number): string[] {
	const selected: string[] = [];
	const used = new Set<string>();

	// First: pick from preferred categories
	for (const cat of preferred) {
		if (selected.length >= count) break;
		if (!used.has(cat)) {
			selected.push(cat);
			used.add(cat);
		}
	}

	// Fill remaining with random categories
	if (selected.length < count) {
		const remaining = allCategories.filter((c) => !used.has(c));
		shuffle(remaining);
		for (const cat of remaining) {
			if (selected.length >= count) break;
			selected.push(cat);
		}
	}

	return selected;
}

function pickRandom<T>(arr: T[]): T {
	return arr[Math.floor(Math.random() * arr.length)];
}

function shuffle<T>(arr: T[]): void {
	for (let i = arr.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[arr[i], arr[j]] = [arr[j], arr[i]];
	}
}
