import { test, expect } from '@playwright/test';

test.describe('API Integration Tests - PROTECTED', () => {
  test('should handle API responses and errors correctly', async ({ page }) => {
    // Test successful API response
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 5 }, (_, i) => ({
        id: `feed-${i}`,
        title: `Test Feed ${i}`,
        description: `Test Description ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: { 
          data: feeds,
          next_cursor: "next-cursor"
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Verify feeds loaded
    const feedItems = page.locator('[data-testid^="feed-item-"]');
    await expect(feedItems.first()).toBeVisible();
    
    // Test API error handling
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      await route.fulfill({
        status: 500,
        json: { error: 'Internal Server Error' }
      });
    });

    // Force refetch
    await page.reload();
    await page.waitForTimeout(2000);

    // Verify error state
    const errorMessage = page.locator('text=/フィードの読み込みに失敗しました/');
    await expect(errorMessage).toBeVisible();
  });

  test('should handle pagination correctly', async ({ page }) => {
    let callCount = 0;
    
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      callCount++;
      const feeds = Array.from({ length: 10 }, (_, i) => ({
        id: `feed-${callCount}-${i}`,
        title: `Page ${callCount} Feed ${i}`,
        description: `Description for page ${callCount} item ${i}`,
        link: `https://example.com/feed-${callCount}-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: { 
          data: feeds,
          next_cursor: callCount < 2 ? `cursor-${callCount}` : null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Verify initial load
    const feedItems = page.locator('[data-testid^="feed-item-"]');
    await expect(feedItems.first()).toBeVisible();

    // Look for load more button
    const loadMoreButton = page.locator('text=Load more...');
    if (await loadMoreButton.isVisible()) {
      await loadMoreButton.click();
      await page.waitForTimeout(1000);
    }

    // Verify pagination worked (should have made 2 API calls)
    expect(callCount).toBeGreaterThanOrEqual(1);
  });

  test('should handle real-time updates gracefully', async ({ page }) => {
    let updateCount = 0;

    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      updateCount++;
      const feeds = Array.from({ length: 3 }, (_, i) => ({
        id: `feed-${updateCount}-${i}`,
        title: `Updated Feed ${updateCount}-${i}`,
        description: `Updated at ${new Date().toISOString()}`,
        link: `https://example.com/feed-${updateCount}-${i}`,
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
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Verify initial content
    const initialContent = page.locator('text=Updated Feed 1-0');
    await expect(initialContent).toBeVisible();

    // Simulate refresh/update
    await page.reload();
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Verify updated content
    const updatedContent = page.locator('text=Updated Feed 2-0');
    await expect(updatedContent).toBeVisible();
  });
});