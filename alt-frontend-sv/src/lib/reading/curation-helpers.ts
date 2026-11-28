import {
	AnnotationStance,
	type ArticleAnnotationItem,
	type ThemeCurationItem,
	ThemeCurationKind,
	type ThemeSentenceItem,
} from "#lib/connect/reading.js";

export const MAX_CURATION_TEXT_LENGTH = 500;

export interface AnnotationStanceOption {
	stance: AnnotationStance;
	label: string;
	placeholder: string;
}

export const ANNOTATION_STANCE_OPTIONS: readonly AnnotationStanceOption[] = [
	{
		stance: AnnotationStance.SURPRISE,
		label: "Surprise",
		placeholder: "What was unexpected here?",
	},
	{
		stance: AnnotationStance.QUESTION,
		label: "Question",
		placeholder: "What question did this raise?",
	},
	{
		stance: AnnotationStance.CONTRADICTION,
		label: "Counter",
		placeholder: "What conflicts with your prior view?",
	},
	{
		stance: AnnotationStance.INSIGHT,
		label: "Insight",
		placeholder: "What is the key takeaway?",
	},
];

export function stanceLabel(stance: AnnotationStance): string {
	switch (stance) {
		case AnnotationStance.SURPRISE:
			return "Surprise";
		case AnnotationStance.QUESTION:
			return "Question";
		case AnnotationStance.CONTRADICTION:
			return "Counter";
		case AnnotationStance.INSIGHT:
			return "Insight";
		default:
			return "Note";
	}
}

/**
 * Strips non-whitespace control characters, trims, and clamps by Unicode code
 * points so client-side length bounds match the Go server's rune count limit.
 */
export function clampMicroText(
	raw: string,
	max = MAX_CURATION_TEXT_LENGTH,
): string {
	let cleaned = "";
	for (const ch of raw) {
		const code = ch.codePointAt(0) ?? 0;
		const isDisallowedControl =
			(code < 0x20 && code !== 0x09 && code !== 0x0a) ||
			(code >= 0x7f && code <= 0x9f);
		if (!isDisallowedControl) {
			cleaned += ch;
		}
	}
	const trimmed = cleaned.trim();
	const chars = [...trimmed];
	if (chars.length <= max) {
		return trimmed;
	}
	return chars.slice(0, max).join("").trim();
}

// FIPS 180-4 SHA-256 initial hash values and round constants.
const SHA256_K = new Uint32Array([
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1,
	0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
	0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786,
	0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147,
	0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
	0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
	0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a,
	0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
	0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

function rotr(x: number, n: number): number {
	return (x >>> n) | (x << (32 - n));
}

function sha256Bytes(bytes: Uint8Array): Uint32Array {
	const bitLen = bytes.length * 8;
	const paddedLen = Math.ceil((bytes.length + 9) / 64) * 64;
	const buf = new Uint8Array(paddedLen);
	buf.set(bytes);
	buf[bytes.length] = 0x80;

	const view = new DataView(buf.buffer);
	const highBits = Math.floor(bitLen / 0x100000000);
	const lowBits = bitLen >>> 0;
	view.setUint32(paddedLen - 8, highBits, false);
	view.setUint32(paddedLen - 4, lowBits, false);

	let h0 = 0x6a09e667;
	let h1 = 0xbb67ae85;
	let h2 = 0x3c6ef372;
	let h3 = 0xa54ff53a;
	let h4 = 0x510e527f;
	let h5 = 0x9b05688c;
	let h6 = 0x1f83d9ab;
	let h7 = 0x5be0cd19;

	const w = new Uint32Array(64);

	for (let offset = 0; offset < paddedLen; offset += 64) {
		for (let i = 0; i < 16; i++) {
			w[i] = view.getUint32(offset + i * 4, false);
		}
		for (let i = 16; i < 64; i++) {
			const w15 = w[i - 15] ?? 0;
			const w2 = w[i - 2] ?? 0;
			const w16 = w[i - 16] ?? 0;
			const w7 = w[i - 7] ?? 0;
			const s0 = rotr(w15, 7) ^ rotr(w15, 18) ^ (w15 >>> 3);
			const s1 = rotr(w2, 17) ^ rotr(w2, 19) ^ (w2 >>> 10);
			w[i] = (w16 + s0 + w7 + s1) >>> 0;
		}

		let a = h0;
		let b = h1;
		let c = h2;
		let d = h3;
		let e = h4;
		let f = h5;
		let g = h6;
		let h = h7;

		for (let i = 0; i < 64; i++) {
			const S1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25);
			const ch = (e & f) ^ (~e & g);
			const k = SHA256_K[i] ?? 0;
			const wi = w[i] ?? 0;
			const temp1 = (h + S1 + ch + k + wi) >>> 0;
			const S0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22);
			const maj = (a & b) ^ (a & c) ^ (b & c);
			const temp2 = (S0 + maj) >>> 0;

			h = g;
			g = f;
			f = e;
			e = (d + temp1) >>> 0;
			d = c;
			c = b;
			b = a;
			a = (temp1 + temp2) >>> 0;
		}

		h0 = (h0 + a) >>> 0;
		h1 = (h1 + b) >>> 0;
		h2 = (h2 + c) >>> 0;
		h3 = (h3 + d) >>> 0;
		h4 = (h4 + e) >>> 0;
		h5 = (h5 + f) >>> 0;
		h6 = (h6 + g) >>> 0;
		h7 = (h7 + h) >>> 0;
	}

	return new Uint32Array([h0, h1, h2, h3, h4, h5, h6, h7]);
}

