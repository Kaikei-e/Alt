import type { Page } from "@playwright/test";
import type { Result, RunOptions } from "axe-core";
import { checkA11y, getViolations, injectAxe } from "axe-playwright";

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
 * Freeze CSS animations and transitions before auditing.
 *
 * The feed list fades its rows in on a stagger. axe samples computed styles at
 * one instant, so auditing mid-animation read three different greys off three
 * elements sharing one class (`.dispatch-dateline`: #9c9c9c, #a1a1a1, #b8b7b7)
 * — colours that exist for a few frames and are in nobody's stylesheet. Whether
 * such a run reported a contrast violation came down to timing. Pinning
 * animations to their end state makes the audit a function of the CSS instead.
 */
async function settleAnimations(page: Page): Promise<void> {
	await page.addStyleTag({
		content: `*, *::before, *::after {
			animation-duration: 0s !important;
			animation-delay: 0s !important;
			animation-iteration-count: 1 !important;
			transition-duration: 0s !important;
			transition-delay: 0s !important;
		}`,
	});
}

/**
 * Check page accessibility using axe-core.
 * Throws an error if violations are found.
 */
export async function checkAccessibility(
	page: Page,
	options: A11yOptions = {},
): Promise<void> {
	await settleAnimations(page);
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
	await settleAnimations(page);
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
