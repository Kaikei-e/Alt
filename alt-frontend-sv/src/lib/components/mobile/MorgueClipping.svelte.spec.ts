import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";

import { renderFeedFixture } from "../../../../tests/fixtures/feeds";
import MorgueClipping from "./MorgueClipping.svelte";

const feed = { ...renderFeedFixture, isRead: true };

describe("MorgueClipping", () => {
	it("renders feed title, excerpt, and author", async () => {
		render(MorgueClipping as never, { props: { feed } });

		await expect
			.element(page.getByText(renderFeedFixture.title))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(renderFeedFixture.excerpt))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					`${renderFeedFixture.publishedAtFormatted} · ${renderFeedFixture.author}`,
				),
			)
			.toBeInTheDocument();
	});

	it("has data-role morgue-clipping attribute", async () => {
		render(MorgueClipping as never, { props: { feed } });

		await expect
			.element(page.getByRole("article"))
			.toHaveAttribute("data-role", "morgue-clipping");
	});

	it("has a Details button (text, no icon)", async () => {
		render(MorgueClipping as never, { props: { feed } });

		await expect
			.element(page.getByRole("button", { name: /details/i }))
			.toBeInTheDocument();
	});

	it("has an Open link pointing to normalizedUrl with target _blank", async () => {
		render(MorgueClipping as never, { props: { feed } });

		const openLink = page.getByRole("link", { name: /open/i });
		await expect.element(openLink).toBeInTheDocument();
		await expect
			.element(openLink)
			.toHaveAttribute("href", renderFeedFixture.normalizedUrl);
		await expect.element(openLink).toHaveAttribute("target", "_blank");
	});

	it("does not use glassmorphism styling", async () => {
		render(MorgueClipping as never, { props: { feed } });

		const article = page.getByRole("article");
		await expect.element(article).toBeInTheDocument();
		// No rounded corners or backdrop-filter in Alt-Paper
		const el = article.element() as HTMLElement;
		const style = window.getComputedStyle(el);
		expect(style.borderRadius).toBe("0px");
	});
});