/**
 * Returns a deterministic 16-hex-char SHA-256 prefix matching Go's
 * `domain.HashTargetSentence` so pinned/dismissed/resolved curations bind
 * stably to theme sentences across renders and versions.
 */
export function hashTargetSentence(text: string): string {
	const normalized = text.trim();
	if (!normalized) return "";
	const bytes = new TextEncoder().encode(normalized);
	const digest = sha256Bytes(bytes);
	const d0 = digest[0] ?? 0;
	const d1 = digest[1] ?? 0;
	return d0.toString(16).padStart(8, "0") + d1.toString(16).padStart(8, "0");
}

export interface VisibleKeyPointRow {
	item: ThemeSentenceItem;
	hash: string;
	pinCuration: ThemeCurationItem | null;
	pinCurations: ThemeCurationItem[];
}

export interface DismissedKeyPointRow {
	item: ThemeSentenceItem;
	hash: string;
	dismissCuration: ThemeCurationItem;
	dismissCurations: ThemeCurationItem[];
}

export interface ThemeOpenQuestionRow {
	text: string;
	sourceArticleIds: string[];
	hash: string;
	isUserAdded: boolean;
	userCurationId: string | null;
	resolvedCuration: ThemeCurationItem | null;
	resolvedCurations: ThemeCurationItem[];
}

export interface ThemeCurationView {
	visibleKeyPoints: VisibleKeyPointRow[];
	dismissedKeyPoints: DismissedKeyPointRow[];
	openQuestionRows: ThemeOpenQuestionRow[];
	seedNotes: ThemeCurationItem[];
	promotedInsights: ThemeCurationItem[];
}

function matchesTarget(
	curation: ThemeCurationItem,
	hash: string,
	text: string,
): boolean {
	if (curation.targetHash && hash && curation.targetHash === hash) {
		return true;
	}
	const target = curation.targetText.trim();
	return target !== "" && target === text.trim();
}

