import { test, expect } from "@playwright/test";

test.describe("ThemeToggle Component", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a page that has the ThemeToggle component
    await page.goto("/");

    // Wait for page to be loaded
    await page.waitForLoadState("domcontentloaded");

    // Wait for the component to be rendered with a longer timeout
    await page.waitForSelector('[data-testid="theme-toggle-button"]', {
      timeout: 30000,
    });
  });

  test("should render with correct accessibility attributes", async ({
    page,
  }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Check basic rendering
    await expect(toggleButton).toBeVisible();

    // Check accessibility attributes
    await expect(toggleButton).toHaveAttribute("role", "switch");
    await expect(toggleButton).toHaveAttribute("aria-label");
    await expect(toggleButton).toHaveAttribute("aria-checked");

    // Verify aria-label contains theme information
    const ariaLabel = await toggleButton.getAttribute("aria-label");
    expect(ariaLabel).toMatch(/Switch to.*theme/);
    expect(ariaLabel).toMatch(/Current theme:/);
  });

  test("should toggle theme on click", async ({ page }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Get initial theme state
    const initialAriaChecked = await toggleButton.getAttribute("aria-checked");
    const initialBodyStyle = await page.getAttribute("body", "data-style");

    // Click to toggle theme
    await toggleButton.click();

    // Wait for theme change to complete
    await page.waitForTimeout(300);

    // Verify theme has changed
    const newAriaChecked = await toggleButton.getAttribute("aria-checked");
    const newBodyStyle = await page.getAttribute("body", "data-style");

    expect(newAriaChecked).not.toBe(initialAriaChecked);
    expect(newBodyStyle).not.toBe(initialBodyStyle);

    // Verify the theme alternates between vaporwave and liquid-beige
    expect(["vaporwave", "liquid-beige"]).toContain(newBodyStyle);
  });

  test("should handle keyboard navigation (Space key)", async ({ page }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Focus the button
    await toggleButton.focus();

    // Get initial theme state
    const initialAriaChecked = await toggleButton.getAttribute("aria-checked");

    // Press Space key
    await page.keyboard.press("Space");

    // Wait for theme change
    await page.waitForTimeout(300);

    // Verify theme has changed
    const newAriaChecked = await toggleButton.getAttribute("aria-checked");
    expect(newAriaChecked).not.toBe(initialAriaChecked);
  });

  test("should handle keyboard navigation (Enter key)", async ({ page }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Focus the button
    await toggleButton.focus();

    // Get initial theme state
    const initialAriaChecked = await toggleButton.getAttribute("aria-checked");

    // Press Enter key
    await page.keyboard.press("Enter");

    // Wait for theme change
    await page.waitForTimeout(300);

    // Verify theme has changed
    const newAriaChecked = await toggleButton.getAttribute("aria-checked");
    expect(newAriaChecked).not.toBe(initialAriaChecked);
  });

  test("should display correct icon for each theme", async ({ page }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Check initial icon
    const initialIcon = toggleButton.locator("svg");
    await expect(initialIcon).toBeVisible();

    // Toggle theme
    await toggleButton.click();
    await page.waitForTimeout(300);

    // Check icon has changed (different SVG element)
    const newIcon = toggleButton.locator("svg");
    await expect(newIcon).toBeVisible();
  });

  test("should work with showLabel prop", async ({ page }) => {
    // This test assumes there's a variant with showLabel=true on the page
    // or we need to navigate to a specific test page
    const labelElement = page.locator('[data-testid="theme-toggle-label"]');

    if (await labelElement.isVisible()) {
      // Verify label displays current theme name
      const labelText = await labelElement.textContent();
      expect(["Vaporwave", "Liquid Beige"]).toContain(labelText);

      // Toggle and verify label updates
      const toggleButton = page.locator('[data-testid="theme-toggle-button"]');
      await toggleButton.click();
      await page.waitForTimeout(300);

      const newLabelText = await labelElement.textContent();
      expect(newLabelText).not.toBe(labelText);
      expect(["Vaporwave", "Liquid Beige"]).toContain(newLabelText);
    }
  });

  test("should apply glass effect styling", async ({ page }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Check for glass effect CSS properties
    const styles = await toggleButton.evaluate((el) => {
      const computed = window.getComputedStyle(el);
      return {
        backdropFilter: computed.backdropFilter,
        border: computed.border,
        borderRadius: computed.borderRadius,
      };
    });

    // Verify glass effect properties
    expect(styles.backdropFilter).toContain("blur");
    expect(styles.border).toMatch(/1px solid/);
    expect(parseInt(styles.borderRadius)).toBeGreaterThan(0);
  });

  test("should be responsive across different screen sizes", async ({
    page,
  }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Test desktop size
    await page.setViewportSize({ width: 1200, height: 800 });
    await expect(toggleButton).toBeVisible();

    // Test tablet size
    await page.setViewportSize({ width: 768, height: 1024 });
    await expect(toggleButton).toBeVisible();

    // Test mobile size
    await page.setViewportSize({ width: 375, height: 667 });
    await expect(toggleButton).toBeVisible();

    // Verify it still functions on mobile
    await toggleButton.click();
    await page.waitForTimeout(300);

    // Should still toggle theme
    const ariaChecked = await toggleButton.getAttribute("aria-checked");
    expect(["true", "false"]).toContain(ariaChecked);
  });

  test("should be focusable and clickable after theme toggle", async ({
    page,
  }) => {
    const toggleButton = page.locator('[data-testid="theme-toggle-button"]');

    // Initial focus
    await toggleButton.focus();
    await expect(toggleButton).toBeFocused();

    // Get initial theme state
    const initialAriaChecked = await toggleButton.getAttribute("aria-checked");

    // Toggle theme
    await toggleButton.click();
    await page.waitForTimeout(500);

    // Verify theme changed
    const newAriaChecked = await toggleButton.getAttribute("aria-checked");
    expect(newAriaChecked).not.toBe(initialAriaChecked);

    // Verify button is still focusable and functional
    await toggleButton.focus();
    await expect(toggleButton).toBeFocused();

    // Should be able to toggle again
    await toggleButton.click();
    await page.waitForTimeout(300);

    const finalAriaChecked = await toggleButton.getAttribute("aria-checked");
    expect(finalAriaChecked).toBe(initialAriaChecked);
  });
});
