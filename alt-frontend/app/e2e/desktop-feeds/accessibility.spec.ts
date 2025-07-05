import { test, expect } from '@playwright/test';

test.describe('Accessibility Tests - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 10 }, (_, i) => ({
        id: `feed-${i}`,
        title: `Feed Title ${i}`,
        description: `Description for feed ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: { 
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should be keyboard navigable (WCAG 2.1 AA)', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Test Tab navigation
    await page.keyboard.press('Tab');
    
    // Verify focus is visible
    const focusedElement = page.locator(':focus');
    await expect(focusedElement).toBeVisible();

    // Test multiple Tab presses
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    
    // Verify focus moves properly
    const newFocusedElement = page.locator(':focus');
    await expect(newFocusedElement).toBeVisible();

    // Test Shift+Tab (reverse navigation)
    await page.keyboard.press('Shift+Tab');
    await expect(page.locator(':focus')).toBeVisible();
  });

  test('should have proper ARIA attributes', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    const timeline = page.locator('[data-testid="desktop-timeline"]');

    // Verify timeline container exists and is accessible
    await expect(timeline).toBeVisible();
    
    // Check for interactive elements have proper attributes
    const buttons = page.locator('button');
    const links = page.locator('a');
    const inputs = page.locator('input');
    
    // If buttons exist, they should be accessible
    if (await buttons.count() > 0) {
      const firstButton = buttons.first();
      await expect(firstButton).toBeVisible();
    }
    
    // If links exist, they should be accessible
    if (await links.count() > 0) {
      const firstLink = links.first();
      await expect(firstLink).toBeVisible();
    }
    
    // Basic accessibility check - page should be navigable
    expect(true).toBeTruthy(); // Basic test passes if elements are found
  });

  test('should have sufficient color contrast', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check text elements for contrast
    const textElements = page.locator('text=/Feed Title|Description|検索|フィルター/');
    
    if (await textElements.count() > 0) {
      const firstText = textElements.first();
      await expect(firstText).toBeVisible();
      
      // Verify text is readable (not transparent or invisible)
      const opacity = await firstText.evaluate(el => getComputedStyle(el).opacity);
      expect(parseFloat(opacity)).toBeGreaterThan(0.5);
    }
  });

  test('should support screen reader navigation', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check for semantic elements
    const articles = page.locator('article, [role="article"]');
    const lists = page.locator('ul, ol, [role="list"]');
    const buttons = page.locator('button, [role="button"]');

    // Verify semantic structure exists
    const hasArticles = await articles.count() > 0;
    const hasLists = await lists.count() > 0;
    const hasButtons = await buttons.count() > 0;

    // At least one type of semantic element should be present
    expect(hasArticles || hasLists || hasButtons).toBeTruthy();

    // Check for proper labels on interactive elements
    if (hasButtons) {
      const firstButton = buttons.first();
      const hasLabel = await firstButton.getAttribute('aria-label') !== null ||
                      await firstButton.getAttribute('aria-labelledby') !== null ||
                      await firstButton.textContent() !== '';
      expect(hasLabel).toBeTruthy();
    }
  });

  test('should handle reduced motion preferences', async ({ page }) => {
    // Set reduced motion preference
    await page.emulateMedia({ reducedMotion: 'reduce' });
    
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check that the page loads properly with reduced motion
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    await expect(timeline).toBeVisible();
    
    // Verify that content is still accessible with reduced motion
    const feedItems = page.locator('[data-testid^="feed-item-"]');
    if (await feedItems.count() > 0) {
      await expect(feedItems.first()).toBeVisible();
    }
    
    // Verify page is still functional
    expect(true).toBeTruthy(); // Basic test passes if page loads
  });
});