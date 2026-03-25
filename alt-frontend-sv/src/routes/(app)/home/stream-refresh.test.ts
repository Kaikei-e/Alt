import { describe, expect, it, vi } from "vitest";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";
import { refreshHomeWithRecallSync } from "./stream-refresh";

describe("refreshHomeWithRecallSync", () => {
	it("re-injects recall candidates after the home refresh completes", async () => {
		const callOrder: string[] = [];
		const home = {
			recallCandidates: [
				{
					itemKey: "article:1",
					recallScore: 0.9,
					reasons: [],
					firstEligibleAt: "",
					nextSuggestAt: "",
				},
			],
			fetchData: vi.fn(async () => {
				callOrder.push("fetch");
			}),
		};
		const recall = {
			setCandidates: vi.fn(() => {
				callOrder.push("set");
			}),
		};

		await refreshHomeWithRecallSync(home, recall, true, "lens-1");

		expect(home.fetchData).toHaveBeenCalledWith(true, "lens-1");
		expect(recall.setCandidates).toHaveBeenCalledWith(home.recallCandidates);
		expect(callOrder).toEqual(["fetch", "set"]);
	});

	it("syncs empty recall candidates without separate fetch", async () => {
		const home = {
			recallCandidates: [] as RecallCandidateData[],
			fetchData: vi.fn(async () => {}),
		};
		const recall = {
			setCandidates: vi.fn(),
		};

		await refreshHomeWithRecallSync(home, recall, true, null);

		expect(recall.setCandidates).toHaveBeenCalledWith([]);
	});

	it("skips recall sync when the rail is disabled", async () => {
		const home = {
			recallCandidates: [
				{
					itemKey: "article:1",
					recallScore: 0.9,
					reasons: [],
					firstEligibleAt: "",
					nextSuggestAt: "",
				},
			],
			fetchData: vi.fn(async () => {}),
		};
		const recall = {
			setCandidates: vi.fn(),
		};

		await refreshHomeWithRecallSync(home, recall, false, null);

		expect(home.fetchData).toHaveBeenCalledWith(true, null);
		expect(recall.setCandidates).not.toHaveBeenCalled();
	});
});
