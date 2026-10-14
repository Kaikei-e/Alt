import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LensData } from "#lib/connect/knowledge_home.js";

const {
	mockCreateClientTransport,
	mockCreateLens,
	mockDeleteLens,
	mockListLenses,
	mockSelectLens,
} = vi.hoisted(() => ({
	mockCreateClientTransport: vi.fn(() => ({})),
	mockCreateLens: vi.fn(),
	mockDeleteLens: vi.fn(),
	mockListLenses: vi.fn(),
	mockSelectLens: vi.fn(),
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: mockCreateClientTransport,
	createLens: mockCreateLens,
	deleteLens: mockDeleteLens,
	listLenses: mockListLenses,
	selectLens: mockSelectLens,
}));

import { useLens } from "./useLens.svelte.js";

function makeLens(overrides: Partial<LensData> = {}): LensData {
	return {
		lensId: "lens-1",
		name: "AI News",
		description: "AI articles only",
		createdAt: "2026-03-17T10:00:00Z",
		updatedAt: "2026-03-17T10:00:00Z",
		currentVersion: {
			versionId: "v1",
			queryText: "agents",
			tagIds: ["AI"],
			sourceIds: [],
			timeWindow: "7d",
			includeRecap: false,
			includePulse: false,
			sortMode: "relevance",
		},
		...overrides,
	};
}

describe("useLens.remove", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("returns true and removes the lens when deleteLens succeeds", async () => {
		const lens1 = makeLens({ lensId: "lens-1", name: "Lens One" });
		const lens2 = makeLens({ lensId: "lens-2", name: "Lens Two" });
		mockListLenses.mockResolvedValue({
			lenses: [lens1, lens2],
			activeLensId: "lens-1",
		});
		mockDeleteLens.mockResolvedValue(true);

		const lens = useLens();
		await lens.fetchLenses();

		const result = await lens.remove("lens-1");

		expect(result).toBe(true);
		expect(lens.lenses.map((l) => l.lensId)).toEqual(["lens-2"]);
		expect(lens.activeLensId).toBeNull();
	});

	it("returns false and restores the optimistically removed lens and activeLensId when deleteLens fails", async () => {
		const lens1 = makeLens({ lensId: "lens-1", name: "Lens One" });
		const lens2 = makeLens({ lensId: "lens-2", name: "Lens Two" });
		mockListLenses.mockResolvedValue({
			lenses: [lens1, lens2],
			activeLensId: "lens-1",
		});
		mockDeleteLens.mockRejectedValue(new Error("delete failed"));

		const lens = useLens();
		await lens.fetchLenses();

		const result = await lens.remove("lens-1");

		expect(result).toBe(false);
		expect(lens.lenses.map((l) => l.lensId)).toEqual(["lens-1", "lens-2"]);
		expect(lens.activeLensId).toBe("lens-1");
		expect(lens.error?.message).toBe("delete failed");
	});
});
