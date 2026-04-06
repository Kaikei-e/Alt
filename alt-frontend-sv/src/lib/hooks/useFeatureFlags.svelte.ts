/**
 * Feature flags have been removed. All features are always enabled.
 * This hook is retained as a no-op for backward compatibility.
 */
export function useFeatureFlags() {
	return {
		get flags() {
			return [];
		},
		get knowledgeHomeEnabled() {
			return true;
		},
		get trackingEnabled() {
			return true;
		},
		get projectionV2Enabled() {
			return true;
		},
		setFlags: (_: unknown) => {},
		isEnabled: (_: string) => true,
	};
}
