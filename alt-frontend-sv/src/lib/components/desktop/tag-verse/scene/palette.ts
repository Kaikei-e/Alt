export const ALT_PAPER_DEFAULTS = {
	paper: "#faf9f7",
	ink: "#1a1a1a",
	accent: "#2f4f4f",
} as const;

export interface TagVersePalette {
	paper: string;
	labelInk: string;
	accent: string;
	ramp: [string, string, string, string, string];
}

type RgbTuple = [number, number, number];

const RAMP_RATIOS: readonly [number, number, number, number, number] = [
	0.5, 0.625, 0.75, 0.875, 1.0,
];

const DEFAULT_PAPER_RGB: RgbTuple = [250, 249, 247];
const DEFAULT_INK_RGB: RgbTuple = [26, 26, 26];
const DEFAULT_ACCENT_RGB: RgbTuple = [47, 79, 79];

const HEX_SHORT_RE = /^#([0-9a-f]{3}|[0-9a-f]{4})$/i;
const HEX_LONG_RE = /^#([0-9a-f]{6}|[0-9a-f]{8})$/i;
const RGB_FN_RE =
	/^rgba?\(\s*(\d{1,3})\s*(?:,|\s)\s*(\d{1,3})\s*(?:,|\s)\s*(\d{1,3})(?:\s*(?:,|\/)\s*[\d.]+%?)?\s*\)$/i;

function parseRgb(raw: string | null | undefined): RgbTuple | null {
	if (!raw) return null;
	const trimmed = raw.trim();
	if (
		!trimmed ||
		trimmed.includes("gradient") ||
		trimmed.includes("var(") ||
		trimmed.includes("url(")
	) {
		return null;
	}

	const shortMatch = HEX_SHORT_RE.exec(trimmed);
	if (shortMatch?.[1]) {
		const hex = shortMatch[1];
		const r = Number.parseInt((hex[0] ?? "0") + (hex[0] ?? "0"), 16);
		const g = Number.parseInt((hex[1] ?? "0") + (hex[1] ?? "0"), 16);
		const b = Number.parseInt((hex[2] ?? "0") + (hex[2] ?? "0"), 16);
		return [r, g, b];
	}

	const longMatch = HEX_LONG_RE.exec(trimmed);
	if (longMatch?.[1]) {
		const hex = longMatch[1];
		const r = Number.parseInt(hex.slice(0, 2), 16);
		const g = Number.parseInt(hex.slice(2, 4), 16);
		const b = Number.parseInt(hex.slice(4, 6), 16);
		return [r, g, b];
	}

	const rgbMatch = RGB_FN_RE.exec(trimmed);
	if (rgbMatch?.[1] && rgbMatch[2] && rgbMatch[3]) {
		const r = Number.parseInt(rgbMatch[1], 10);
		const g = Number.parseInt(rgbMatch[2], 10);
		const b = Number.parseInt(rgbMatch[3], 10);
		if (r <= 255 && g <= 255 && b <= 255) {
			return [r, g, b];
		}
	}

	return null;
}

function clampByte(n: number): number {
	return Math.max(0, Math.min(255, Math.round(n)));
}

function rgbToHex([r, g, b]: RgbTuple): string {
	const hr = clampByte(r).toString(16).padStart(2, "0");
	const hg = clampByte(g).toString(16).padStart(2, "0");
	const hb = clampByte(b).toString(16).padStart(2, "0");
	return `#${hr}${hg}${hb}`;
}

function mixRgb(paper: RgbTuple, ink: RgbTuple, inkRatio: number): RgbTuple {
	const w = Math.max(0, Math.min(1, inkRatio));
	return [
		clampByte(paper[0] * (1 - w) + ink[0] * w),
		clampByte(paper[1] * (1 - w) + ink[1] * w),
		clampByte(paper[2] * (1 - w) + ink[2] * w),
	];
}

