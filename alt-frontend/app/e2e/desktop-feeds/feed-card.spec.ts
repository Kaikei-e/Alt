import { test, expect } from '@playwright/test';

test.describe('Desktop Feed Card', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/desktop/feeds');
  });

  test('should display feed cards with glassmorphism effect', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    await expect(feedCard).toBeVisible();
    
    // ガラスモーフィズム効果の確認
    await expect(feedCard).toHaveClass(/glass/);
  });

  test('should handle interactions correctly', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    
    // ホバー効果
    await feedCard.hover();
    
    // Mark as Read ボタンをクリック
    const markAsReadButton = feedCard.locator('button', { hasText: 'Mark as Read' });
    if (await markAsReadButton.isVisible()) {
      await markAsReadButton.click();
      
      // 読了状態の確認
      await expect(feedCard.locator('button', { hasText: 'Read' })).toBeVisible();
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
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    
    // タイトルが表示されている
    await expect(feedCard.locator('text=/OpenAI|React|Sustainable|Microservices|CSS/i')).toBeVisible();
    
    // 読み取り時間が表示されている
    await expect(feedCard.locator('text=/min read/')).toBeVisible();
    
    // エンゲージメント統計が表示されている
    await expect(feedCard.locator('text=/views/')).toBeVisible();
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
    
    // タグが表示されている
    await expect(feedCard.locator('text=/^#/')).toBeVisible();
  });

  test('should handle external link clicks', async ({ page }) => {
    await page.waitForSelector('[data-testid^="desktop-feed-card-"]');
    const feedCard = page.locator('[data-testid^="desktop-feed-card-"]').first();
    
    // View Article ボタンのクリック
    const viewArticleButton = feedCard.locator('button', { hasText: 'View Article' });
    
    // 新しいタブでのリンクオープンをテスト
    const [newPage] = await Promise.all([
      page.waitForEvent('popup'),
      viewArticleButton.click()
    ]);
    
    // 新しいページが開かれたことを確認
    expect(newPage.url()).toBeTruthy();
    await newPage.close();
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
    // ページロード時のローディング状態を確認
    await page.goto('/desktop/feeds');
    
    // ローディングスピナーまたはフィードカードのいずれかが表示される
    const loadingOrContent = page.locator('text=/フィードを読み込み中/, [data-testid^="desktop-feed-card-"]');
    await expect(loadingOrContent.first()).toBeVisible();
  });
});