export function deriveThemeCurationView(
	keyPoints: readonly ThemeSentenceItem[],
	openQuestions: readonly ThemeSentenceItem[],
	curations: readonly ThemeCurationItem[],
): ThemeCurationView {
	const pins = curations.filter(
		(c) => c.kind === ThemeCurationKind.PIN_KEY_POINT,
	);
	const dismissals = curations.filter(
		(c) => c.kind === ThemeCurationKind.DISMISS_KEY_POINT,
	);
	const resolutions = curations.filter(
		(c) => c.kind === ThemeCurationKind.RESOLVE_QUESTION,
	);
	const userQuestions = curations.filter(
		(c) => c.kind === ThemeCurationKind.ADD_QUESTION,
	);
	const seedNotes = curations.filter(
		(c) => c.kind === ThemeCurationKind.ADD_SEED_NOTE,
	);
	const promotedInsights = curations.filter(
		(c) => c.kind === ThemeCurationKind.PROMOTE_AUGUR_INSIGHT,
	);

	const pinnedRows: VisibleKeyPointRow[] = [];
	const unpinnedRows: VisibleKeyPointRow[] = [];
	const dismissedKeyPoints: DismissedKeyPointRow[] = [];

	for (const item of keyPoints) {
		const hash = hashTargetSentence(item.text);
		const matchingDismissals = dismissals.filter((c) =>
			matchesTarget(c, hash, item.text),
		);
		const firstDismiss = matchingDismissals[0] ?? null;
		if (firstDismiss) {
			dismissedKeyPoints.push({
				item,
				hash,
				dismissCuration: firstDismiss,
				dismissCurations: matchingDismissals,
			});
			continue;
		}
		const matchingPins = pins.filter((c) => matchesTarget(c, hash, item.text));
		const firstPin = matchingPins[0] ?? null;
		if (firstPin) {
			pinnedRows.push({
				item,
				hash,
				pinCuration: firstPin,
				pinCurations: matchingPins,
			});
		} else {
			unpinnedRows.push({
				item,
				hash,
				pinCuration: null,
				pinCurations: [],
			});
		}
	}

	const openQuestionRows: ThemeOpenQuestionRow[] = [];
	for (const item of openQuestions) {
		const hash = hashTargetSentence(item.text);
		const matchingResolutions = resolutions.filter((c) =>
			matchesTarget(c, hash, item.text),
		);
		openQuestionRows.push({
			text: item.text,
			sourceArticleIds: [...item.sourceArticleIds],
			hash,
			isUserAdded: false,
			userCurationId: null,
			resolvedCuration: matchingResolutions[0] ?? null,
			resolvedCurations: matchingResolutions,
		});
	}
	for (const q of userQuestions) {
		const text = q.targetText.trim();
		if (!text) continue;
		const hash = q.targetHash || hashTargetSentence(text);
		const matchingResolutions = resolutions.filter((c) =>
			matchesTarget(c, hash, text),
		);
		openQuestionRows.push({
			text,
			sourceArticleIds: [],
			hash,
			isUserAdded: true,
			userCurationId: q.curationId,
			resolvedCuration: matchingResolutions[0] ?? null,
			resolvedCurations: matchingResolutions,
		});
	}

	return {
		visibleKeyPoints: [...pinnedRows, ...unpinnedRows],
		dismissedKeyPoints,
		openQuestionRows,
		seedNotes,
		promotedInsights,
	};
}

export function buildThemeAugurHref(params: {
	question: string;
	themeId: string;
	themeTitle?: string | null;
}): string {
	const search = new URLSearchParams();
	const q = clampMicroText(params.question);
	if (q) search.set("q", q);
	const title = params.themeTitle?.trim();
	if (title) {
		search.set("context", title);
		search.set("themeTitle", title);
	}
	if (params.themeId.trim()) {
		search.set("themeId", params.themeId.trim());
	}
	return `/augur?${search.toString()}`;
}

export function buildArticleAugurHref(params: {
	question: string;
	articleTitle?: string | null;
	articleId?: string | null;
}): string {
	const search = new URLSearchParams();
	const q = clampMicroText(params.question);
	if (q) search.set("q", q);
	const title = params.articleTitle?.trim();
	if (title) search.set("context", title);
	const articleId = params.articleId?.trim();
	if (articleId) search.set("articleId", articleId);
	return `/augur?${search.toString()}`;
}

const RECALL_PRIORITY_STANCES = new Set<AnnotationStance>([
	AnnotationStance.QUESTION,
	AnnotationStance.CONTRADICTION,
	AnnotationStance.INSIGHT,
]);

export function selectRecallAnnotations(
	annotations: readonly ArticleAnnotationItem[],
	limit = 3,
): ArticleAnnotationItem[] {
	if (limit <= 0 || annotations.length === 0) return [];
	const prioritized = annotations.filter((a) =>
		RECALL_PRIORITY_STANCES.has(a.stance),
	);
	const remainder = annotations.filter(
		(a) => !RECALL_PRIORITY_STANCES.has(a.stance),
	);
	return [...prioritized, ...remainder].slice(0, limit);
}
