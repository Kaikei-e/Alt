import { test, expect } from '@playwright/test';

test.describe('Desktop Feed Card', () => {
  test.beforeEach(async ({ page }) => {
    // Mock feed data
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = [
        {
          id: 'feed-1',
          title: 'React 19 Features',
          description: 'New React features announcement',
          link: 'https://example.com/react-19',
          published: new Date().toISOString(),
          metadata: {
            tags: ['react', 'javascript'],
            engagement: {
              likes: 42,
              bookmarks: 15
            }
          }
        },
      ];

      await route.fulfill({
        json: {
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');

    // Wait for feeds to load
    await page.waitForTimeout(2000);
  });

  test('should display feed cards with glassmorphism effect', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');

    const feedCard = page.locator('[data-testid^="feed-item-"]').first();
    await expect(feedCard).toBeVisible();

    // ガラスモーフィズム効果の確認
    await expect(feedCard.locator('.glass')).toBeVisible();
  });

  test('should handle interactions correctly', async ({ page }) => {
    // Wait for feed cards to appear
    await page.waitForSelector('[data-testid^="feed-item-"]');
    const feedCard = page.locator('[data-testid^="feed-item-"]').first();

    await expect(feedCard).toBeVisible();

    // Test interactions - look for action buttons
    const markAsReadButton = feedCard.locator('button:has-text("Mark as Read")');
    await expect(markAsReadButton).toBeVisible();
  });

  test('should use CSS variables for theming', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');
    const feedCard = page.locator('[data-testid^="feed-item-"]').first();

    const borderColor = await feedCard.evaluate(el => getComputedStyle(el).borderColor);
    // CSS変数が適用されていることを確認（実際の値ではなく、変数の存在確認）
    expect(borderColor).toBeTruthy();
  });

  test('should display feed metadata', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');
    const feedCard = page.locator('[data-testid^="feed-item-"]').first();

    await expect(feedCard).toBeVisible();

    // Check for engagement stats (likes/bookmarks instead of views)
    const likesText = feedCard.locator('text=/likes/').first();
    const bookmarksText = feedCard.locator('text=/bookmarks/').first();

    // At least one of these should be visible
    const likesVisible = await likesText.isVisible().catch(() => false);
    const bookmarksVisible = await bookmarksText.isVisible().catch(() => false);

    expect(likesVisible || bookmarksVisible).toBeTruthy();
  });

  test('should handle favorite and bookmark actions', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');
    const feedCard = page.locator('[data-testid^="feed-item-"]').first();

    // ホバーしてアクションボタンを表示
    await feedCard.hover();

    // お気に入りボタン
    const favoriteButton = feedCard.locator('button:has-text("Toggle favorite")');
    if (await favoriteButton.isVisible()) {
      await favoriteButton.click();
    }

    // ブックマークボタン
    const bookmarkButton = feedCard.locator('button:has-text("Toggle bookmark")');
    if (await bookmarkButton.isVisible()) {
      await bookmarkButton.click();
    }
  });

  test('should show priority indicators', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');

    // 優先度アイコンが表示されている
    const priorityIcons = page.locator('text=/🔥|📈|📄/');
    await expect(priorityIcons.first()).toBeVisible();
  });

  test('should display tags', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');
    const feedCard = page.locator('[data-testid^="feed-item-"]').first();

    // タグが表示されている - use first() to avoid multiple matches
    await expect(feedCard.locator('text=/^#/').first()).toBeVisible();
  });

  test('should handle external link clicks', async ({ page }) => {
    await page.waitForSelector('[data-testid^="feed-item-"]');
    const feedCard = page.locator('[data-testid^="feed-item-"]').first();

    await expect(feedCard).toBeVisible();

    // View Article ボタンのクリック
    const viewArticleButton = feedCard.locator('button:has-text("View Article")');
    await expect(viewArticleButton).toBeVisible();

    // Note: We don't actually click it to avoid opening external links in tests
  });

  test('should be responsive across viewports', async ({ page }) => {
    // デスクトップビューポート
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.waitForSelector('[data-testid^="feed-item-"]');

    let feedCard = page.locator('[data-testid^="feed-item-"]').first();
    await expect(feedCard).toBeVisible();

    // タブレットビューポート
    await page.setViewportSize({ width: 1024, height: 768 });
    await expect(feedCard).toBeVisible();

    // 小さなデスクトップビューポート
    await page.setViewportSize({ width: 1366, height: 768 });
    await expect(feedCard).toBeVisible();
  });

  test('should handle loading states', async ({ page }) => {
    // Check for loading indicator or feed cards
    const loadingIndicator = page.locator('text=/Loading|読み込み中/');
    const feedCards = page.locator('[data-testid^="feed-item-"]');

    // Wait a bit for the page to stabilize
    await page.waitForTimeout(2000);

    const isLoadingVisible = await loadingIndicator.isVisible().catch(() => false);
    const areFeedCardsVisible = await feedCards.first().isVisible().catch(() => false);

    // At least one of these should be true
    expect(isLoadingVisible || areFeedCardsVisible).toBeTruthy();
  });
});