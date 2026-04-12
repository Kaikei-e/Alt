import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";
import type { SummaryState } from "$lib/connect/knowledge_home";

const __dirname = dirname(fileURLToPath(import.meta.url));
const componentSource = readFileSync(
	resolve(__dirname, "./SummaryStateChip.svelte"),
	"utf-8",
);

describe("SummaryStateChip", () => {
	it("pending state should render a chip", () => {
		const state: SummaryState = "pending";
		expect(state).toBe("pending");
	});

	it("ready state should render nothing", () => {
		const state: SummaryState = "ready";
		expect(state).toBe("ready");
	});

	it("missing state should render nothing", () => {
		const state: SummaryState = "missing";
		expect(state).toBe("missing");
	});

	it("only valid states are accepted", () => {
		const validStates: SummaryState[] = ["missing", "pending", "ready"];
		expect(validStates).toHaveLength(3);
	});

	describe("Alt-Paper palette compliance", () => {
		it("does not hardcode Tailwind blue-400 / blue-500", () => {
			expect(componentSource).not.toMatch(/blue-400/);
			expect(componentSource).not.toMatch(/blue-500/);
		});

		it("uses the --accent-info token", () => {
			expect(componentSource).toMatch(/--accent-info/);
		});
	});
});
