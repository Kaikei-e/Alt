import { createRawSnippet } from "svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";

const { mockNavigating } = vi.hoisted(() => ({
	mockNavigating: {
		type: "goto" as "goto" | null,
		shallow: true,
		from: null,
		to: null,
		delta: null,
		willUnload: false,
		complete: Promise.resolve(),
	},
}));

vi.mock("$app/state", () => ({
	get navigating() {
		return mockNavigating;
	},
	page: {
		url: new URL("http://localhost/home"),
		params: {},
	},
}));

vi.mock("$app/navigation", () => ({
	afterNavigate: vi.fn(),
}));

vi.mock("#lib/stores/loading.svelte.js", () => ({
	getLoadingStore: () => ({ isDesktopLoading: false }),
}));

import Layout from "./+layout.svelte";

const children = createRawSnippet(() => ({
	render: () => `<div data-testid="layout-child">Child Content</div>`,
}));

describe("(app) +layout.svelte navigation loader", () => {
	beforeEach(() => {
		mockNavigating.type = "goto";
		mockNavigating.shallow = true;
	});

	it("does not render SystemLoader when navigating is shallow", async () => {
		mockNavigating.shallow = true;
		const { container } = render(Layout, { props: { children } });

		expect(container.querySelector('[data-testid="system-loader"]')).toBeNull();
	});

	it("renders SystemLoader when navigating is not shallow", async () => {
		mockNavigating.shallow = false;
		const { container } = render(Layout, { props: { children } });

		expect(
			container.querySelector('[data-testid="system-loader"]'),
		).not.toBeNull();
		await expect.element(page.getByTestId("system-loader")).toBeInTheDocument();
	});
});
