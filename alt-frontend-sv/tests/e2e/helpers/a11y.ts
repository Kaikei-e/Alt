import type { Page } from "@playwright/test";
import { injectAxe, getViolations, checkA11y } from "axe-playwright";
import type { Result, RunOptions } from "axe-core";

// Type workaround: axe-playwright depends on an older playwright-core version
// which causes type incompatibility. Using 'any' cast is the standard workaround.
// biome-ignore lint/suspicious/noExplicitAny: playwright version mismatch workaround
type AxeCompatiblePage = any;

/**
 * Accessibility testing helper using axe-playwright.
 */

export interface A11yOptions {
	/** Rules to disable (e.g., ['color-contrast', 'region']) */
	disableRules?: string[];
	/** CSS selector to include in the check */
	includeSelector?: string;
	/** CSS selector to exclude from the check */
	excludeSelector?: string;
	/** WCAG tags to check (e.g., ['wcag2a', 'wcag2aa']) */
	tags?: string[];
}

export interface A11yViolation {
	id: string;
	impact: string | null;
	description: string;
	nodes: number;
	help: string;
	helpUrl: string;
}

/**
 * Build axe-core RunOptions from our A11yOptions
 */
function buildAxeOptions(options: A11yOptions): RunOptions {
	const runOptions: RunOptions = {};

	if (options.disableRules?.length) {
		runOptions.rules = options.disableRules.reduce(
			(acc, rule) => {
				acc[rule] = { enabled: false };
				return acc;
			},
			{} as Record<string, { enabled: boolean }>,
		);
	}

	if (options.tags?.length) {
		runOptions.runOnly = {
			type: "tag",
			values: options.tags,
		};
	}

	return runOptions;
}

/**
 * Check page accessibility using axe-core.
 * Throws an error if violations are found.
 */
export async function checkAccessibility(
	page: Page,
	options: A11yOptions = {},
): Promise<void> {
	const axePage = page as AxeCompatiblePage;
	await injectAxe(axePage);

	const context = options.includeSelector || undefined;
	const axeOptions = buildAxeOptions(options);

	// checkA11y throws if there are violations
	await checkA11y(axePage, context, { axeOptions }, false);
}

/**
 * Get accessibility violations without throwing.
 * Useful for reporting or custom assertions.
 */
export async function getAccessibilityViolations(
	page: Page,
	options: A11yOptions = {},
): Promise<A11yViolation[]> {
	const axePage = page as AxeCompatiblePage;
	await injectAxe(axePage);

	const context = options.includeSelector || undefined;
	const axeOptions = buildAxeOptions(options);

	const violations = await getViolations(axePage, context, axeOptions);

	return violations.map((v: Result) => ({
		id: v.id,
		impact: v.impact ?? null,
		description: v.description,
		nodes: v.nodes.length,
		help: v.help,
		helpUrl: v.helpUrl,
	}));
}
