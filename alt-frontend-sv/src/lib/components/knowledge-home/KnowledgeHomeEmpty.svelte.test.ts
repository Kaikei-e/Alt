import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import KnowledgeHomeEmpty from "./KnowledgeHomeEmpty.svelte";

describe("KnowledgeHomeEmpty", () => {
	it("shows generic warming-up message when no reason specified", async () => {
		render(KnowledgeHomeEmpty as never, { props: {} });

		await expect
			.element(page.getByText("Your knowledge is warming up"))
			.toBeInTheDocument();
	});

	it("shows ingest_pending message when reason is ingest_pending", async () => {
		render(KnowledgeHomeEmpty as never, {
			props: { reason: "ingest_pending" },
		});

		await expect
			.element(page.getByText("Articles are being processed"))
			.toBeInTheDocument();
	});

	it("shows no_data message when reason is no_data", async () => {
		render(KnowledgeHomeEmpty as never, {
			props: { reason: "no_data" },
		});

		await expect.element(page.getByText("No articles yet")).toBeInTheDocument();
	});

	it("shows lens-specific message when reason is lens_strict", async () => {
		render(KnowledgeHomeEmpty as never, {
			props: { reason: "lens_strict", activeLensName: "AI News" },
		});

		await expect
			.element(page.getByText("No matches in AI News"))
			.toBeInTheDocument();
	});

	it("shows hard_error message when reason is hard_error", async () => {
		render(KnowledgeHomeEmpty as never, {
			props: { reason: "hard_error" },
		});

		await expect
			.element(page.getByText("Unable to load Knowledge Home"))
			.toBeInTheDocument();
	});

	it("shows clear lens button when lens_strict and onClearLens provided", async () => {
		const clearFn = vi.fn();
		render(KnowledgeHomeEmpty as never, {
			props: {
				reason: "lens_strict",
				activeLensName: "AI News",
				onClearLens: clearFn,
			},
		});

		await expect.element(page.getByText("Clear lens")).toBeInTheDocument();
	});

	it("does not show clear lens button when reason is not lens_strict", async () => {
		render(KnowledgeHomeEmpty as never, {
			props: { reason: "no_data", onClearLens: vi.fn() },
		});

		await expect.element(page.getByText("Clear lens")).not.toBeInTheDocument();
	});
});
