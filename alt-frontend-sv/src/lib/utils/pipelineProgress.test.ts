import { describe, expect, it } from "vitest";
import {
	shouldShowSubStageProgress,
	formatSubStageProgress,
	inferStageCompletion,
} from "./pipelineProgress";
import type { SubStageProgress } from "$lib/schema/dashboard";

describe("shouldShowSubStageProgress", () => {
	const createProgress = (
		phase: "evidence_building" | "clustering" | "summarization",
		completed: number = 5,
		total: number = 20,
	): SubStageProgress => ({
		phase,
		completed_genres: completed,
		total_genres: total,
	});

	describe("evidence stage", () => {
		it("shows progress when evidence_building phase is active", () => {
			const result = shouldShowSubStageProgress(
				"evidence",
				"running",
				createProgress("evidence_building"),
			);
			expect(result).toBe(true);
		});

		it("does not show progress when clustering phase is active", () => {
			const result = shouldShowSubStageProgress(
				"evidence",
				"running",
				createProgress("clustering"),
			);
			expect(result).toBe(false);
		});

		it("does not show progress when stage is not running", () => {
			const result = shouldShowSubStageProgress(
				"evidence",
				"completed",
				createProgress("evidence_building"),
			);
			expect(result).toBe(false);
		});
	});

	describe("dispatch stage", () => {
		it("shows progress when clustering phase is active", () => {
			const result = shouldShowSubStageProgress(
				"dispatch",
				"running",
				createProgress("clustering"),
			);
			expect(result).toBe(true);
		});

		it("shows progress when summarization phase is active", () => {
			const result = shouldShowSubStageProgress(
				"dispatch",
				"running",
				createProgress("summarization"),
			);
			expect(result).toBe(true);
		});

		it("does not show progress when evidence_building phase is active", () => {
			const result = shouldShowSubStageProgress(
				"dispatch",
				"running",
				createProgress("evidence_building"),
			);
			expect(result).toBe(false);
		});

		it("does not show progress when stage is not running", () => {
			const result = shouldShowSubStageProgress(
				"dispatch",
				"pending",
				createProgress("clustering"),
			);
			expect(result).toBe(false);
		});
	});

	describe("other stages", () => {
		it("does not show progress for fetch stage", () => {
			const result = shouldShowSubStageProgress(
				"fetch",
				"running",
				createProgress("clustering"),
			);
			expect(result).toBe(false);
		});

		it("does not show progress for persist stage", () => {
			const result = shouldShowSubStageProgress(
				"persist",
				"running",
				createProgress("summarization"),
			);
			expect(result).toBe(false);
		});
	});

	describe("edge cases", () => {
		it("returns false when subStageProgress is null", () => {
			const result = shouldShowSubStageProgress("dispatch", "running", null);
			expect(result).toBe(false);
		});

		it("returns false when subStageProgress is undefined", () => {
			const result = shouldShowSubStageProgress(
				"evidence",
				"running",
				undefined,
			);
			expect(result).toBe(false);
		});
	});
});

describe("formatSubStageProgress", () => {
	it("formats progress as n/m string", () => {
		const progress: SubStageProgress = {
			phase: "clustering",
			completed_genres: 5,
			total_genres: 20,
		};
		expect(formatSubStageProgress(progress)).toBe("5/20");
	});

	it("handles zero completed", () => {
		const progress: SubStageProgress = {
			phase: "evidence_building",
			completed_genres: 0,
			total_genres: 15,
		};
		expect(formatSubStageProgress(progress)).toBe("0/15");
	});

	it("handles all completed", () => {
		const progress: SubStageProgress = {
			phase: "summarization",
			completed_genres: 10,
			total_genres: 10,
		};
		expect(formatSubStageProgress(progress)).toBe("10/10");
	});
});

describe("inferStageCompletion", () => {
	describe("explicit completion", () => {
		it("returns completed when stage is in stagesCompleted", () => {
			const result = inferStageCompletion(
				"fetch",
				0,
				["fetch", "preprocess"],
				"dedup",
				2,
			);
			expect(result).toBe("completed");
		});
	});

	describe("running detection", () => {
		it("returns running when currentStage matches", () => {
			const result = inferStageCompletion(
				"genre",
				3,
				["fetch", "preprocess", "dedup"],
				"genre",
				3,
			);
			expect(result).toBe("running");
		});

		it("returns running when index matches stageIndex", () => {
			const result = inferStageCompletion(
				"select",
				4,
				["fetch", "preprocess", "dedup", "genre"],
				null,
				4,
			);
			expect(result).toBe("running");
		});
	});

	describe("evidence stage inference (critical fix)", () => {
		it("returns completed for evidence when dispatch is running", () => {
			// This is the key bug fix: evidence stage index is 5, dispatch is 6
			// When currentStageIndex >= 6, evidence must be completed
			const result = inferStageCompletion(
				"evidence",
				5,
				["fetch", "preprocess", "dedup", "genre", "select"], // evidence NOT in completed list
				"dispatch",
				6,
			);
			expect(result).toBe("completed");
		});

		it("returns completed for evidence when persist is running", () => {
			const result = inferStageCompletion(
				"evidence",
				5,
				["fetch", "preprocess", "dedup", "genre", "select", "dispatch"],
				"persist",
				7,
			);
			expect(result).toBe("completed");
		});

		it("returns pending for evidence when select is running", () => {
			const result = inferStageCompletion(
				"evidence",
				5,
				["fetch", "preprocess", "dedup", "genre"],
				"select",
				4,
			);
			expect(result).toBe("pending");
		});

		it("returns running for evidence when it is the current stage", () => {
			const result = inferStageCompletion(
				"evidence",
				5,
				["fetch", "preprocess", "dedup", "genre", "select"],
				"evidence",
				5,
			);
			expect(result).toBe("running");
		});
	});

	describe("general stage inference", () => {
		it("returns completed for earlier stages when later stage is running", () => {
			// When dispatch (index 6) is running, all earlier stages should be completed
			const result = inferStageCompletion(
				"genre",
				3,
				["fetch", "preprocess", "dedup"], // genre NOT in completed list
				"dispatch",
				6,
			);
			expect(result).toBe("completed");
		});

		it("returns completed when later stage is in stagesCompleted", () => {
			const result = inferStageCompletion(
				"select",
				4,
				["fetch", "preprocess", "dedup", "genre", "dispatch"], // dispatch completed but select missing
				"persist",
				7,
			);
			expect(result).toBe("completed");
		});

		it("returns pending for stages after current stage", () => {
			const result = inferStageCompletion(
				"persist",
				7,
				["fetch", "preprocess", "dedup", "genre", "select"],
				"dispatch",
				6,
			);
			expect(result).toBe("pending");
		});
	});

	describe("edge cases", () => {
		it("handles empty stagesCompleted", () => {
			const result = inferStageCompletion("fetch", 0, [], "fetch", 0);
			expect(result).toBe("running");
		});

		it("handles null currentStage with valid stageIndex", () => {
			const result = inferStageCompletion(
				"evidence",
				5,
				["fetch", "preprocess", "dedup", "genre", "select"],
				null,
				6,
			);
			expect(result).toBe("completed");
		});

		it("handles first stage pending", () => {
			const result = inferStageCompletion("fetch", 0, [], null, -1);
			expect(result).toBe("pending");
		});
	});
});
