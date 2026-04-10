import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import StatsBarWidget from "./StatsBarWidget.svelte";

describe("StatsBarWidget", () => {
	it("renders feed count", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("127")).toBeInTheDocument();
	});

	it("renders total articles count formatted", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("4,231")).toBeInTheDocument();
	});

	it("renders summarized count", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("3,890")).toBeInTheDocument();
	});

	it("shows Live when connected", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("Live")).toBeInTheDocument();
	});

	it("shows Offline when disconnected", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 0,
				totalArticlesAmount: 0,
				unsummarizedArticlesAmount: 0,
				isConnected: false,
			},
		});

		await expect.element(page.getByText("Offline")).toBeInTheDocument();
	});

	it("renders FEEDS label", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("FEEDS")).toBeInTheDocument();
	});

	it("renders ARTICLES label", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("ARTICLES")).toBeInTheDocument();
	});

	it("renders SUMMARIZED label", async () => {
		render(StatsBarWidget as never, {
			props: {
				feedAmount: 127,
				totalArticlesAmount: 4231,
				unsummarizedArticlesAmount: 341,
				isConnected: true,
			},
		});

		await expect.element(page.getByText("SUMMARIZED")).toBeInTheDocument();
	});
});
