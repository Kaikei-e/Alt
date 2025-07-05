import { test, expect } from '@playwright/test';

test.describe('DesktopSidebar Component', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/test/desktop-sidebar');
    await page.waitForSelector('[data-testid="desktop-sidebar"]', { timeout: 5000 });
  });

  test.describe('Navigation Mode', () => {
    test('should render logo and navigation items correctly', async ({ page }) => {
      // Check logo text and subtext
      await expect(page.getByText('Alt RSS')).toBeVisible();
      await expect(page.getByText('Feed Reader')).toBeVisible();

      // Check navigation items
      await expect(page.getByRole('link', { name: 'Dashboard' })).toBeVisible();
      await expect(page.getByRole('link', { name: 'Feeds' })).toBeVisible();
      await expect(page.getByRole('link', { name: 'Statistics' })).toBeVisible();
      await expect(page.getByRole('link', { name: 'Settings' })).toBeVisible();
    });

    test('should highlight active navigation item', async ({ page }) => {
      // Ensure we're in navigation mode
      await page.getByText('Navigation').click();
      await page.waitForTimeout(500);

      // Check if the active item has the correct styling or background
      const activeLink = page.getByRole('link', { name: 'Dashboard' });
      await expect(activeLink).toBeVisible();

      // Check for active styling - look for the Flex container within the link
      const activeContainer = activeLink.locator('..');
      const hasActiveClass = await activeContainer.locator('.active').count() > 0;
      const hasActiveBackground = await activeLink.evaluate(el => {
        const computed = getComputedStyle(el);
        return computed.backgroundColor !== 'rgba(0, 0, 0, 0)' && computed.backgroundColor !== 'transparent';
      });

      // Either active class or active background should be present
      expect(hasActiveClass || hasActiveBackground).toBeTruthy();
    });

    test('should have proper accessibility attributes', async ({ page }) => {
      await page.getByText('Navigation').click();
      await page.waitForTimeout(500);

      const nav = page.getByRole('navigation');
      await expect(nav).toHaveAttribute('aria-label', 'Main navigation');
    });

    test('should apply glassmorphism styling', async ({ page }) => {
      await page.getByText('Navigation').click();
      await page.waitForTimeout(500);

      const sidebar = page.locator('[data-testid="desktop-sidebar"]').first();
      await expect(sidebar).toHaveClass(/glass/);
    });
  });

  test.describe('Feeds Filter Mode', () => {
    test.beforeEach(async ({ page }) => {
      // Switch to feeds filter mode using the correct button
      const filterButton = page.getByRole('button', { name: 'Feeds Filter' });
      await filterButton.click();
      await page.waitForTimeout(500);
    });

    test('should display filters header and collapse toggle', async ({ page }) => {
      await expect(page.getByTestId('filter-header-title')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Collapse sidebar' })).toBeVisible();
    });

      test('should display read status filter options', async ({ page }) => {
    await expect(page.getByText('Read Status')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-read-status-all')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-read-status-unread')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-read-status-read')).toBeVisible();
  });

    test('should display feed sources with unread counts', async ({ page }) => {
      await expect(page.getByText('Sources')).toBeVisible();
      await expect(page.getByText('TechCrunch')).toBeVisible();
      await expect(page.getByText('12').first()).toBeVisible(); // unread count
      await expect(page.getByText('Hacker News')).toBeVisible();
      await expect(page.getByText('8').first()).toBeVisible(); // unread count
    });

      test('should display time range filter options', async ({ page }) => {
    await expect(page.getByText('Time Range')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-time-range-all')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-time-range-today')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-time-range-week')).toBeVisible();
    await expect(page.getByTestId('sidebar-filter-time-range-month')).toBeVisible();
  });

    test('should handle filter interactions', async ({ page }) => {
      // Test read status filter
      await page.getByLabel('unread').click();
      await expect(page.getByLabel('unread')).toBeChecked();

      // Test time range filter
      await page.getByLabel('week').click();
      await expect(page.getByLabel('week')).toBeChecked();
    });

    test('should clear all filters when clear button is clicked', async ({ page }) => {
      // Select some filters first
      await page.getByLabel('unread').click();
      await page.getByLabel('week').click();

      // Clear filters
      await page.getByRole('button', { name: 'Clear Filters' }).click();

      // Verify filters are reset
      await expect(page.getByLabel('all').first()).toBeChecked();
    });

    test('should handle sidebar collapse', async ({ page }) => {
      const collapseButton = page.getByRole('button', { name: 'Collapse sidebar' });
      await collapseButton.click();

      // Check if filter content is hidden
      await expect(page.getByText('Read Status')).not.toBeVisible();
    });

    test('should apply glassmorphism styling in filter mode', async ({ page }) => {
      const sidebar = page.locator('[data-testid="desktop-sidebar-filters"]');
      await expect(sidebar).toHaveClass(/glass/);
    });
  });

  test.describe('Responsive Behavior', () => {
    test.beforeEach(async ({ page }) => {
      // Switch to feeds filter mode
      const filterButton = page.getByRole('button', { name: 'Feeds Filter' });
      await filterButton.click();
      await page.waitForTimeout(500);
    });

    test('should be responsive on mobile devices', async ({ page }) => {
      // Simulate mobile viewport
      await page.setViewportSize({ width: 375, height: 667 });
      await page.waitForTimeout(500);

      // Sidebar should still be functional
      await expect(page.getByTestId('filter-header-title')).toBeVisible();
    });

    test('should handle overflow in sources list', async ({ page }) => {
      const sourcesList = page.locator('[data-testid="filter-sources-label"]').locator('..');
      await expect(sourcesList).toBeVisible();
    });
  });

  test.describe('Accessibility', () => {
    test.beforeEach(async ({ page }) => {
      // Switch to feeds filter mode
      const filterButton = page.getByRole('button', { name: 'Feeds Filter' });
      await filterButton.click();
      await page.waitForTimeout(500);
    });

    test('should have proper ARIA labels and roles', async ({ page }) => {
      const collapseButton = page.getByRole('button', { name: 'Collapse sidebar' });
      await expect(collapseButton).toHaveAttribute('aria-label', 'Collapse sidebar');

      const clearButton = page.getByRole('button', { name: 'Clear Filters' });
      await expect(clearButton).toBeVisible();
    });
  });
});