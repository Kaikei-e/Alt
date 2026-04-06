import type { RecallCandidateData } from "$lib/connect/knowledge_home";

interface HomeRefreshSource {
	fetchData(reset?: boolean, lensId?: string | null): Promise<void>;
	readonly recallCandidates: RecallCandidateData[];
}

interface RecallRefreshTarget {
	setCandidates(data: RecallCandidateData[]): void;
}

export async function refreshHomeWithRecallSync(
	home: HomeRefreshSource,
	recall: RecallRefreshTarget,
	lensId: string | null,
): Promise<void> {
	await home.fetchData(true, lensId);
	recall.setCandidates(home.recallCandidates);
}
