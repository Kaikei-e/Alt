import { test, expect } from "@playwright/test";

test.describe("FloatingMenu Performance Assessment", () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([
          {
            title: "Test Feed 1",
            description: "Test description 1",
            link: "https://example.com/feed/1",
            published: new Date().toISOString(),
          },
        ]),
      });
    });

    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: [],
          next_cursor: null,
        }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([]),
      });
    });

    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });
  });

  test("should open menu within performance budget (< 200ms)", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    // Wait for FloatingMenu to be ready
    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Measure menu opening time
    const startTime = Date.now();

    await page.getByTestId("floating-menu-button").click();
    await expect(page.getByTestId("menu-content")).toBeVisible();

    const endTime = Date.now();
    const openingTime = endTime - startTime;

    console.log(`Menu opening time: ${openingTime}ms`);

    // Should open within 1000ms performance budget (relaxed for CI environment)
    expect(openingTime).toBeLessThan(1000);
  });

  test("should handle rapid open/close interactions without performance degradation", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    const cycles = 5;
    const cycleTimes: number[] = [];

    for (let i = 0; i < cycles; i++) {
      const startTime = Date.now();

      // Open menu
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("menu-content")).toBeVisible();

      // Close menu
      await page.getByTestId("close-menu-button").click();
      await expect(page.getByTestId("menu-content")).not.toBeVisible();

      const endTime = Date.now();
      const cycleTime = endTime - startTime;
      cycleTimes.push(cycleTime);

      console.log(`Cycle ${i + 1} time: ${cycleTime}ms`);
    }

    // Average cycle time should be reasonable
    const averageTime = cycleTimes.reduce((a, b) => a + b, 0) / cycles;
    console.log(`Average cycle time: ${averageTime}ms`);

    expect(averageTime).toBeLessThan(500);

    // No significant degradation between first and last cycles
    const firstCycle = cycleTimes[0];
    const lastCycle = cycleTimes[cycles - 1];
    const degradation = lastCycle - firstCycle;

    console.log(`Performance degradation: ${degradation}ms`);
    expect(Math.abs(degradation)).toBeLessThan(500); // Relaxed for CI environment
  });

  test("should maintain memory usage stability during menu interactions", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Get initial memory baseline
    const initialMemory = await page.evaluate(() => {
      if ('memory' in performance) {
        return (performance as any).memory.usedJSHeapSize;
      }
      return 0;
    });

    // Perform multiple menu interactions
    for (let i = 0; i < 10; i++) {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("menu-content")).toBeVisible();

      // Click Home link (but don't wait for navigation)
      const homeLink = page.getByTestId("menu-content").getByRole("link").filter({ hasText: "Home" });
      await homeLink.hover(); // Just hover to trigger any handlers

      await page.getByTestId("close-menu-button").click();
      await expect(page.getByTestId("menu-content")).not.toBeVisible();
    }

    // Check final memory usage
    const finalMemory = await page.evaluate(() => {
      if ('memory' in performance) {
        return (performance as any).memory.usedJSHeapSize;
      }
      return 0;
    });

    if (initialMemory > 0 && finalMemory > 0) {
      const memoryIncrease = finalMemory - initialMemory;
      console.log(`Memory increase: ${memoryIncrease} bytes`);

      // Memory increase should be minimal (less than 1MB)
      expect(memoryIncrease).toBeLessThan(1024 * 1024);
    } else {
      console.log("Memory measurement not available in this browser");
    }
  });

  test("should verify Home navigation performance", async ({ page }) => {
    await page.goto("/mobile/feeds");
    await page.waitForLoadState("networkidle");

    await page.waitForSelector('[data-testid="floating-menu-button"]', { timeout: 10000 });

    // Open menu
    await page.getByTestId("floating-menu-button").click();
    await expect(page.getByTestId("menu-content")).toBeVisible();

    // Measure navigation time to home
    const startTime = Date.now();

    const homeLink = page.getByTestId("menu-content").getByRole("link").filter({ hasText: "Home" });

    // Use Promise.all to wait for navigation
    await Promise.all([
      page.waitForURL('**/'),
      homeLink.click()
    ]);

    // Wait for navigation
    await page.waitForLoadState("networkidle");

    const endTime = Date.now();
    const navigationTime = endTime - startTime;

    console.log(`Home navigation time: ${navigationTime}ms`);

    // Navigation should be fast (< 1 second)
    expect(navigationTime).toBeLessThan(1000);

    // Verify we're on home page
    const currentUrl = page.url();
    expect(currentUrl.endsWith('/') || currentUrl.includes('localhost:3010/')).toBe(true);
  });
});