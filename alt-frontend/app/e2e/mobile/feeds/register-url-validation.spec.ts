import { test, expect } from '@playwright/test';

test.describe('Feed Registration URL Validation', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/mobile/feeds/register');
  });

  test('should accept valid RSS feed URLs', async ({ page }) => {
    const validUrls = [
      'https://feeds.feedburner.com/example',
      'https://example.com/rss.xml',
      'https://example.com/feed.xml',
      'https://example.com/atom.xml',
      'https://example.com/feed',
      'https://example.com/rss',
    ];

    for (const url of validUrls) {
      await page.fill('input[type="url"]', url);
      await page.locator('input[type="url"]').blur();
      
      // Check that no validation error is shown
      const errorMessage = page.locator('text="Invalid or unsafe URL"');
      await expect(errorMessage).not.toBeVisible();
      
      // Check that submit button is enabled
      const submitButton = page.locator('button[type="submit"]');
      await expect(submitButton).toBeEnabled();
      
      // Clear field for next test
      await page.fill('input[type="url"]', '');
    }
  });

  test('should reject dangerous URLs', async ({ page }) => {
    const dangerousUrls = [
      'javascript:alert("XSS")',
      'data:text/html,<script>alert("XSS")</script>',
      'vbscript:alert("XSS")',
      'file:///etc/passwd',
    ];

    for (const url of dangerousUrls) {
      await page.fill('input[type="url"]', url);
      await page.locator('input[type="url"]').blur();
      
      // Check that validation error is shown
      const errorMessage = page.locator('text="Invalid or unsafe URL"');
      await expect(errorMessage).toBeVisible();
      
      // Check that submit button is disabled
      const submitButton = page.locator('button[type="submit"]');
      await expect(submitButton).toBeDisabled();
      
      // Clear field for next test
      await page.fill('input[type="url"]', '');
    }
  });

  test('should reject non-RSS URLs', async ({ page }) => {
    const nonRssUrls = [
      'https://example.com',
      'https://example.com/about',
      'https://example.com/blog',
      'https://example.com/page.html',
    ];

    for (const url of nonRssUrls) {
      await page.fill('input[type="url"]', url);
      await page.locator('input[type="url"]').blur();
      
      // Check that validation error is shown
      const errorMessage = page.locator('text="URL does not appear to be a valid RSS or Atom feed"');
      await expect(errorMessage).toBeVisible();
      
      // Check that submit button is disabled
      const submitButton = page.locator('button[type="submit"]');
      await expect(submitButton).toBeDisabled();
      
      // Clear field for next test
      await page.fill('input[type="url"]', '');
    }
  });

  test('should show validation error in real-time', async ({ page }) => {
    // Start typing a dangerous URL
    await page.fill('input[type="url"]', 'javascript:alert');
    await page.locator('input[type="url"]').blur();
    
    // Error should appear immediately
    const errorMessage = page.locator('text="Invalid or unsafe URL"');
    await expect(errorMessage).toBeVisible();
    
    // Submit button should be disabled
    const submitButton = page.locator('button[type="submit"]');
    await expect(submitButton).toBeDisabled();
    
    // Input border should be red
    const input = page.locator('input[type="url"]');
    await expect(input).toHaveCSS('border-color', 'rgb(255, 0, 0)'); // Assuming red color
  });

  test('should clear validation error when input is cleared', async ({ page }) => {
    // Enter invalid URL
    await page.fill('input[type="url"]', 'javascript:alert("XSS")');
    await page.locator('input[type="url"]').blur();
    
    // Error should be visible
    const errorMessage = page.locator('text="Invalid or unsafe URL"');
    await expect(errorMessage).toBeVisible();
    
    // Clear the input
    await page.fill('input[type="url"]', '');
    
    // Error should be gone
    await expect(errorMessage).not.toBeVisible();
    
    // Submit button should be enabled (assuming no other validation errors)
    const submitButton = page.locator('button[type="submit"]');
    await expect(submitButton).toBeEnabled();
  });

  test('should handle edge cases correctly', async ({ page }) => {
    const edgeCases = [
      {
        url: 'https://example.com/feed.xml?format=rss',
        shouldBeValid: true,
        description: 'RSS URL with query parameters'
      },
      {
        url: 'https://example.com/rss#latest',
        shouldBeValid: true,
        description: 'RSS URL with fragment'
      },
      {
        url: 'https://subdomain.example.com/feeds/all',
        shouldBeValid: true,
        description: 'RSS URL with subdomain'
      }
    ];

    for (const testCase of edgeCases) {
      await page.fill('input[type="url"]', testCase.url);
      await page.locator('input[type="url"]').blur();
      
      const submitButton = page.locator('button[type="submit"]');
      
      if (testCase.shouldBeValid) {
        await expect(submitButton).toBeEnabled();
      } else {
        await expect(submitButton).toBeDisabled();
      }
      
      // Clear field for next test
      await page.fill('input[type="url"]', '');
    }
  });
});