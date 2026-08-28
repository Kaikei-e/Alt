/**
 * ThreadEntry component tests.
 *
 * A reader-facing turn must never show the article id. The id lives in the
 * message body because rag-orchestrator scopes retrieval on it
 * (query_intent.go), so the wire text keeps it and this component is the one
 * place that decides what a person sees.
 */
import { page } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

import ThreadEntry from "./ThreadEntry.svelte";

const { loadProxyImageDefault, resolveThumbnail } = vi.hoisted(() => ({
	loadProxyImageDefault: vi.fn(),
	resolveThumbnail: vi.fn(),
}));

vi.mock("$lib/utils/loadProxyImage", () => ({ loadProxyImageDefault }));
vi.mock("$lib/utils/articleThumbnail", () => ({
	articleThumbnailResolver: () => ({ resolve: resolveThumbnail }),
}));

const ARTICLE_ID = "c6463b44-589f-4959-879f-40eda019f95a";
const ARTICLE_TITLE =
	"Love me, love my bruised ego: what the narcissist-artist film tells us about the fear of ageing | Film | The Guardian";
const SCOPED_MESSAGE = `Regarding the article: ${ARTICLE_TITLE} [articleId: ${ARTICLE_ID}]\n\nQuestion:\n3行でまとめると？`;
const PROXY_URL = "/api/og-image?u=https%3A%2F%2Falt.ai%2Fog.png";

describe("ThreadEntry", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		resolveThumbnail.mockResolvedValue(null);
		loadProxyImageDefault.mockResolvedValue({ status: "absent" });
	});

	it("shows the question alone and never the article id", async () => {
		render(ThreadEntry, {
			props: { message: SCOPED_MESSAGE, role: "user" },
		});

		await expect
			.element(page.getByText("3行でまとめると？"))
			.toBeInTheDocument();
		expect(document.body.textContent).not.toContain(ARTICLE_ID);
		expect(document.body.textContent).not.toContain("articleId");
		expect(document.body.textContent).not.toContain("Regarding the article:");
	});

	it("keeps the article visible as its own scope card", async () => {
		render(ThreadEntry, {
			props: { message: SCOPED_MESSAGE, role: "user" },
		});

		await expect
			.element(page.getByTestId("article-scope-card"))
			.toBeInTheDocument();
		await expect.element(page.getByText(ARTICLE_TITLE)).toBeInTheDocument();
	});

	it("renders the thumbnail once it resolves", async () => {
		resolveThumbnail.mockResolvedValue(PROXY_URL);
		loadProxyImageDefault.mockResolvedValue({ status: "loaded" });

		render(ThreadEntry, {
			props: { message: SCOPED_MESSAGE, role: "user" },
		});

		await expect
			.element(page.getByTestId("article-scope-thumbnail"))
			.toBeInTheDocument();
		// The thread has no source URL to hand over — the resolver looks it up.
		expect(resolveThumbnail).toHaveBeenCalledWith(ARTICLE_ID, null);
	});

	it("keeps the card readable when no image can be had", async () => {
		render(ThreadEntry, {
			props: { message: SCOPED_MESSAGE, role: "user" },
		});

		await expect.element(page.getByText(ARTICLE_TITLE)).toBeInTheDocument();
		await expect
			.element(page.getByTestId("article-scope-thumbnail"))
			.not.toBeInTheDocument();
	});

	it("leaves an unscoped question exactly as written", async () => {
		render(ThreadEntry, {
			props: { message: "What changed today?", role: "user" },
		});

		await expect
			.element(page.getByText("What changed today?"))
			.toBeInTheDocument();
		expect(document.querySelector('[data-testid="article-scope-card"]')).toBe(
			null,
		);
		expect(resolveThumbnail).not.toHaveBeenCalled();
	});

	// The footer is the citation surface below 1280px — the width every phone
	// reads at — so it is the one that meets an unsanitized url first.
	it("never binds a javascript: citation url to an href", async () => {
		render(ThreadEntry, {
			props: {
				message: "answer",
				role: "assistant",
				citations: [{ URL: "javascript:alert(1)", Title: "Malicious source" }],
			},
		});

		await expect
			.element(page.getByText("Malicious source"))
			.toBeInTheDocument();
		expect(document.querySelector('a[href^="javascript:"]')).toBe(null);
	});

	it("sends an article citation to the article, not to a dead relative url", async () => {
		render(ThreadEntry, {
			props: {
				message: "answer",
				role: "assistant",
				citations: [
					{
						URL: "",
						Title: "A stored article",
						Kind: "ARTICLE" as const,
						RefID: ARTICLE_ID,
					},
				],
			},
		});

		const link = page.getByRole("link", { name: "A stored article" });
		await expect
			.element(link)
			.toHaveAttribute("href", `/articles/${ARTICLE_ID}`);
	});

	it("still links a legacy citation that carries only a url", async () => {
		render(ThreadEntry, {
			props: {
				message: "answer",
				role: "assistant",
				citations: [{ URL: "https://example.com/a", Title: "Legacy source" }],
			},
		});

		const link = page.getByRole("link", { name: "Legacy source" });
		await expect.element(link).toHaveAttribute("href", "https://example.com/a");
		await expect.element(link).toHaveAttribute("target", "_blank");
	});

	it("renders an assistant turn as prose with its byline", async () => {
		render(ThreadEntry, {
			props: {
				message: "**Bold** answer",
				role: "assistant",
				timestamp: "21:57:30",
			},
		});

		await expect.element(page.getByText("Augur")).toBeInTheDocument();
		await expect.element(page.getByText("21:57:30")).toBeInTheDocument();
		await expect.element(page.getByText("Bold")).toBeInTheDocument();
	});
});
