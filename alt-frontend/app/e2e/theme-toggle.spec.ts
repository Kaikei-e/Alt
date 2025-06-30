import { test, expect } from '@playwright/test';

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
// Theme Toggle E2E Tests - Alt Design System
// Following CLAUDE.md guidelines: Maximum 3 comprehensive tests

test.describe('Theme Toggle - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock all necessary API responses for consistent testing
    await page.route('**/api/v1/feeds/stats', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 28 },
        }),
      });
    });

    await page.route('**/v1/feeds/stats', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feed_amount: { amount: 42 },
          summarized_feed: { amount: 28 },
        }),
      });
    });

    // Mock feeds endpoint for mobile/feeds page
    await page.route('**/api/v1/feeds', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          feeds: [
            {
              title: "Test Feed 1",
              description: "Test description 1",
              link: "https://example.com/feed1",
              published: "2024-01-01",
              authors: [{ name: "Test Author" }]
            }
          ],
          cursor: "test-cursor",
          hasMore: false
        }),
      });
    });

    // Mock health endpoint
    await page.route('**/api/v1/health', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ status: "ok" }),
      });
    });

    // Mock SSE endpoint for stats page
    await page.route('**/api/v1/feeds/stats/sse', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: 'data: {"feed_amount":{"amount":42},"summarized_feed":{"amount":28}}\n\n',
      });
    });

    await page.goto('/');
    await page.waitForLoadState('networkidle');
  });

  test('should toggle between vaporwave and liquid-beige themes (PROTECTED)', async ({ page }) => {
    // Wait for theme to be fully loaded
    await page.waitForTimeout(1000);

    const body = page.locator('body');

    // Check for theme-specific styling
    const initialBackground = await body.evaluate((el) => getComputedStyle(el).backgroundColor);
    expect(initialBackground).toBeTruthy();

    // Look for theme toggle button/mechanism with more specific selectors
    const themeToggle = page.locator('[data-testid="theme-toggle-button"]').or(
      page.locator('[data-testid="theme-toggle"]').or(
        page.locator('button[role="switch"]').or(
          page.locator('button:has-text("Theme")').or(
            page.locator('button:has-text("Switch Theme")')
          )
        )
      )
    );

    // If theme toggle exists, test the toggle functionality
    if (await themeToggle.count() > 0) {
      // Wait for theme toggle to be ready
      await expect(themeToggle.first()).toBeVisible({ timeout: 5000 });

      // Get initial theme data attribute
      const initialDataStyle = await body.getAttribute('data-style');

      // Click the theme toggle
      await themeToggle.first().click();

      // Wait for theme change to complete
      await page.waitForTimeout(500);

      // Verify theme change by checking data-style attribute
      const newDataStyle = await body.getAttribute('data-style');
      expect(newDataStyle).not.toBe(initialDataStyle);

      // Also verify background changed
      const newBackground = await body.evaluate((el) => getComputedStyle(el).backgroundColor);
      expect(newBackground).not.toBe(initialBackground);

      // Verify the theme alternates between expected values
      expect(['vaporwave', 'liquid-beige']).toContain(newDataStyle);
    } else {
      // If no theme toggle exists, verify theme persistence across reload
      // Get initial data-style attribute for more reliable comparison
      const initialDataStyle = await body.getAttribute('data-style');

      await page.reload();
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      const reloadedDataStyle = await body.getAttribute('data-style');
      expect(reloadedDataStyle).toBe(initialDataStyle);
    }
  });

  test('should maintain glass effect across themes (PROTECTED)', async ({ page }) => {
    // Navigate to a page with glass components
    await page.goto('/mobile/feeds');
    await page.waitForLoadState('networkidle');

    // Wait for glass components to load
    await expect(page.locator('.glass').first()).toBeVisible({ timeout: 10000 });

    const glassElement = page.locator('.glass').first();

    // Verify glass effect properties
    const styles = await glassElement.evaluate((el) => {
      const computedStyle = getComputedStyle(el);
      return {
        backdropFilter: computedStyle.backdropFilter,
        background: computedStyle.background,
        borderRadius: computedStyle.borderRadius,
      };
    });

    // Verify glassmorphism properties exist regardless of theme
    expect(styles.backdropFilter).toContain('blur');
    expect(styles.borderRadius).toBeTruthy();
    expect(parseFloat(styles.borderRadius)).toBeGreaterThan(0);
  });

  test('should display components with current theme styling (PROTECTED)', async ({ page }) => {
    // Test multiple pages for consistent theming
    const testPages = [
      '/',
      '/mobile/feeds',
      '/mobile/feeds/stats',
    ];

    for (const testPage of testPages) {
      await page.goto(testPage);
      await page.waitForLoadState('networkidle');

      // Wait a bit for theme to be applied
      await page.waitForTimeout(1000);

      // Verify consistent theme application
      const body = page.locator('body');
      const bodyStyles = await body.evaluate((el) => {
        const computedStyle = getComputedStyle(el);
        return {
          backgroundColor: computedStyle.backgroundColor,
          color: computedStyle.color,
        };
      });

      // Verify theme styles are applied
      expect(bodyStyles.backgroundColor).toBeTruthy();
      expect(bodyStyles.color).toBeTruthy();

      // Look for theme-specific CSS classes or data attributes
      const hasThemeClass = await body.evaluate((el) => {
        return el.className.includes('theme-') ||
               el.hasAttribute('data-theme') ||
               el.hasAttribute('data-style') ||
               document.documentElement.hasAttribute('data-theme') ||
               document.documentElement.hasAttribute('data-style');
      });

      // At least one theme identifier should be present
      expect(hasThemeClass).toBeTruthy();
    }
  });
});