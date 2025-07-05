import { test, expect } from '@playwright/test';

test.describe('Right Panel Analytics', () => {
  test.beforeEach(async ({ page }) => {
    // Mock API responses for analytics
    await page.route('/api/analytics/reading-stats', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          today: {
            articlesRead: 12,
            timeSpent: 45,
            favoriteCount: 3,
            completionRate: 78,
            avgReadingTime: 4.2,
            topCategories: [
              { category: 'Tech', count: 8, percentage: 67, color: 'var(--accent-primary)' },
              { category: 'Design', count: 3, percentage: 25, color: 'var(--accent-secondary)' }
            ]
          },
          week: {
            totalArticles: 67,
            totalTime: 245,
            dailyBreakdown: [
              { day: 'Mon', articles: 8, timeSpent: 32, completion: 75 },
              { day: 'Tue', articles: 12, timeSpent: 45, completion: 78 }
            ],
            trendDirection: 'up',
            weekOverWeek: 15
          },
          streak: {
            current: 7,
            longest: 23,
            lastReadDate: '2024-01-15T10:30:00Z'
          }
        })
      });
    });

    await page.route('/api/analytics/trending-topics', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { tag: 'AI', count: 45, trend: 'up', trendValue: 23, category: 'tech', color: 'var(--accent-primary)' },
          { tag: 'React', count: 32, trend: 'up', trendValue: 12, category: 'development', color: 'var(--accent-secondary)' }
        ])
      });
    });

    await page.route('/api/analytics/source-analytics', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          {
            id: 'techcrunch',
            name: 'TechCrunch',
            icon: '📰',
            unreadCount: 12,
            totalArticles: 145,
            avgReadingTime: 4.2,
            reliability: 9.2,
            lastUpdate: '2024-01-15T10:30:00Z',
            engagement: 89,
            category: 'tech'
          }
        ])
      });
    });
  });

  test('should display analytics with glassmorphism effect', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    const rightPanel = page.locator('.glass').last();
    await expect(rightPanel).toBeVisible();
    
    // Analytics tab should be active by default
    const analyticsTab = page.locator('button', { hasText: 'Analytics' });
    await expect(analyticsTab).toHaveAttribute('aria-selected', 'true');
    
    // Should show reading stats using CSS variables
    const statsElements = page.locator('[data-testid*="stat"]');
    if (await statsElements.count() > 0) {
      const styles = await statsElements.first().evaluate(el => getComputedStyle(el));
      expect(styles.color).toBeDefined();
    }
  });

  test('should switch between tabs', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    // Switch to Actions tab
    await page.click('button:has-text("Actions")');
    await expect(page.locator('text=Quick Actions')).toBeVisible();
    
    // Switch back to Analytics tab
    await page.click('button:has-text("Analytics")');
    await expect(page.locator('text=Today\'s Reading')).toBeVisible();
  });

  test('should display reading analytics correctly', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    // Wait for analytics data to load
    await page.waitForSelector('text=Today\'s Reading');
    
    // Check today's stats
    await expect(page.locator('text=12').first()).toBeVisible(); // Articles read
    await expect(page.locator('text=45m')).toBeVisible(); // Time spent
    await expect(page.locator('text=3').nth(1)).toBeVisible(); // Favorites
    
    // Check weekly trend
    await expect(page.locator('text=Weekly Trend')).toBeVisible();
    await expect(page.locator('text=67')).toBeVisible(); // Total articles
    
    // Check reading streak
    await expect(page.locator('text=Reading Streak')).toBeVisible();
    await expect(page.locator('text=7').first()).toBeVisible(); // Current streak
  });

  test('should display trending topics', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    // Wait for trending topics to load
    await page.waitForSelector('text=Trending Topics');
    
    // Check trending topics
    await expect(page.locator('text=#AI')).toBeVisible();
    await expect(page.locator('text=#React')).toBeVisible();
    await expect(page.locator('text=45 articles')).toBeVisible();
    await expect(page.locator('text=+23%')).toBeVisible(); // Trend value
  });

  test('should display source analytics', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    // Wait for source analytics to load
    await page.waitForSelector('text=Source Analytics');
    
    // Check source information
    await expect(page.locator('text=TechCrunch')).toBeVisible();
    await expect(page.locator('text=145')).toBeVisible(); // Total articles
    await expect(page.locator('text=9.2/10')).toBeVisible(); // Reliability
  });

  test('should show quick actions in Actions tab', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    // Switch to Actions tab
    await page.click('button:has-text("Actions")');
    
    // Check quick actions
    await expect(page.locator('text=Quick Actions')).toBeVisible();
    await expect(page.locator('text=View Unread')).toBeVisible();
    await expect(page.locator('text=View Bookmarks')).toBeVisible();
    await expect(page.locator('text=Reading Queue')).toBeVisible();
  });

  test('should display bookmarks and reading queue', async ({ page }) => {
    await page.goto('/desktop/feeds');
    
    // Switch to Actions tab
    await page.click('button:has-text("Actions")');
    
    // Check bookmarks section
    await expect(page.locator('text=Recent Bookmarks')).toBeVisible();
    
    // Check reading queue section
    await expect(page.locator('text=Reading Queue')).toBeVisible();
  });

  test('should handle loading states', async ({ page }) => {
    // Mock delayed response
    await page.route('/api/analytics/reading-stats', async (route) => {
      await new Promise(resolve => setTimeout(resolve, 1000));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({})
      });
    });

    await page.goto('/desktop/feeds');
    
    // Should show loading spinner initially
    const spinner = page.getByRole('progressbar').first();
    await expect(spinner).toBeVisible();
  });

  test('should be responsive', async ({ page }) => {
    // Test desktop view
    await page.setViewportSize({ width: 1200, height: 800 });
    await page.goto('/desktop/feeds');
    
    const rightPanel = page.locator('.glass').last();
    await expect(rightPanel).toBeVisible();
    
    // Test tablet view
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(500); // Allow for responsive changes
    
    // Panel should still be visible but may have different layout
    await expect(rightPanel).toBeVisible();
  });
});