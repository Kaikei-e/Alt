import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({ transport: true })),
	getKnowledgeHome: vi.fn(),
	trackHomeItemsSeen: vi.fn(),
	trackHomeAction: vi.fn(),
}));

import { trackHomeAction } from "$lib/connect";
import { useKnowledgeHome } from "./useKnowledgeHome.svelte";

describe("useKnowledgeHome", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("dismissItem only removes the item locally and does not track by itself", () => {
		const home = useKnowledgeHome();
		home.fetchData = vi.fn() as typeof home.fetchData;

		// Seed state via direct property access is not available, so reuse the public API surface.
		// The hook exposes a live array reference, which we can populate for local-state tests.
		(home.items as Array<{ itemKey: string; title: string }>).push(
			{ itemKey: "article:1", title: "One" },
			{ itemKey: "article:2", title: "Two" },
		);

		home.dismissItem("article:1");

		expect(home.items.map((item) => item.itemKey)).toEqual(["article:2"]);
		expect(trackHomeAction).not.toHaveBeenCalled();
	});
});