function channelToLinear(byteValue: number): number {
	const cs = clampByte(byteValue) / 255;
	return cs <= 0.04045 ? cs / 12.92 : ((cs + 0.055) / 1.055) ** 2.4;
}

function luminanceFromRgb([r, g, b]: RgbTuple): number {
	return (
		0.2126 * channelToLinear(r) +
		0.7152 * channelToLinear(g) +
		0.0722 * channelToLinear(b)
	);
}

function contrastFromRgb(a: RgbTuple, b: RgbTuple): number {
	const l1 = luminanceFromRgb(a);
	const l2 = luminanceFromRgb(b);
	return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
}

export function relativeLuminance(hexOrCss: string): number {
	const rgb = parseRgb(hexOrCss);
	if (!rgb) {
		throw new Error(`Unparseable CSS colour: "${hexOrCss}"`);
	}
	return luminanceFromRgb(rgb);
}

export function contrastRatio(colorA: string, colorB: string): number {
	const l1 = relativeLuminance(colorA);
	const l2 = relativeLuminance(colorB);
	const lighter = Math.max(l1, l2);
	const darker = Math.min(l1, l2);
	return (lighter + 0.05) / (darker + 0.05);
}

function buildRamp(
	paperRgb: RgbTuple,
	inkRgb: RgbTuple,
): [string, string, string, string, string] {
	return [
		rgbToHex(mixRgb(paperRgb, inkRgb, RAMP_RATIOS[0])),
		rgbToHex(mixRgb(paperRgb, inkRgb, RAMP_RATIOS[1])),
		rgbToHex(mixRgb(paperRgb, inkRgb, RAMP_RATIOS[2])),
		rgbToHex(mixRgb(paperRgb, inkRgb, RAMP_RATIOS[3])),
		rgbToHex(mixRgb(paperRgb, inkRgb, RAMP_RATIOS[4])),
	];
}

export function buildPalette(
	paper?: string | null,
	ink?: string | null,
	accent?: string | null,
): TagVersePalette {
	let paperRgb = parseRgb(paper) ?? DEFAULT_PAPER_RGB;
	let inkRgb = parseRgb(ink) ?? DEFAULT_INK_RGB;
	let accentRgb = parseRgb(accent) ?? DEFAULT_ACCENT_RGB;

	if (contrastFromRgb(inkRgb, paperRgb) < 4.5) {
		inkRgb = DEFAULT_INK_RGB;
		if (contrastFromRgb(inkRgb, paperRgb) < 4.5) {
			paperRgb = DEFAULT_PAPER_RGB;
		}
	}

	const lightestNodeRgb = mixRgb(paperRgb, inkRgb, RAMP_RATIOS[0]);
	if (contrastFromRgb(lightestNodeRgb, paperRgb) < 3.0) {
		paperRgb = DEFAULT_PAPER_RGB;
		inkRgb = DEFAULT_INK_RGB;
	}

	if (contrastFromRgb(accentRgb, paperRgb) < 3.0) {
		accentRgb = DEFAULT_ACCENT_RGB;
		if (contrastFromRgb(accentRgb, paperRgb) < 3.0) {
			paperRgb = DEFAULT_PAPER_RGB;
			inkRgb = DEFAULT_INK_RGB;
		}
	}

	return {
		paper: rgbToHex(paperRgb),
		labelInk: rgbToHex(inkRgb),
		accent: rgbToHex(accentRgb),
		ramp: buildRamp(paperRgb, inkRgb),
	};
}

export function readPaletteFromElement(
	el?: HTMLElement | null,
): TagVersePalette {
	if (
		!el ||
		typeof window === "undefined" ||
		typeof window.getComputedStyle !== "function"
	) {
		return buildPalette();
	}
	const styles = window.getComputedStyle(el);
	return buildPalette(
		styles.getPropertyValue("--app-bg"),
		styles.getPropertyValue("--text-primary"),
		styles.getPropertyValue("--interactive-text"),
	);
}
