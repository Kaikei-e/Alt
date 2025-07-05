import { test, expect } from '@playwright/test';

test.describe('FilterBar Component - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    // Mock the test page for FilterBar
    await page.route('**/api/feeds*', async (route) => {
      await route.fulfill({
        json: { feeds: [], hasMore: false }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should render filter UI with glass styling (PROTECTED)', async ({ page }) => {
    // Check for the timeline FilterBar component
    const timelineFilterBar = page.locator('[data-testid="filter-bar"]');
    await expect(timelineFilterBar).toBeVisible();
    
    // Verify glass styling
    await expect(timelineFilterBar).toHaveClass(/glass/);
    
    // Check for the sidebar filter UI
    const sidebarFilters = page.locator('[data-testid="desktop-sidebar-filters"]');
    await expect(sidebarFilters).toBeVisible();
    
    // Verify basic filter sections are present
    await expect(page.locator('[data-testid="filter-read-status-label"]')).toContainText('Read Status');
    await expect(page.locator('[data-testid="filter-sources-label"]')).toContainText('Sources');
    await expect(page.locator('[data-testid="filter-time-range-label"]')).toContainText('Time Range');
  });

  test('should handle read status filter changes (PROTECTED)', async ({ page }) => {
    // Use the sidebar filter elements
    const allStatusFilter = page.locator('[data-testid="filter-read-status-all"]');
    const unreadStatusFilter = page.locator('[data-testid="filter-read-status-unread"]');
    
    // Verify initial state (all should be checked)
    await expect(allStatusFilter).toBeChecked();
    
    // Click to select unread
    await unreadStatusFilter.click();
    
    // Wait for state update and verify change
    await page.waitForTimeout(100);
    await expect(unreadStatusFilter).toBeChecked();
    await expect(allStatusFilter).not.toBeChecked();
    
    // Click to go back to all
    await allStatusFilter.click();
    await page.waitForTimeout(100);
    await expect(allStatusFilter).toBeChecked();
    await expect(unreadStatusFilter).not.toBeChecked();
  });

  test('should handle source filter changes (PROTECTED)', async ({ page }) => {
    // Check if any source filters are available
    const techcrunchFilter = page.locator('[data-testid="filter-source-techcrunch"]');
    
    // Only run this test if TechCrunch source is available
    if (await techcrunchFilter.isVisible().catch(() => false)) {
      // Verify initial state (should be unchecked)
      await expect(techcrunchFilter).not.toBeChecked();
      
      // Click to select TechCrunch
      await techcrunchFilter.click();
      
      // Wait for state update and verify change
      await page.waitForTimeout(100);
      await expect(techcrunchFilter).toBeChecked();
      
      // Click again to unselect
      await techcrunchFilter.click();
      await page.waitForTimeout(100);
      await expect(techcrunchFilter).not.toBeChecked();
    }
  });

  test('should handle time range filter changes (PROTECTED)', async ({ page }) => {
    // Use the sidebar time range filter elements
    const allTimeFilter = page.locator('[data-testid="filter-time-range-all"]');
    const todayTimeFilter = page.locator('[data-testid="filter-time-range-today"]');
    
    // Verify initial state (all should be checked)
    await expect(allTimeFilter).toBeChecked();
    
    // Click to select today
    await todayTimeFilter.click();
    
    // Wait for state update and verify change
    await page.waitForTimeout(100);
    await expect(todayTimeFilter).toBeChecked();
    await expect(allTimeFilter).not.toBeChecked();
    
    // Click to go back to all
    await allTimeFilter.click();
    await page.waitForTimeout(100);
    await expect(allTimeFilter).toBeChecked();
    await expect(todayTimeFilter).not.toBeChecked();
  });
});