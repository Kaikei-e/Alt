/**
 * Factory for evening pulse mock data.
 */

export interface PulseTopic {
	clusterId: number;
	title: string;
	summary: string;
	genre: string;
	articleCount: number;
	topTerms: string[];
}

export interface EveningPulseResponse {
	generatedAt: string;
	status: string;
	topics: PulseTopic[];
}

let topicCounter = 0;

export function buildPulseTopic(
	overrides: Partial<PulseTopic> = {},
): PulseTopic {
	topicCounter++;
	return {
		clusterId: topicCounter,
		title: `Topic ${topicCounter}`,
		summary: `Summary for topic ${topicCounter}`,
		genre: "Technology",
		articleCount: 5,
		topTerms: ["AI", "Web"],
		...overrides,
	};
}

export function buildEveningPulseResponse(
	topics?: PulseTopic[],
): EveningPulseResponse {
	return {
		generatedAt: new Date().toISOString(),
		status: "ready",
		topics: topics ?? [
			buildPulseTopic({ title: "AI Breakthrough" }),
			buildPulseTopic({ title: "Web Standards Update" }),
			buildPulseTopic({ title: "Open Source Trends" }),
		],
	};
}

export function buildQuietDayResponse(): EveningPulseResponse {
	return {
		generatedAt: new Date().toISOString(),
		status: "quiet_day",
		topics: [],
	};
}

export function resetPulseCounter(): void {
	topicCounter = 0;
}
