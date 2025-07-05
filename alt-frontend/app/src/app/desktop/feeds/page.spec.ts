import { test, expect } from '@playwright/test';

// PROTECTED E2E TESTS - CLAUDE: DO NOT MODIFY
test.describe('Desktop Feeds Page - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/desktop/feeds');
    await page.waitForLoadState('networkidle');
  });

  test('should render feeds page with all components (PROTECTED)', async ({ page }) => {
    // Verify main layout components are present
    await expect(page.locator('[data-testid="desktop-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="desktop-sidebar-filters"]')).toBeVisible();
    await expect(page.locator('[data-testid="desktop-timeline"]')).toBeVisible();

    // Verify header shows correct unread count (86 from mock stats)
    await expect(page.locator('text=86')).toBeVisible();

    // Verify filter sections are present
    await expect(page.locator('[data-testid="filter-header-title"]')).toHaveText('Filters');
    await expect(page.locator('[data-testid="filter-read-status-label"]')).toHaveText('Read Status');
  });

  test('should handle theme switching between vaporwave and liquid-beige (PROTECTED)', async ({ page }) => {
    // Check initial liquid-beige theme (default)
    await expect(page.locator('html')).toHaveAttribute('data-style', 'liquid-beige');

    // Verify liquid-beige theme styling
    const body = page.locator('body');
    const bodyStyles = await body.evaluate(el => getComputedStyle(el));
    expect(bodyStyles.background).toContain('linear-gradient');

    // Switch to vaporwave theme
    await page.locator('[data-testid="theme-toggle"]').click();
    await expect(page.locator('html')).toHaveAttribute('data-style', 'vaporwave');

    // Verify vaporwave theme styling
    const newBodyStyles = await body.evaluate(el => getComputedStyle(el));
    expect(newBodyStyles.background).toContain('linear-gradient');
    expect(newBodyStyles.background).not.toBe(bodyStyles.background);

    // Verify glass components have different styling in vaporwave
    const sidebar = page.locator('[data-testid="desktop-sidebar-filters"]');
    const sidebarStyles = await sidebar.evaluate(el => getComputedStyle(el));
    expect(sidebarStyles.boxShadow).toBeTruthy();
  });

  test('should handle responsive layout across different viewports (PROTECTED)', async ({ page }) => {
    // Desktop layout (lg+)
    await page.setViewportSize({ width: 1200, height: 800 });
    await page.waitForTimeout(500);

    // Verify 3-column layout is visible
    await expect(page.locator('[data-testid="desktop-sidebar-filters"]')).toBeVisible();
    await expect(page.locator('[data-testid="desktop-timeline"]')).toBeVisible();
    const rightPanel = page.locator('[data-testid="right-panel"]');
    if (await rightPanel.count() > 0) {
      await expect(rightPanel).toBeVisible();
    }

    // Tablet layout (md)
    await page.setViewportSize({ width: 768, height: 800 });
    await page.waitForTimeout(500);

    // Verify 2-column layout
    await expect(page.locator('[data-testid="desktop-sidebar-filters"]')).toBeVisible();
    await expect(page.locator('[data-testid="desktop-timeline"]')).toBeVisible();

    // Mobile layout (sm)
    await page.setViewportSize({ width: 480, height: 800 });
    await page.waitForTimeout(500);

    // Verify 1-column layout (sidebar hidden on mobile)
    await expect(page.locator('[data-testid="desktop-sidebar-filters"]')).toBeHidden();
    await expect(page.locator('[data-testid="desktop-timeline"]')).toBeVisible();
  });

  test('should handle filter interactions correctly (PROTECTED)', async ({ page }) => {
    // Test read status filter
    await page.locator('[data-testid="filter-read-status-unread"]').click();
    await page.waitForTimeout(300);

    // Test feed source filter
    const firstCheckbox = page.locator('[data-testid="filter-source-checkbox"]').first();
    await firstCheckbox.click();
    await page.waitForTimeout(300);

    // Test time range filter
    await page.locator('[data-testid="filter-time-range-today"]').click();
    await page.waitForTimeout(300);

    // Test clear filters
    await page.locator('[data-testid="sidebar-filter-clear-button"]').click();
    await page.waitForTimeout(300);

    // Verify filters are cleared
    await expect(page.locator('[data-testid="filter-read-status-all"]')).toBeChecked();
    await expect(page.locator('[data-testid="filter-time-range-all"]')).toBeChecked();
  });

  test('should handle independent timeline scrolling (PROTECTED)', async ({ page }) => {
    const timeline = page.locator('[data-testid="desktop-timeline"]');

    // Verify timeline has scrollable content
    const scrollHeight = await timeline.evaluate(el => el.scrollHeight);
    const clientHeight = await timeline.evaluate(el => el.clientHeight);

    if (scrollHeight > clientHeight) {
      // Test scrolling functionality
      await timeline.hover();
      await page.mouse.wheel(0, 500);
      await page.waitForTimeout(300);

      const scrollTop = await timeline.evaluate(el => el.scrollTop);
      expect(scrollTop).toBeGreaterThan(0);
    }
  });

  test('should maintain visual consistency across themes (PROTECTED)', async ({ page }) => {
    // Check liquid-beige theme contrast (default)
    const filterLabel = page.locator('[data-testid="filter-read-status-label"]');
    const liquidBeigeColor = await filterLabel.evaluate(el => getComputedStyle(el).color);

    // Switch to vaporwave
    await page.locator('[data-testid="theme-toggle"]').click();
    await page.waitForTimeout(500);

    // Check vaporwave theme contrast
    const vaporwaveColor = await filterLabel.evaluate(el => getComputedStyle(el).color);

    // Colors should be different for proper contrast
    expect(liquidBeigeColor).not.toBe(vaporwaveColor);

    // Verify glass effects are present in both themes
    const sidebar = page.locator('[data-testid="desktop-sidebar-filters"]');
    const backdropFilter = await sidebar.evaluate(el => getComputedStyle(el).backdropFilter);
    expect(backdropFilter).toContain('blur');
  });

  test('should handle sidebar collapse functionality (PROTECTED)', async ({ page }) => {
    // Only test if collapse toggle exists
    const collapseToggle = page.locator('[data-testid="sidebar-collapse-toggle"]');

    if (await collapseToggle.count() > 0) {
      // Test sidebar collapse
      await collapseToggle.click();
      await page.waitForTimeout(300);

      // Test sidebar expand
      await collapseToggle.click();
      await page.waitForTimeout(300);

      // Verify sidebar is expanded
      await expect(page.locator('[data-testid="filter-read-status-label"]')).toBeVisible();
    }
  });
});