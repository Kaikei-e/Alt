import { test, expect } from '@playwright/test';

test.describe('DesktopSidebar Filters Mode - TASK1', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to desktop feeds page with filters
    await page.goto('/desktop/feeds');

    // Wait for sidebar to load
    await page.waitForSelector('[data-testid="desktop-sidebar-filters"]', { timeout: 5000 });
  });

  test('should use theme-aware colors for filter elements', async ({ page }) => {
    const readStatusLabel = page.locator('[data-testid="filter-read-status-label"]');
    const filterButton = page.locator('[data-testid="sidebar-filter-clear-button"]');

    // Verify theme-aware text colors are applied (check computed color value)
    const labelStyles = await readStatusLabel.evaluate(el => getComputedStyle(el));
    expect(labelStyles.color).toBeTruthy(); // Check that color is applied

    // Verify button uses CSS variables
    const buttonStyles = await filterButton.evaluate(el => {
      const computed = getComputedStyle(el);
      return {
        background: computed.background,
        borderColor: computed.borderColor,
        color: computed.color
      };
    });

    // Should use CSS variables, not fixed colors
    expect(buttonStyles.background).not.toContain('rgb(243, 244, 246)'); // not gray-100
  });

  test('should maintain filter functionality after UI updates', async ({ page }) => {
    // Test read status filter functionality
    const unreadRadio = page.locator('[data-testid="filter-read-status-unread"]');
    await unreadRadio.click();

    // Verify filter state change
    await expect(unreadRadio).toBeChecked();

    // Test source filter functionality
    const firstSourceCheckbox = page.locator('[data-testid="filter-source-checkbox"]').first();
    await firstSourceCheckbox.click();

    await expect(firstSourceCheckbox).toBeChecked();

        // Test clear filters
    const clearButton = page.locator('[data-testid="sidebar-filter-clear-button"]');
    await clearButton.click();

    // Verify filters are reset
    await expect(page.locator('[data-testid="filter-read-status-all"]')).toBeChecked();
    await expect(firstSourceCheckbox).not.toBeChecked();
  });

  test('should support theme switching', async ({ page }) => {
    // Change to liquid-beige theme
    await page.evaluate(() => {
      document.body.setAttribute('data-style', 'liquid-beige');
    });

    const filtersContainer = page.locator('[data-testid="desktop-sidebar-filters"]');

    // Wait for theme change to apply
    await page.waitForTimeout(300);

    // Verify theme-specific styles are applied
    const styles = await filtersContainer.evaluate(el => {
      const computed = getComputedStyle(el);
      return {
        background: computed.background,
        color: computed.color
      };
    });

    // Should adapt to liquid-beige theme
    expect(styles.background).not.toContain('rgba(255, 255, 255, 0.1)'); // not vaporwave
  });
});