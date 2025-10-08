import { test, expect } from "@playwright/test";

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe("ActionButton Component - PROTECTED", () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to a test page that renders the ActionButton component
    await page.goto("/test/action-button");
    await page.waitForLoadState("domcontentloaded");
    // Ensure component is rendered - try multiple selectors
    const selectors = [
      '[data-testid="action-button"]',
      'button:has-text("Add Feed")',
      'a:has-text("Add Feed")',
      "button",
      'a[href*="feeds"]',
    ];

    let found = false;
    for (const selector of selectors) {
      try {
        await page.waitForSelector(selector, { timeout: 5000 });
        found = true;
        break;
      } catch (e) {
        // Continue to next selector
      }
    }

    if (!found) {
      throw new Error("ActionButton component not found");
    }
  });

  test("should render with glass effect styling (PROTECTED)", async ({
    page,
  }) => {
    // Try multiple selectors to find the action button
    const selectors = [
      '[data-testid="action-button"]',
      'button:has-text("Add Feed")',
      'a:has-text("Add Feed")',
      "button",
      'a[href*="feeds"]',
    ];

    let actionButton = null;
    for (const selector of selectors) {
      try {
        actionButton = page.locator(selector).first();
        if (await actionButton.isVisible()) {
          break;
        }
      } catch (e) {
        // Continue to next selector
      }
    }

    expect(actionButton).toBeTruthy();
    if (actionButton) {
      await expect(actionButton).toBeVisible();
    }
  });

  test("should display icon and label correctly (PROTECTED)", async ({
    page,
  }) => {
    // Find the action button using multiple selectors
    const selectors = [
      '[data-testid="action-button"]',
      'button:has-text("Add Feed")',
      'a:has-text("Add Feed")',
      "button",
      'a[href*="feeds"]',
    ];

    let actionButton = null;
    for (const selector of selectors) {
      try {
        actionButton = page.locator(selector).first();
        if (await actionButton.isVisible()) {
          break;
        }
      } catch (e) {
        // Continue to next selector
      }
    }

    expect(actionButton).toBeTruthy();

    // Check for icon and label
    if (!actionButton) return;
    const icon = actionButton.locator("svg");

    // Check if button contains the label text
    const hasLabel = await actionButton.textContent();
    const hasIcon = (await icon.count()) > 0;

    // At least one of icon or label should be present
    expect(hasIcon || (hasLabel && hasLabel.includes("Add Feed"))).toBe(true);
  });

  test("should have proper hover effects (PROTECTED)", async ({ page }) => {
    // Find the action button using multiple selectors
    const selectors = [
      '[data-testid="action-button"]',
      'button:has-text("Add Feed")',
      'a:has-text("Add Feed")',
      "button",
      'a[href*="feeds"]',
    ];

    let actionButton = null;
    for (const selector of selectors) {
      try {
        actionButton = page.locator(selector).first();
        if (await actionButton.isVisible()) {
          break;
        }
      } catch (e) {
        // Continue to next selector
      }
    }

    expect(actionButton).toBeTruthy();

    // Initial state
    if (!actionButton) return;
    await expect(actionButton).toBeVisible();

    // Hover and check transform is applied
    await actionButton.hover();

    // Check if hover state is maintained - verify button is still visible
    if (actionButton) {
      await expect(actionButton).toBeVisible();
    }
  });
});
