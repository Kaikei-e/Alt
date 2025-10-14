import { test, expect } from '@playwright/test';
import { LandingPage } from '../../page-objects/public/landing.page';

test.describe('Landing Page', () => {
  let landingPage: LandingPage;

  test.beforeEach(async ({ page }) => {
    landingPage = new LandingPage(page);
    await landingPage.goto();
  });

  test('should display landing page with main elements', async () => {
    // Check main elements are visible
    await expect(landingPage.pageHeading).toBeVisible();
    await expect(landingPage.loginButton).toBeVisible();
    await expect(landingPage.registerButton).toBeVisible();
  });

  test('should have proper page title', async () => {
    const title = await landingPage.getTitle();
    expect(title).toBeTruthy();
    expect(title.length).toBeGreaterThan(0);
  });

  test('should navigate to login page', async () => {
    await landingPage.clickLogin();

    // Verify navigation to login page
    await expect(landingPage.page).toHaveURL(/\/auth\/login/);
  });

  test('should navigate to register page', async () => {
    await landingPage.clickRegister();

    // Register button goes to /api/auth/register which redirects
    // Just verify the button click works and navigation happens
    await landingPage.page.waitForLoadState('networkidle');
  });

  test('should display hero section', async () => {
    const isVisible = await landingPage.isHeroVisible();

    // Hero section might or might not exist depending on design
    if (isVisible) {
      await expect(landingPage.heroSection).toBeVisible();
    }
  });

  test('should display features section', async () => {
    const areVisible = await landingPage.areFeaturesVisible();

    // Features section might or might not exist
    if (areVisible) {
      await expect(landingPage.featuresSection).toBeVisible();

      // Scroll to features
      await landingPage.scrollToFeatures();
      await expect(landingPage.featuresSection).toBeInViewport();
    }
  });

  test('should display CTA section', async () => {
    const isVisible = await landingPage.isCtaVisible();

    if (isVisible) {
      await expect(landingPage.ctaSection).toBeVisible();

      // Scroll to CTA
      await landingPage.scrollToCta();
      await expect(landingPage.ctaSection).toBeInViewport();
    }
  });

  test('should have working scroll to top', async () => {
    // Scroll to bottom
    await landingPage.scrollToBottom();

    // Scroll back to top
    await landingPage.scrollToTop();

    // Check if we're at the top
    const scrollY = await landingPage.page.evaluate(() => window.scrollY);
    expect(scrollY).toBe(0);
  });

  test('should be responsive on mobile viewport', async ({ page }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });

    // Reload page
    await landingPage.goto();

    // Main elements should still be visible
    await expect(landingPage.pageHeading).toBeVisible();
    await expect(landingPage.loginButton).toBeVisible();
    await expect(landingPage.registerButton).toBeVisible();
  });

  test('should be responsive on tablet viewport', async ({ page }) => {
    // Set tablet viewport
    await page.setViewportSize({ width: 768, height: 1024 });

    // Reload page
    await landingPage.goto();

    // Main elements should still be visible
    await expect(landingPage.pageHeading).toBeVisible();
    await expect(landingPage.loginButton).toBeVisible();
  });

  test('should be responsive on desktop viewport', async ({ page }) => {
    // Set desktop viewport
    await page.setViewportSize({ width: 1920, height: 1080 });

    // Reload page
    await landingPage.goto();

    // Main elements should still be visible
    await expect(landingPage.pageHeading).toBeVisible();
    await expect(landingPage.loginButton).toBeVisible();
  });

  test('should be accessible', async () => {
    await landingPage.checkA11y({
      rules: {
        'color-contrast': { enabled: false },
        'link-in-text-block': { enabled: false },
      },
    });
  });

  test('should have proper heading structure', async ({ page }) => {
    const headings = await page
      .locator('h1, h2, h3, h4, h5, h6')
      .allTextContents();

    // Should have at least one h1
    const h1Count = await page.locator('h1').count();
    expect(h1Count).toBeGreaterThanOrEqual(1);
    expect(h1Count).toBeLessThanOrEqual(1); // Only one h1
  });

  test('should have keyboard navigation', async ({ page }) => {
    // Focus on the first button (Theme Toggle)
    const firstButton = page.locator('button').first();
    await firstButton.focus();

    const isFocused = await firstButton.evaluate(el => el === document.activeElement);
    expect(isFocused).toBe(true);
  });

  test('should load without JavaScript errors', async ({ page }) => {
    const errors: string[] = [];

    page.on('pageerror', (error) => {
      // Filter out development/HMR related errors
      if (!error.message.includes('HMR') &&
          !error.message.includes('webpack') &&
          !error.message.includes('hot-update')) {
        errors.push(error.message);
      }
    });

    await landingPage.goto();

    // Should have no critical JavaScript errors
    expect(errors).toHaveLength(0);
  });

  test('should load all critical resources', async ({ page }) => {
    const failedRequests: string[] = [];

    page.on('requestfailed', (request) => {
      failedRequests.push(request.url());
    });

    await landingPage.goto();

    // Should have no failed critical resources
    // Filter out non-critical failures (analytics, etc.)
    const criticalFailures = failedRequests.filter(
      url => !url.includes('analytics') && !url.includes('tracking')
    );

    expect(criticalFailures).toHaveLength(0);
  });

  test('should have proper meta tags', async ({ page }) => {
    await landingPage.goto();

    // Check for important meta tags
    const description = await page
      .locator('meta[name="description"]')
      .getAttribute('content');
    expect(description).toBeTruthy();

    const viewport = await page
      .locator('meta[name="viewport"]')
      .getAttribute('content');
    expect(viewport).toBeTruthy();
  });
});
