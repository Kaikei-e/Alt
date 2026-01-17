import { describe, expect, it } from "vitest";
import { filterGenreProgress } from "./genreProgress";
import type { GenreProgressInfo } from "$lib/schema/dashboard";

describe("filterGenreProgress", () => {
	const createGenreInfo = (status: "pending" | "running" | "succeeded" | "failed"): GenreProgressInfo => ({
		status,
		cluster_count: null,
		article_count: null,
	});

	it("filters out classification when other genres are present", () => {
		const input: Record<string, GenreProgressInfo> = {
			classification: createGenreInfo("succeeded"),
			ai: createGenreInfo("succeeded"),
			tech: createGenreInfo("running"),
		};

		const result = filterGenreProgress(input);

		expect(result.length).toBe(2);
		expect(result.map(([genre]) => genre)).toEqual(["ai", "tech"]);
		expect(result.map(([genre]) => genre)).not.toContain("classification");
	});

	it("keeps classification when it is the only genre", () => {
		const input: Record<string, GenreProgressInfo> = {
			classification: createGenreInfo("succeeded"),
		};

		const result = filterGenreProgress(input);

		expect(result.length).toBe(1);
		expect(result[0][0]).toBe("classification");
	});

	it("returns sorted genres alphabetically", () => {
		const input: Record<string, GenreProgressInfo> = {
			tech: createGenreInfo("succeeded"),
			ai: createGenreInfo("running"),
			business: createGenreInfo("pending"),
		};

		const result = filterGenreProgress(input);

		expect(result.map(([genre]) => genre)).toEqual(["ai", "business", "tech"]);
	});

	it("handles empty input", () => {
		const result = filterGenreProgress({});
		expect(result).toEqual([]);
	});

	it("preserves genre info data", () => {
		const input: Record<string, GenreProgressInfo> = {
			ai: {
				status: "succeeded",
				cluster_count: 5,
				article_count: 20,
			},
		};

		const result = filterGenreProgress(input);

		expect(result.length).toBe(1);
		expect(result[0][1]).toEqual({
			status: "succeeded",
			cluster_count: 5,
			article_count: 20,
		});
	});

	it("handles genre names with special characters", () => {
		const input: Record<string, GenreProgressInfo> = {
			"AI/ML": createGenreInfo("succeeded"),
			"Web-Dev": createGenreInfo("running"),
		};

		const result = filterGenreProgress(input);

		expect(result.length).toBe(2);
		expect(result.map(([genre]) => genre)).toEqual(["AI/ML", "Web-Dev"]);
	});
});
