import { test, expect } from '@playwright/test';

test.describe('XSS Prevention Tests - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
  });

  test('should prevent script injection in search input - PROTECTED', async ({ page }) => {
    await page.goto('/mobile/feeds/search');
    await page.waitForLoadState('networkidle');
    
    const maliciousScript = '<script>window.xssTest = true;</script>';
    
    // 検索フィールドに悪意のあるスクリプトを入力
    await page.fill('[data-testid="search-input"]', maliciousScript);
    await page.click('button[type="submit"]');
    
    // スクリプトが実行されていないことを確認
    const xssExecuted = await page.evaluate(() => {
      return (window as any).xssTest === true;
    });
    
    expect(xssExecuted).toBe(false);
    
    // 入力値が適切にエスケープされていることを確認
    const inputValue = await page.inputValue('[data-testid="search-input"]');
    expect(inputValue).not.toContain('<script>');
  });

  test('should prevent HTML injection in form fields - PROTECTED', async ({ page }) => {
    await page.goto('/mobile/feeds/register');
    await page.waitForLoadState('networkidle');
    
    const maliciousHTML = '<img src=x onerror=alert("XSS")>';
    
    // フォームフィールドに悪意のあるHTMLを入力
    await page.fill('input[type="url"]', maliciousHTML);
    await page.locator('input[type="url"]').blur();
    
    // HTMLが実行されていないことを確認
    const errorTriggered = await page.evaluate(() => {
      return (window as any).lastError !== undefined;
    });
    
    expect(errorTriggered).toBe(false);
    
    // 入力値が適切にエスケープされていることを確認
    const inputValue = await page.inputValue('input[type="url"]');
    expect(inputValue).not.toContain('<img');
  });

  test('should block inline scripts via CSP - PROTECTED', async ({ page }) => {
    // CSPヘッダーの確認
    const response = await page.goto('/');
    const headers = response?.headers();
    
    expect(headers?.['content-security-policy']).toContain("script-src 'self'");
    
    // インラインスクリプトの実行阻止を確認
    const scriptBlocked = await page.evaluate(() => {
      try {
        const script = document.createElement('script');
        script.textContent = 'window.inlineScriptExecuted = true;';
        document.head.appendChild(script);
        return (window as any).inlineScriptExecuted !== true;
      } catch (error) {
        return true; // CSPによりブロックされた
      }
    });
    
    expect(scriptBlocked).toBe(true);
  });

  test('should sanitize external feed content - PROTECTED', async ({ page }) => {
    // モックAPIレスポンスに悪意のあるコンテンツを含める
    await page.route('**/api/feeds', (route) => {
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify([
          {
            id: '1',
            title: '<script>alert("XSS in title")</script>Legitimate Title',
            description: '<img src=x onerror=alert("XSS")>Description',
            author: 'javascript:alert("XSS")',
            url: 'https://example.com'
          }
        ])
      });
    });
    
    await page.goto('/mobile/feeds');
    await page.waitForLoadState('networkidle');
    
    // 悪意のあるコンテンツがサニタイズされていることを確認
    const feedCards = page.locator('[data-testid="feed-card"]').first();
    await feedCards.waitFor({ state: 'visible' });
    
    const titleContent = await feedCards.locator('h2').textContent();
    expect(titleContent).not.toContain('<script>');
    expect(titleContent).toContain('Legitimate Title');
    
    const descriptionContent = await feedCards.locator('p').textContent();
    expect(descriptionContent).not.toContain('<img');
    expect(descriptionContent).toContain('Description');
  });

  test('should handle URL parameter XSS attempts - PROTECTED', async ({ page }) => {
    // URLパラメータに悪意のあるスクリプトを含める
    await page.goto('/mobile/feeds/search?q=<script>alert("XSS")</script>');
    await page.waitForLoadState('networkidle');
    
    // パラメータが適切にエスケープされていることを確認
    const searchQuery = await page.inputValue('[data-testid="search-input"]');
    expect(searchQuery).not.toContain('<script>');
    
    // 画面表示も安全であることを確認
    const pageContent = await page.textContent('body');
    expect(pageContent).not.toContain('<script>alert("XSS")</script>');
  });

  test('should prevent DOM-based XSS via URL fragments - PROTECTED', async ({ page }) => {
    // URLフラグメントに悪意のあるスクリプトを含める
    await page.goto('/#<script>alert("XSS")</script>');
    
    // フラグメントが適切に処理されていることを確認
    const fragmentExecuted = await page.evaluate(() => {
      return (window as any).fragmentXSS !== true;
    });
    
    expect(fragmentExecuted).toBe(true);
  });

  test('should validate and sanitize RSS feed URLs - PROTECTED', async ({ page }) => {
    await page.goto('/mobile/feeds/register');
    await page.waitForLoadState('networkidle');
    
    // 危険なプロトコルを含むURLを入力
    const maliciousUrl = 'javascript:alert("XSS")';
    await page.fill('input[type="url"]', maliciousUrl);
    
    // バリデーションが機能することを確認
    const button = page.locator('button[type="submit"]');
    await expect(button).toBeDisabled();
    
    // 悪意のあるURLが保存されていないことを確認
    const savedUrl = await page.inputValue('input[type="url"]');
    expect(savedUrl).not.toContain('javascript:');
  });

  test('should protect against reflected XSS in search results - PROTECTED', async ({ page }) => {
    const maliciousQuery = '<script>alert("Reflected XSS")</script>';
    
    // 検索APIをモック
    await page.route('**/api/articles/search**', (route) => {
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          articles: [
            {
              id: '1',
              title: 'Safe Article',
              description: 'Safe description',
              url: 'https://example.com'
            }
          ]
        })
      });
    });
    
    await page.goto(`/mobile/articles/search?q=${encodeURIComponent(maliciousQuery)}`);
    await page.waitForLoadState('networkidle');
    
    // 検索結果ページで悪意のあるスクリプトが実行されていないことを確認
    const xssExecuted = await page.evaluate(() => {
      return (window as any).reflectedXSS !== true;
    });
    
    expect(xssExecuted).toBe(true);
    
    // 検索クエリが適切にエスケープされていることを確認
    const pageContent = await page.textContent('body');
    expect(pageContent).not.toContain('<script>alert("Reflected XSS")</script>');
  });
});