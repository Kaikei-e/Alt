import { expect, test } from "@playwright/test";

/**
 * Phase A RED — Knowledge Loop primary surface.
 *
 * Establishes the new frontend path for the cognitive-state engine:
 *
 *   /  (authenticated)
 *     -> /loop/welcome    (first-time, no `alt_loop_welcomed` cookie)
 *     -> /loop            (returning, cookie present)
 *
 *   /home  shows a one-line cross-link banner to /loop
 *   /feeds shows a one-line cross-link banner to /loop
 *
 * The banner is the explicit handover from the Phase-0 read model (/home as
 * Knowledge Home, /feeds as raw inbox) to the v2 cognitive state engine
 * (/loop). Users must always be one click away from /loop regardless of which
 * legacy surface they land on.
 */

const WELCOMED_COOKIE = "alt_loop_welcomed";

test.describe("Knowledge Loop — primary surface routing", () => {
	test("authenticated `/` redirects to `/loop` when user is already welcomed", async ({
		page,
		context,
	}) => {
		await context.addCookies([
			{
				name: WELCOMED_COOKIE,
				value: "true",
				domain: "127.0.0.1",
				path: "/",
			},
		]);

		await page.goto("/");

		await expect(page).toHaveURL(/\/loop(\?.*)?$/);
	});

	test("authenticated `/` redirects to `/loop/welcome` for first-time users", async ({
		page,
		context,
	}) => {
		await context.clearCookies({ name: WELCOMED_COOKIE });

		await page.goto("/");

		await expect(page).toHaveURL(/\/loop\/welcome$/);
	});

	test("`/loop/welcome` shows editorial copy and an explicit Start CTA", async ({
		page,
		context,
	}) => {
		await context.clearCookies({ name: WELCOMED_COOKIE });

		await page.goto("/loop/welcome");

		await expect(
			page.getByRole("heading", { name: /Knowledge Loop/i }),
		).toBeVisible();

		const startCta = page.getByRole("button", { name: /Start/i });
		await expect(startCta).toBeVisible();
	});

	test("clicking Start on `/loop/welcome` sets the welcomed cookie and lands on `/loop`", async ({
		page,
		context,
	}) => {
		await context.clearCookies({ name: WELCOMED_COOKIE });

		await page.goto("/loop/welcome");

		await page.getByRole("button", { name: /Start/i }).click();

		await expect(page).toHaveURL(/\/loop(\?.*)?$/);

		const cookies = await context.cookies();
		const welcomed = cookies.find((c) => c.name === WELCOMED_COOKIE);
		expect(welcomed?.value).toBe("true");
	});

	test("`/home` shows a cross-link banner pointing to `/loop`", async ({
		page,
	}) => {
		await page.goto("/home");

		const banner = page.getByTestId("loop-cross-link");
		await expect(banner).toBeVisible();
		await expect(banner).toHaveAttribute("href", /\/loop(\?.*)?$/);
	});

	test("`/feeds` shows a cross-link banner pointing to `/loop`", async ({
		page,
	}) => {
		await page.goto("/feeds");

		const banner = page.getByTestId("loop-cross-link");
		await expect(banner).toBeVisible();
		await expect(banner).toHaveAttribute("href", /\/loop(\?.*)?$/);
	});
});
