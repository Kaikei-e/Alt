import { test, expect } from '@playwright/test';

test.describe('DesktopSidebar Component', () => {
  test.describe('Navigation Mode', () => {
    test.beforeEach(async ({ page }) => {
      // Navigate to the test page
      await page.goto('/test/desktop-sidebar');
      await page.waitForLoadState('domcontentloaded');
    });

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
      const activeItem = page.getByRole('link', { name: 'Dashboard' });
      await expect(activeItem).toHaveClass(/active/);
    });

    test('should have proper accessibility attributes', async ({ page }) => {
      const nav = page.getByRole('navigation');
      await expect(nav).toBeVisible();
      await expect(nav).toHaveAttribute('aria-label', 'Main navigation');

      // Check that navigation items are accessible
      const navItems = page.locator('nav a');
      const count = await navItems.count();
      expect(count).toBeGreaterThan(0);
    });

    test('should apply glassmorphism styling', async ({ page }) => {
      const sidebar = page.locator('[data-testid="desktop-sidebar"]');
      await expect(sidebar).toHaveClass(/glass/);
    });
  });

  test.describe('Feeds Filter Mode', () => {
    test.beforeEach(async ({ page }) => {
      // Navigate to the feeds page which uses filter mode
      await page.goto('/desktop/feeds');
      await page.waitForLoadState('domcontentloaded');
    });

    test('should display filters header and collapse toggle', async ({ page }) => {
      await expect(page.getByText('Filters')).toBeVisible();
      await expect(page.getByRole('button', { name: 'Collapse sidebar' })).toBeVisible();
    });

    test('should display read status filter options', async ({ page }) => {
      await expect(page.getByText('Read Status')).toBeVisible();
      await expect(page.getByLabel('all')).toBeVisible();
      await expect(page.getByLabel('unread')).toBeVisible();
      await expect(page.getByLabel('read')).toBeVisible();
    });

    test('should display feed sources with unread counts', async ({ page }) => {
      await expect(page.getByText('Sources')).toBeVisible();
      await expect(page.getByText('TechCrunch')).toBeVisible();
      await expect(page.getByText('12')).toBeVisible(); // unread count
      await expect(page.getByText('Hacker News')).toBeVisible();
      await expect(page.getByText('8')).toBeVisible(); // unread count
    });

    test('should display time range filter options', async ({ page }) => {
      await expect(page.getByText('Time Range')).toBeVisible();
      await expect(page.getByLabel('all')).toBeVisible();
      await expect(page.getByLabel('today')).toBeVisible();
      await expect(page.getByLabel('week')).toBeVisible();
      await expect(page.getByLabel('month')).toBeVisible();
    });

    test('should have clear filters button', async ({ page }) => {
      await expect(page.getByRole('button', { name: 'Clear Filters' })).toBeVisible();
    });

    test('should handle filter interactions', async ({ page }) => {
      // Test read status filter
      const unreadRadio = page.getByLabel('unread');
      await unreadRadio.check();
      await expect(unreadRadio).toBeChecked();

      // Test source filter
      const techcrunchCheckbox = page.getByLabel('TechCrunch');
      await techcrunchCheckbox.check();
      await expect(techcrunchCheckbox).toBeChecked();

      // Test time range filter
      const weekRadio = page.getByLabel('week');
      await weekRadio.check();
      await expect(weekRadio).toBeChecked();
    });

    test('should handle sidebar collapse', async ({ page }) => {
      const collapseButton = page.getByRole('button', { name: 'Collapse sidebar' });

      // Initially expanded
      await expect(page.getByText('Read Status')).toBeVisible();

      // Click to collapse
      await collapseButton.click();
      await expect(page.getByText('Read Status')).not.toBeVisible();

      // Click to expand again
      await collapseButton.click();
      await expect(page.getByText('Read Status')).toBeVisible();
    });

    test('should clear all filters when clear button is clicked', async ({ page }) => {
      // Set some filters first
      await page.getByLabel('unread').check();
      await page.getByLabel('TechCrunch').check();
      await page.getByLabel('week').check();

      // Click clear filters
      await page.getByRole('button', { name: 'Clear Filters' }).click();

      // Verify filters are reset
      await expect(page.getByLabel('all')).toBeChecked();
      await expect(page.getByLabel('TechCrunch')).not.toBeChecked();
    });

    test('should apply glassmorphism styling in filter mode', async ({ page }) => {
      const sidebar = page.locator('.glass');
      await expect(sidebar.first()).toBeVisible();
    });
  });

  test.describe('Responsive Behavior', () => {
    test('should be responsive on mobile devices', async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto('/desktop/feeds');
      await page.waitForLoadState('domcontentloaded');

      // Sidebar should still be functional
      await expect(page.getByText('Filters')).toBeVisible();
    });

    test('should handle overflow in sources list', async ({ page }) => {
      await page.goto('/desktop/feeds');
      await page.waitForLoadState('domcontentloaded');

      const sourcesContainer = page.locator('.max-h-40.overflow-y-auto');
      await expect(sourcesContainer).toBeVisible();
    });
  });

  test.describe('Accessibility', () => {
    test('should have proper ARIA labels and roles', async ({ page }) => {
      await page.goto('/desktop/feeds');
      await page.waitForLoadState('domcontentloaded');

      // Check collapse button has proper aria-label
      await expect(page.getByRole('button', { name: 'Collapse sidebar' })).toBeVisible();

      // Check form controls have proper labels
      await expect(page.getByLabel('all')).toBeVisible();
      await expect(page.getByLabel('unread')).toBeVisible();
    });

    test('should support keyboard navigation', async ({ page }) => {
      await page.goto('/desktop/feeds');
      await page.waitForLoadState('domcontentloaded');

      // Focus should be manageable
      await page.keyboard.press('Tab');
      await expect(page.locator(':focus')).toBeVisible();
    });
  });
});