import type { Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

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
 * Check page accessibility using axe-core.
 * Throws an error if violations are found.
 */
export async function checkAccessibility(
	page: Page,
	options: A11yOptions = {},
): Promise<void> {
	let axe = new AxeBuilder({ page });

	if (options.disableRules?.length) {
		axe = axe.disableRules(options.disableRules);
	}

	if (options.includeSelector) {
		axe = axe.include(options.includeSelector);
	}

	if (options.excludeSelector) {
		axe = axe.exclude(options.excludeSelector);
	}

	if (options.tags?.length) {
		axe = axe.withTags(options.tags);
	}

	const results = await axe.analyze();

	if (results.violations.length > 0) {
		const violations: A11yViolation[] = results.violations.map((v) => ({
			id: v.id,
			impact: v.impact ?? null,
			description: v.description,
			nodes: v.nodes.length,
			help: v.help,
			helpUrl: v.helpUrl,
		}));

		const summary = violations
			.map(
				(v) =>
					`- ${v.id} (${v.impact}): ${v.description} [${v.nodes} node(s)]`,
			)
			.join("\n");

		throw new Error(
			`Accessibility violations found:\n${summary}\n\nSee detailed report for fixes.`,
		);
	}
}

/**
 * Get accessibility violations without throwing.
 * Useful for reporting or custom assertions.
 */
export async function getAccessibilityViolations(
	page: Page,
	options: A11yOptions = {},
): Promise<A11yViolation[]> {
	let axe = new AxeBuilder({ page });

	if (options.disableRules?.length) {
		axe = axe.disableRules(options.disableRules);
	}

	if (options.includeSelector) {
		axe = axe.include(options.includeSelector);
	}

	if (options.excludeSelector) {
		axe = axe.exclude(options.excludeSelector);
	}

	if (options.tags?.length) {
		axe = axe.withTags(options.tags);
	}

	const results = await axe.analyze();

	return results.violations.map((v) => ({
		id: v.id,
		impact: v.impact ?? null,
		description: v.description,
		nodes: v.nodes.length,
		help: v.help,
		helpUrl: v.helpUrl,
	}));
}
