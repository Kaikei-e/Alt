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

export function relativeLuminance(_hexOrCss: string): number {
	return 0;
}

export function contrastRatio(_colorA: string, _colorB: string): number {
	return 1;
}

export function buildPalette(
	_paper?: string | null,
	_ink?: string | null,
	_accent?: string | null,
): TagVersePalette {
	return {
		paper: "#000000",
		labelInk: "#000000",
		accent: "#000000",
		ramp: ["#000000", "#000000", "#000000", "#000000", "#000000"],
	};
}

export function readPaletteFromElement(_el?: HTMLElement | null): TagVersePalette {
	return buildPalette();
}
