import { expect, test } from "../../fixtures/pomFixtures";

/**
 * Where a user lands when something has gone wrong. Neither surface had any
 * coverage, and both are the kind of page that quietly rots: nobody visits them
 * on purpose, so a broken recovery link survives indefinitely.
 *
 *  - `[...path]` catches anything outside the route tree and throws 404. It has
 *    to stay a 404: an unknown path that renders 200 is invisible to monitoring
 *    and tells a crawler the page exists.
 *  - `/error` is where the Kratos flows dump the user on an auth failure. It is
 *    public by design — a user who cannot authenticate must still be able to
 *    read it — and its whole job is to offer a way back.
 */
test.describe("not found", () => {
	test("responds 404 for an unknown path", async ({ page }) => {
		const response = await page.goto("./no-such-page");

		expect(response?.status()).toBe(404);
	});

	test("shows an error page rather than a blank document", async ({ page }) => {
		await page.goto("./no-such-page");

		// SvelteKit's fallback error page renders the status; whatever the
		// wording, the user must see that this is a 404 and not an empty screen.
		await expect(page.getByText(/404|not found/i).first()).toBeVisible();
	});

	test("a deep unknown path is still a 404, not a redirect", async ({
		page,
	}) => {
		// Nested unknown paths must not be swallowed by the legacy-redirect table
		// or the auth guard.
		const response = await page.request.get("/desktop/does/not/exist", {
			maxRedirects: 0,
		});

		expect(response.status()).toBe(404);
	});
});

test.describe("auth error page", () => {
	test("renders without a session", async ({ page }) => {
		await page.goto("./error");

		await expect(
			page.getByRole("heading", { name: /error occurred/i }),
		).toBeVisible();
	});

	test("offers a route back to login and home", async ({ page }) => {
		// This page exists solely to un-stick the user; the links are the feature.
		await page.goto("./error");

		await expect(
			page.getByRole("link", { name: /go to login page/i }),
		).toBeVisible();
		await expect(page.getByRole("link", { name: /go to home/i })).toBeVisible();
	});

	test("surfaces the error id when Kratos supplies one", async ({ page }) => {
		// Support asks for this id; if the query parameter stops being read the
		// page still looks fine, which is exactly why it needs asserting.
		await page.goto("./error?id=e2e-error-id");

		await expect(page.getByText(/e2e-error-id/)).toBeVisible();
	});

	test("omits the error id block when there is none", async ({ page }) => {
		await page.goto("./error");

		await expect(page.getByText(/error id:/i)).toBeHidden();
	});
});
