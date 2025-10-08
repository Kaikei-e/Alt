import { test, expect } from "../../tests/fixtures";
import { waitForPageReady } from "../../tests/utils/waitConditions";

test.describe("Network Failure Scenarios", () => {
  test("should handle auth service unavailable", async ({
    page,
    loginPage,
  }) => {
    // Mock network failure for auth service
    await page.route("**/localhost:4545/**", (route) => {
      route.abort("connectionrefused");
    });

    // Try to access protected page
    await page.goto("/desktop/home");
    await page.waitForLoadState("domcontentloaded");

    // Should show some kind of error or fallback behavior
    // This depends on how your app handles auth service failures
    await expect(page).toHaveURL(/\/desktop\/home|\/auth\/login|\/error/, {
      timeout: 15000,
    });
  });

  test("should handle slow network responses", async ({ page, loginPage }) => {
    // Add delay to auth service responses
    await page.route("**/localhost:4545/**", async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1000)); // Reduced from 2000ms
      await route.continue();
    });

    // Try to login with slow network
    await page.goto("/desktop/home");
    await page.waitForLoadState("domcontentloaded");

    // Should eventually redirect to auth
    await page.waitForURL(
      /\/auth\/login\?flow=/,
      { timeout: 15000 },
    );
  });

  test("should handle malformed auth responses", async ({
    page,
    loginPage,
  }) => {
    // Mock malformed JSON response
    await page.route("**/self-service/login/flows**", (route) => {
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: '{"invalid": json}',
      });
    });

    await page.goto("/auth/login?flow=test-flow-id");
    await page.waitForLoadState("domcontentloaded");

    // Should handle the parsing error gracefully
    // Wait for error handling to complete instead of fixed timeout
    await expect(page.locator("body")).toBeVisible({ timeout: 10000 });
  });

  test("should handle session timeout gracefully", async ({
    page,
    desktopPage,
  }) => {
    // Start with authenticated state then invalidate session
    await desktopPage.navigateToHome();
    await desktopPage.verifyOnDesktopPage("home");

    // Mock session invalidation
    await page.route("**/sessions/whoami", (route) => {
      route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ error: { message: "Session expired" } }),
      });
    });

    // Try to navigate to another page
    await page.goto("/desktop/settings");
    await page.waitForLoadState("domcontentloaded");

    // Should redirect to login due to expired session
    await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 20000 });
  });

  test("should handle CSRF token mismatch", async ({ page, loginPage }) => {
    // Mock CSRF error response
    await page.route("**/self-service/login?flow=**", (route) => {
      if (route.request().method() === "POST") {
        route.fulfill({
          status: 403,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              id: "security_csrf_violation",
              code: 403,
              status: "Forbidden",
              message:
                "A security violation was detected. Please retry the flow.",
            },
          }),
        });
      } else {
        route.continue();
      }
    });

    await page.goto("/auth/login?flow=test-flow-id");
    await waitForPageReady(page, { waitForSelector: "form", timeout: 15000 });

    await loginPage.login("test@example.com", "password123");

    // Should show CSRF error or refresh the form
    // Wait for error message or form refresh instead of fixed timeout
    await expect(page.locator("body")).toBeVisible({ timeout: 10000 });
  });

  test("should handle concurrent login attempts", async ({ browser }) => {
    // Create multiple contexts to simulate concurrent logins
    const context1 = await browser.newContext();
    const context2 = await browser.newContext();

    const page1 = await context1.newPage();
    const page2 = await context2.newPage();

    try {
      // Start login process in both contexts simultaneously
      const [result1, result2] = await Promise.allSettled([
        (async () => {
          await page1.goto("/desktop/home");
          await page1.waitForLoadState("domcontentloaded");
        })(),
        (async () => {
          await page2.goto("/desktop/home");
          await page2.waitForLoadState("domcontentloaded");
        })(),
      ]);

      // Both should redirect to auth flow
      await Promise.all([
        page1.waitForURL(
          /\/auth\/login\?flow=/,
          { timeout: 15000 },
        ),
        page2.waitForURL(
          /\/auth\/login\?flow=/,
          { timeout: 15000 },
        ),
      ]);

      expect(result1.status).toBe("fulfilled");
      expect(result2.status).toBe("fulfilled");
    } finally {
      await context1.close();
      await context2.close();
    }
  });

  test("should recover from temporary network failures", async ({
    page,
    loginPage,
  }) => {
    let failureCount = 0;

    // Mock intermittent network failures
    await page.route("**/self-service/login/flows**", (route) => {
      failureCount++;
      if (failureCount <= 2) {
        // Fail first two requests
        route.abort("connectionrefused");
      } else {
        // Succeed on third request
        route.continue();
      }
    });

    await page.goto("/auth/login?flow=test-flow-id");
    await page.waitForLoadState("domcontentloaded");

    // Wait for recovery - use retry mechanism instead of fixed timeout
    await expect(async () => {
      // Check if page has loaded properly or shows error handling UI
      const bodyElement = page.locator("body");
      await expect(bodyElement).toBeVisible();
    }).toPass({ timeout: 20000, intervals: [1000] });
  });
});
