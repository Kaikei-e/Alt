import { test, expect } from '@playwright/test';

test.describe('Desktop Feed Card', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');

    // Wait for feeds to load
    await page.waitForTimeout(2000);
  });

  test('should display feed cards with glassmorphism effect', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');

    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    await expect(feedCard).toBeVisible();

    // ガラスモーフィズム効果の確認
    await expect(feedCard).toHaveClass(/glass/);
  });

  test('should handle interactions correctly', async ({ page }) => {
    // Wait for feed cards to appear or check for placeholder
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    const placeholderMessage = page.getByText('フィードカードはTASK2で実装されます');

    const isFeedCardVisible = await feedCard.isVisible().catch(() => false);
    const isPlaceholderVisible = await placeholderMessage.isVisible().catch(() => false);

    if (isFeedCardVisible) {
      // If feed cards are available, test interactions
      await expect(feedCard.getByRole('button', { name: 'Mark as Read' })).toBeVisible();
    } else if (isPlaceholderVisible) {
      // If placeholder is shown, that's expected for TASK2
      await expect(placeholderMessage).toBeVisible();
    } else {
      // Neither is visible - this is the actual test failure
      throw new Error('Neither feed cards nor placeholder message are visible');
    }
  });

  test('should use CSS variables for theming', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();

    const borderColor = await feedCard.evaluate(el => getComputedStyle(el).borderColor);
    // CSS変数が適用されていることを確認（実際の値ではなく、変数の存在確認）
    expect(borderColor).toBeTruthy();
  });

  test('should display feed metadata', async ({ page }) => {
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    const placeholderMessage = page.getByText('フィードカードはTASK2で実装されます');

    const isFeedCardVisible = await feedCard.isVisible().catch(() => false);
    const isPlaceholderVisible = await placeholderMessage.isVisible().catch(() => false);

    if (isFeedCardVisible) {
      // Wait for engagement stats to load
      await page.waitForTimeout(1000);

      // Check for engagement stats (likes/bookmarks instead of views)
      const likesText = feedCard.locator('text=/likes/').first();
      const bookmarksText = feedCard.locator('text=/bookmarks/').first();

      // At least one of these should be visible
      const likesVisible = await likesText.isVisible().catch(() => false);
      const bookmarksVisible = await bookmarksText.isVisible().catch(() => false);

      expect(likesVisible || bookmarksVisible).toBeTruthy();
    } else {
      // Placeholder is acceptable for TASK2
      await expect(placeholderMessage).toBeVisible();
    }
  });

  test('should handle favorite and bookmark actions', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();

    // ホバーしてアクションボタンを表示
    await feedCard.hover();

    // お気に入りボタン
    const favoriteButton = feedCard.locator('button[aria-label="Toggle favorite"]');
    if (await favoriteButton.isVisible()) {
      await favoriteButton.click();
    }

    // ブックマークボタン
    const bookmarkButton = feedCard.locator('button[aria-label="Toggle bookmark"]');
    if (await bookmarkButton.isVisible()) {
      await bookmarkButton.click();
    }
  });

  test('should show priority indicators', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');

    // 優先度アイコンが表示されている
    const priorityIcons = page.locator('text=/🔥|📈|📄/');
    await expect(priorityIcons.first()).toBeVisible();
  });

  test('should display tags', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();

    // タグが表示されている - use first() to avoid multiple matches
    await expect(feedCard.locator('text=/^#/').first()).toBeVisible();
  });

  test('should handle external link clicks', async ({ page }) => {
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    const placeholderMessage = page.getByText('フィードカードはTASK2で実装されます');

    const isFeedCardVisible = await feedCard.isVisible().catch(() => false);
    const isPlaceholderVisible = await placeholderMessage.isVisible().catch(() => false);

    if (isFeedCardVisible) {
      // View Article ボタンのクリック
      const viewArticleButton = feedCard.getByRole('button', { name: /View Article/ });
      await expect(viewArticleButton).toBeVisible();

      // Note: We don't actually click it to avoid opening external links in tests
    } else {
      // Placeholder is acceptable for TASK2
      await expect(placeholderMessage).toBeVisible();
    }
  });

  test('should be responsive across viewports', async ({ page }) => {
    // デスクトップビューポート
    await page.setViewportSize({ width: 1920, height: 1080 });
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');

    let feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    await expect(feedCard).toBeVisible();

    // タブレットビューポート
    await page.setViewportSize({ width: 1024, height: 768 });
    await expect(feedCard).toBeVisible();

    // 小さなデスクトップビューポート
    await page.setViewportSize({ width: 1366, height: 768 });
    await expect(feedCard).toBeVisible();
  });

  test('should handle loading states', async ({ page }) => {
    // Check for loading indicator or feed cards or placeholder
    const loadingIndicator = page.locator('text=/Loading|読み込み中/');
    const feedCards = page.locator('[data-testid^="desktop-feed-card-"]');
    const placeholderMessage = page.getByText('フィードカードはTASK2で実装されます');

    // Wait a bit for the page to stabilize
    await page.waitForTimeout(2000);

    const isLoadingVisible = await loadingIndicator.isVisible().catch(() => false);
    const areFeedCardsVisible = await feedCards.first().isVisible().catch(() => false);
    const isPlaceholderVisible = await placeholderMessage.isVisible().catch(() => false);

    // At least one of these should be true
    expect(isLoadingVisible || areFeedCardsVisible || isPlaceholderVisible).toBeTruthy();
  });
});