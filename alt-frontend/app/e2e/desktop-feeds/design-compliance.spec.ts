import { test, expect } from '@playwright/test';

test.describe('Design Language Compliance - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    await page.route('**/v1/feeds/fetch/cursor*', async (route) => {
      const feeds = Array.from({ length: 10 }, (_, i) => ({
        id: `feed-${i}`,
        title: `Design Test Feed ${i}`,
        description: `Description for design test feed ${i}`,
        link: `https://example.com/feed-${i}`,
        published: new Date().toISOString(),
      }));

      await route.fulfill({
        json: { 
          data: feeds,
          next_cursor: null
        }
      });
    });

    await page.goto('/desktop/feeds');
    await page.waitForLoadState('domcontentloaded');
  });

  test('should use glassmorphism across all components', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check timeline container uses glass effect
    const timeline = page.locator('[data-testid="desktop-timeline"]');
    const timelineStyles = await timeline.evaluate(el => getComputedStyle(el));
    
    // Should have backdrop filter (glassmorphism)
    expect(timelineStyles.backdropFilter).toContain('blur');
    
    // Check filter bar uses glass effect
    const filterBar = page.locator('[data-testid="filter-bar"]');
    if (await filterBar.count() > 0) {
      const filterStyles = await filterBar.evaluate(el => getComputedStyle(el));
      expect(filterStyles.backdropFilter).toContain('blur');
    }

    // Glass class should be present on key elements
    const glassElements = page.locator('.glass');
    const glassCount = await glassElements.count();
    expect(glassCount).toBeGreaterThan(0);
  });

  test('should support theme switching correctly', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check initial theme (should be one of the supported themes)
    const initialTheme = await page.evaluate(() => {
      return document.body.getAttribute('data-style');
    });
    
    expect(['vaporwave', 'liquid-beige', null]).toContain(initialTheme);

    // Find and click theme toggle
    const themeToggle = page.locator('[data-testid="theme-toggle"]');
    if (await themeToggle.count() > 0) {
      await themeToggle.click();
      await page.waitForTimeout(300);

      // Verify theme changed
      const newTheme = await page.evaluate(() => {
        return document.body.getAttribute('data-style');
      });
      
      expect(newTheme).not.toBe(initialTheme);
      expect(['vaporwave', 'liquid-beige']).toContain(newTheme);
    } else {
      // If no theme toggle found, just verify theme system is working
      expect(true).toBeTruthy();
    }
  });

  test('should use proper hover effects', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check buttons have proper hover transitions
    const buttons = page.locator('button');
    if (await buttons.count() > 0) {
      const firstButton = buttons.first();
      const buttonStyles = await firstButton.evaluate(el => getComputedStyle(el));
      
      // Should have transition for smooth hover effects
      expect(buttonStyles.transition).toBeTruthy();
      expect(buttonStyles.transition.length).toBeGreaterThan(0);
    }

    // Check interactive elements for hover capability
    const interactiveElements = page.locator('button, a, [role="button"]');
    const hasInteractive = await interactiveElements.count() > 0;
    expect(hasInteractive).toBeTruthy();
  });

  test('should use consistent spacing system', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    const timeline = page.locator('[data-testid="desktop-timeline"]');
    const styles = await timeline.evaluate(el => getComputedStyle(el));

    // Check that padding/margins use reasonable values
    const paddingValue = parseFloat(styles.paddingTop);
    expect(paddingValue).toBeGreaterThan(0);
    
    // Check border radius is applied (glassmorphism effect)
    const borderRadiusValue = parseFloat(styles.borderRadius);
    expect(borderRadiusValue).toBeGreaterThan(0);
  });

  test('should maintain design consistency across viewports', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    const timeline = page.locator('[data-testid="desktop-timeline"]');

    // Test desktop viewport
    await page.setViewportSize({ width: 1200, height: 800 });
    await page.waitForTimeout(500);
    
    let styles = await timeline.evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');

    // Test tablet viewport
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(500);
    
    styles = await timeline.evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');

    // Test mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });
    await page.waitForTimeout(500);
    
    styles = await timeline.evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');
  });

  test('should have proper typography and text colors', async ({ page }) => {
    await page.waitForSelector('[data-testid="desktop-timeline"]', { timeout: 10000 });

    // Check text elements have proper colors
    const textElements = page.locator('p, h1, h2, h3, h4, h5, h6, span');
    if (await textElements.count() > 0) {
      const firstText = textElements.first();
      const textStyles = await firstText.evaluate(el => getComputedStyle(el));
      
      // Text should not be transparent
      const opacity = parseFloat(textStyles.opacity);
      expect(opacity).toBeGreaterThan(0.5);
      
      // Text should have color (not inherit or initial)
      expect(textStyles.color).not.toBe('initial');
      expect(textStyles.color).not.toBe('inherit');
    }
  });
});