/**
 * Feature flag system for gradual Connect-RPC migration rollout
 *
 * Provides a simple way to toggle between REST and Connect-RPC implementations
 * during the migration period. Flags can be controlled via:
 * - Environment variables (for deployment-level control)
 * - LocalStorage (for testing/debugging)
 */

import { browser } from "$app/environment";
import { env } from "$env/dynamic/public";

// =============================================================================
// Feature Flag Types
// =============================================================================

export type FeatureFlag =
	| "use_connect_feeds"
	| "use_connect_articles"
	| "use_connect_rss";

// =============================================================================
// Default Flag Values
// =============================================================================

/**
 * Default flag values - can be overridden by environment or localStorage
 */
const defaultFlags: Record<FeatureFlag, boolean> = {
	use_connect_feeds: false,
	use_connect_articles: false,
	use_connect_rss: false,
};

// =============================================================================
// Environment Variable Mapping
// =============================================================================

/**
 * Maps feature flags to their environment variable names
 */
const envVarMap: Record<FeatureFlag, string> = {
	use_connect_feeds: "PUBLIC_USE_CONNECT_FEEDS",
	use_connect_articles: "PUBLIC_USE_CONNECT_ARTICLES",
	use_connect_rss: "PUBLIC_USE_CONNECT_RSS",
};

// =============================================================================
// Feature Flag Functions
// =============================================================================

/**
 * Checks if a feature flag is enabled
 *
 * Priority order:
 * 1. localStorage override (for testing)
 * 2. Environment variable
 * 3. Default value
 *
 * @param flag - The feature flag to check
 * @returns Whether the feature is enabled
 */
export function isFeatureEnabled(flag: FeatureFlag): boolean {
	// Only check localStorage on client side
	if (browser) {
		const override = localStorage.getItem(`ff_${flag}`);
		if (override !== null) {
			return override === "true";
		}
	}

	// Check environment variable
	const envVar = envVarMap[flag];
	const envValue = env[envVar as keyof typeof env];
	if (envValue !== undefined) {
		return envValue === "true";
	}

	// Fall back to default
	return defaultFlags[flag];
}

/**
 * Sets a feature flag override in localStorage (for testing)
 *
 * @param flag - The feature flag to set
 * @param enabled - Whether the feature should be enabled
 */
export function setFeatureFlag(flag: FeatureFlag, enabled: boolean): void {
	if (browser) {
		localStorage.setItem(`ff_${flag}`, String(enabled));
	}
}

/**
 * Clears a feature flag override from localStorage
 *
 * @param flag - The feature flag to clear
 */
export function clearFeatureFlag(flag: FeatureFlag): void {
	if (browser) {
		localStorage.removeItem(`ff_${flag}`);
	}
}

/**
 * Clears all feature flag overrides from localStorage
 */
export function clearAllFeatureFlags(): void {
	if (browser) {
		for (const flag of Object.keys(defaultFlags) as FeatureFlag[]) {
			localStorage.removeItem(`ff_${flag}`);
		}
	}
}

/**
 * Gets all current feature flag states (for debugging)
 *
 * @returns Object with all flag states and their sources
 */
export function getAllFeatureFlags(): Record<
	FeatureFlag,
	{ enabled: boolean; source: "localStorage" | "env" | "default" }
> {
	const result = {} as Record<
		FeatureFlag,
		{ enabled: boolean; source: "localStorage" | "env" | "default" }
	>;

	for (const flag of Object.keys(defaultFlags) as FeatureFlag[]) {
		let source: "localStorage" | "env" | "default" = "default";
		let enabled = defaultFlags[flag];

		// Check localStorage
		if (browser) {
			const override = localStorage.getItem(`ff_${flag}`);
			if (override !== null) {
				enabled = override === "true";
				source = "localStorage";
			}
		}

		// Check env (only if not overridden by localStorage)
		if (source === "default") {
			const envVar = envVarMap[flag];
			const envValue = env[envVar as keyof typeof env];
			if (envValue !== undefined) {
				enabled = envValue === "true";
				source = "env";
			}
		}

		result[flag] = { enabled, source };
	}

	return result;
}
