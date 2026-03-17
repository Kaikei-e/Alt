/**
 * Headless composable for Knowledge Home feature flag management.
 * Reads flag states from the GetKnowledgeHome response.
 */
import type { FeatureFlagData } from "$lib/connect/knowledge_home";

export function useFeatureFlags() {
	let flags = $state<FeatureFlagData[]>([]);

	const setFlags = (newFlags: FeatureFlagData[]) => {
		flags = newFlags;
	};

	const isEnabled = (name: string): boolean => {
		const flag = flags.find((f) => f.name === name);
		return flag?.enabled ?? false;
	};

	return {
		get flags() {
			return flags;
		},
		get knowledgeHomeEnabled() {
			return isEnabled("enable_knowledge_home_page");
		},
		get trackingEnabled() {
			return isEnabled("enable_knowledge_home_tracking");
		},
		get projectionV2Enabled() {
			return isEnabled("enable_knowledge_home_projection_v2");
		},
		setFlags,
		isEnabled,
	};
}
