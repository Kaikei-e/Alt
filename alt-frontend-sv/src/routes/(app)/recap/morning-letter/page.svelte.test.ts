import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { MorningLetterDocument } from "#lib/gen/alt/morning_letter/v2/morning_letter_pb.js";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

const { pageState } = vi.hoisted(() => {
	const value: {
		url: URL;
		data: {
			letter: MorningLetterDocument | null;
			requestedDate: string | null;
			error: string | null;
		};
	} = {
		url: new URL("http://localhost/recap/morning-letter"),
		data: {
			letter: null,
			requestedDate: null,
			error: null,
		},
	};
	return {
		pageState: {
			get url() {
				return value.url;
			},
			set url(next: URL) {
				value.url = next;
			},
			get data() {
				return value.data;
			},
			set data(next: typeof value.data) {
				value.data = next;
			},
		},
	};
});

vi.mock("$app/state", () => ({ page: pageState }));

let currentLetterState = $state({
	letter: null as MorningLetterDocument | null,
	sources: [],
	enrichments: [],
	letterLoading: false,
	sourcesLoading: false,
	enrichmentsLoading: false,
	error: null as Error | null,
	regenerating: false,
	regenerateCooldownMsg: null as string | null,
	fetchLetter: vi.fn(),
	fetchByDate: vi.fn(),
	regenerate: vi.fn(),
});

vi.mock("#lib/hooks/useMorningLetter.svelte.js", () => ({
	useMorningLetter: () => currentLetterState,
}));

import Page from "./+page.svelte";

function makeLetter(
	overrides: Record<string, unknown> = {},
): MorningLetterDocument {
	return {
		$typeName: "alt.morning_letter.v2.MorningLetterDocument",
		id: "letter-001",
		targetDate: "2026-04-07",
		editionTimezone: "Asia/Tokyo",
		isDegraded: false,
		schemaVersion: 1,
		generationRevision: 1,
		model: "gemma4-e4b",
		createdAt: {
			$typeName: "google.protobuf.Timestamp",
			seconds: BigInt(1700000000),
			nanos: 0,
		},
		etag: '"rev-1"',
		body: {
			$typeName: "alt.morning_letter.v2.MorningLetterBody",
			lead: "Key developments across AI and web engineering.",
			generatedAt: {
				$typeName: "google.protobuf.Timestamp",
				seconds: BigInt(1700000000),
				nanos: 0,
			},
			sourceRecapWindowDays: 3,
			throughLine: "Inference efficiency and browser standards converge.",
			sections: [],
		},
		surfaceStatus: {
			$typeName: "alt.recap.v2.RecapSurfaceStatus",
			generatedAt: "2026-04-07T06:00:00Z",
			windowStart: "2026-04-04T06:00:00Z",
			windowEnd: "2026-04-07T06:00:00Z",
			isStale: false,
			staleAfter: "2026-04-07T09:00:00Z",
			degradationReasons: [],
		},
		...overrides,
	} as unknown as MorningLetterDocument;
}

describe("Morning Letter page freshness and wording", () => {
	beforeEach(async () => {
		await page.viewport(1024, 768);
		pageState.data = { letter: null, requestedDate: null, error: null };
		currentLetterState.letter = makeLetter();
	});

	it("shows Today's Edition alongside the target date in the dateline when surfaceStatus is current", async () => {
		currentLetterState.letter = makeLetter({
			surfaceStatus: {
				$typeName: "alt.recap.v2.RecapSurfaceStatus",
				generatedAt: "2026-04-07T06:00:00Z",
				windowStart: "2026-04-04T06:00:00Z",
				windowEnd: "2026-04-07T06:00:00Z",
				isStale: false,
				staleAfter: "2026-04-07T09:00:00Z",
				degradationReasons: [],
			},
		});

		const { container } = render(Page);

		const dateline = container.querySelector(".edition-dateline");
		expect(dateline?.textContent?.trim()).toBe("Today's Edition · 2026-4-7");
		expect(container.querySelector(".edition-badge--stale")).toBeNull();
		expect(container.textContent).toContain(
			"Ask follow-up questions about this edition.",
		);
		expect(container.textContent).not.toContain(
			"Ask follow-up questions about today's edition.",
		);
	});

	it("replaces Today's Edition with dated heading and renders SurfaceStatusBanner when stale", async () => {
		currentLetterState.letter = makeLetter({
			surfaceStatus: {
				$typeName: "alt.recap.v2.RecapSurfaceStatus",
				generatedAt: "2026-04-07T06:00:00Z",
				windowStart: "2026-04-04T06:00:00Z",
				windowEnd: "2026-04-07T06:00:00Z",
				isStale: true,
				staleAfter: "2026-04-07T09:00:00Z",
				degradationReasons: [
					{
						$typeName: "alt.recap.v2.RecapDegradationReason",
						code: "placeholder_output",
						detail: "",
					},
				],
			},
		});

		const { container } = render(Page);

		const dateline = container.querySelector(".edition-dateline");
		expect(dateline?.textContent?.trim()).toBe("2026-4-7");
		expect(dateline?.textContent).not.toContain("Today's Edition");
		expect(container.querySelector(".edition-badge--stale")).toBeNull();

		const alertEl = page.getByRole("alert");
		await expect.element(alertEl).toHaveTextContent("This was last updated");
		await expect
			.element(alertEl)
			.toHaveTextContent("a newer update has not completed.");
		await expect
			.element(alertEl)
			.toHaveTextContent(
				"The AI draft had empty or placeholder sections, so this letter shows extracted excerpts instead.",
			);
	});
});
