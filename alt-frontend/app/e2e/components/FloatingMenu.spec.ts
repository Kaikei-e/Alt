import { test, expect } from "@playwright/test";
import { Feed, BackendFeedItem } from "@/schema/feed";

// Helper function to generate mock feed data
const generateMockFeeds = (count: number, startId: number = 1): Feed[] => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Feed ${startId + index}`,
    description: `This is test feed description ${startId + index}`,
    link: `https://example.com/feed/${startId + index}`,
    published: new Date().toISOString(),
  }));
};

const generateMockArticles = (count: number, startId: number = 1) => {
  return Array.from({ length: count }, (_, index) => ({
    id: `${startId + index}`,
    title: `Test Article ${startId + index}`,
    content: `Content for test article ${startId + index}. This is a longer content to test how the UI handles different text lengths.`,
  }));
};

test.describe("FloatingMenu Component - Refined Design Tests", () => {
  const menuItems = [
    {
      label: "View Feeds",
      href: "/mobile/feeds",
      category: "feeds",
    },
    {
      label: "Read Feeds",
      href: "/mobile/feeds/read",
      category: "feeds",
    },
    {
      label: "Register Feed",
      href: "/mobile/feeds/register",
      category: "feeds",
    },
    {
      label: "Search Feeds",
      href: "/mobile/feeds/search",
      category: "feeds",
    },
    {
      label: "Search Articles",
      href: "/mobile/articles/search",
      category: "articles",
    },
    {
      label: "View Stats",
      href: "/mobile/feeds/stats",
      category: "other",
    },
    {
      label: "Home",
      href: "/",
      category: "other",
    },
  ];

  test.beforeEach(async ({ page }) => {
    const mockFeeds = generateMockFeeds(10, 1);

    // Convert Feed[] to BackendFeedItem[] for API compatibility
    const backendFeeds: BackendFeedItem[] = mockFeeds.map((feed) => ({
      title: feed.title,
      description: feed.description,
      link: feed.link,
      published: feed.published,
    }));

    // Mock the feeds API endpoints to prevent dependency on backend
    await page.route("**/api/v1/feeds/fetch/page/0", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/feeds/fetch/cursor**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: backendFeeds,
          next_cursor: null,
        }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/list", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(backendFeeds),
      });
    });

    await page.route("**/api/v1/feeds/read", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ message: "Feed marked as read" }),
      });
    });

    await page.route("**/api/v1/feeds/fetch/details", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          feed_url: "https://example.com/feed/1",
          summary: "Test summary for this feed",
        }),
      });
    });

    await page.route("**/api/v1/articles/search", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(generateMockArticles(10, 1)),
      });
    });

    await page.route("**/api/v1/health", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ status: "ok" }),
      });
    });

    // Navigate to the page that has the FloatingMenu component
    await page.goto("/mobile/feeds");

    // Wait for the page to load and become stable
    await page.waitForLoadState("networkidle");

    // Wait for feeds to load first, then floating menu
    await expect(page.locator('[data-testid="feed-card"]').first()).toBeVisible(
      { timeout: 15000 },
    );

    // Now wait for the FloatingMenu
    await page.waitForSelector('[data-testid="floating-menu-button"]', {
      timeout: 10000,
    });
  });

  test.describe("Initial State", () => {
    test("should render compact floating menu trigger", async ({ page }) => {
      const menuTrigger = page.getByTestId("floating-menu-button");
      await expect(menuTrigger).toBeVisible();
      await expect(menuTrigger.locator("xpath=..")).toHaveCSS(
        "position",
        "fixed",
      );
    });

    test("should have elegant trigger styling", async ({ page }) => {
      const menuTrigger = page.getByTestId("floating-menu-button");
      // Check for proper border radius (should be circular/rounded)
      await expect(menuTrigger).toHaveCSS(
        "border-radius",
        /\d+px/,
      );
    });

    test("should not show menu content initially", async ({ page }) => {
      await expect(page.getByTestId("bottom-sheet-menu")).not.toBeVisible();
    });
  });

  test.describe("Menu Opening", () => {
    test("should open bottom sheet menu on trigger click", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("bottom-sheet-menu")).toBeVisible();
    });

    test("should show backdrop overlay", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("modal-backdrop")).toBeVisible();
    });
  });

  test.describe("Accordion Menu Items Display", () => {
    test("should display all menu items in accordion layout", async ({
      page,
    }) => {
      await page.getByTestId("floating-menu-button").click();

      // Feeds category should be open by default
      const feedsAccordionButton = page.getByTestId("tab-feeds");
      await expect(feedsAccordionButton).toBeVisible();
      await expect(feedsAccordionButton).toHaveAttribute("aria-expanded", "true");

      const feedItems = menuItems.filter(item => item.category === "feeds");
      for (const item of feedItems) {
        await expect(
          page.getByTestId("bottom-sheet-menu").getByText(item.label),
        ).toBeVisible();
      }

      // Check articles items by clicking accordion
      const articlesAccordionButton = page.getByTestId("tab-articles");
      await articlesAccordionButton.click();
      await expect(articlesAccordionButton).toHaveAttribute("aria-expanded", "true");

      const articlesItems = menuItems.filter(item => item.category === "articles");
      for (const item of articlesItems) {
        await expect(
          page.getByTestId("bottom-sheet-menu").getByText(item.label),
        ).toBeVisible();
      }
      // Feeds should now be closed
      await expect(feedsAccordionButton).toHaveAttribute("aria-expanded", "false");

      // Switch to other tab and check other items
      const otherAccordionButton = page.getByTestId("tab-other");
      await otherAccordionButton.click();
      await expect(otherAccordionButton).toHaveAttribute("aria-expanded", "true");

      const otherItems = menuItems.filter(item => item.category === "other");
      for (const item of otherItems) {
        await expect(
          page.getByTestId("bottom-sheet-menu").getByText(item.label),
        ).toBeVisible();
      }
    });

    test("should have correct navigation links", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();

      // We need to open all accordions to check links
      await page.getByTestId("tab-articles").click();
      await page.getByTestId("tab-other").click();

      for (const item of menuItems) {
        // Open the correct accordion for this item
        if (item.category === "feeds") {
          await page.getByTestId("tab-feeds").click();
        } else if (item.category === "articles") {
          await page.getByTestId("tab-articles").click();
        } else if (item.category === "other") {
          await page.getByTestId("tab-other").click();
        }
        const linkElement = page
          .getByTestId("bottom-sheet-menu")
          .getByRole("link")
          .filter({ hasText: item.label });
        await expect(linkElement).toHaveAttribute("href", item.href);
      }
    });

    test("should display close control", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      const closeControl = page.getByTestId("close-menu-button");
      await expect(closeControl).toBeVisible();
    });

    test("should have bottom sheet dimensions", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      const bottomSheet = page.getByTestId("bottom-sheet-menu");
      await expect(bottomSheet).toBeVisible({ timeout: 10000 });

      // Check positioning - should be on the bottom
      await expect(bottomSheet.locator("xpath=..")).toHaveCSS("position", "fixed");
      await expect(bottomSheet.locator("xpath=..")).toHaveCSS("bottom", "0px");

      const boundingBox = await bottomSheet.boundingBox();
      const viewport = page.viewportSize();

      if (boundingBox && viewport) {
        // Allow 5px tolerance for width
        expect(boundingBox.width).toBeGreaterThan(viewport.width - 5);
        expect(boundingBox.width).toBeLessThan(viewport.width + 5);
        // Height is content-dependent, but should be less than viewport height
        expect(boundingBox.height).toBeLessThan(viewport.height);
      }
    });
  });

  test.describe("Menu Interactions", () => {
    test("should close menu via close control", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("bottom-sheet-menu")).toBeVisible();
      await page.getByTestId("close-menu-button").click();
      await expect(page.getByTestId("bottom-sheet-menu")).not.toBeVisible();
      await expect(page.getByTestId("floating-menu-button")).toBeVisible();
    });

    test("should close menu when clicking backdrop", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("bottom-sheet-menu")).toBeVisible();
      await page
        .getByTestId("modal-backdrop")
        .click({ position: { x: 50, y: 50 }, force: true });
      await expect(page.getByTestId("bottom-sheet-menu")).not.toBeVisible();
      await expect(page.getByTestId("floating-menu-button")).toBeVisible();
    });
  });

  test.describe("Responsive Design", () => {
    test("should maintain bottom sheet design on mobile", async ({ page }) => {
      await page.setViewportSize({ width: 375, height: 667 });
      await page.goto("/mobile/feeds");
      await page.waitForLoadState("networkidle");
      await expect(
        page.locator('[data-testid="feed-card"]').first(),
      ).toBeVisible({ timeout: 15000 });
      await expect(page.getByTestId("floating-menu-button")).toBeVisible();
      await page.getByTestId("floating-menu-button").click();
      const bottomSheet = page.getByTestId("bottom-sheet-menu");
      await expect(bottomSheet).toBeVisible({ timeout: 10000 });
      const boundingBox = await bottomSheet.boundingBox();
      if (boundingBox) {
        expect(boundingBox.width).toBe(375);
      }
    });
  });

  test.describe("Home Menu Item Tests", () => {
    test("should display Home menu item in Other accordion", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();

      await page.getByTestId("tab-other").click();

      const homeLink = page
        .getByTestId("bottom-sheet-menu")
        .getByRole("link")
        .filter({ hasText: "Home" });

      await expect(homeLink).toBeVisible();
      await expect(homeLink).toHaveAttribute("href", "/");
    });
  });

  test.describe("Accessibility", () => {
    test("should have proper ARIA attributes", async ({ page }) => {
      const menuTrigger = page.getByTestId("floating-menu-button");
      await menuTrigger.focus();
      await expect(menuTrigger).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(page.getByTestId("bottom-sheet-menu")).toBeVisible();
    });

    test("should support keyboard navigation", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      // Focus should go to the first accordion button
      await expect(page.getByTestId("tab-feeds")).toBeFocused();

      // Tab to the first link inside
      await page.keyboard.press("Tab");
      const firstLink = page
        .getByRole("link")
        .filter({ hasText: menuItems[0].label });
      await expect(firstLink).toBeFocused();
    });

    test("should close menu with Escape key", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("bottom-sheet-menu")).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(page.getByTestId("bottom-sheet-menu")).not.toBeVisible();
      await expect(page.getByTestId("floating-menu-button")).toBeVisible();
    });
  });

  test.describe("Bottom-Sheet Menu Design", () => {
    test("should slide in from bottom", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      const bottomSheet = page.getByTestId("bottom-sheet-menu");
      await expect(bottomSheet).toBeVisible();

      // Check positioning - should be on the bottom
      await expect(bottomSheet.locator("xpath=..")).toHaveCSS("position", "fixed");
      await expect(bottomSheet.locator("xpath=..")).toHaveCSS("bottom", "0px");

      const boundingBox = await bottomSheet.boundingBox();
      const viewport = page.viewportSize();
      if (boundingBox && viewport) {
        expect(boundingBox.width).toBe(viewport.width);
      }
    });

    test("should show accordion navigation", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      const accordionContainer = page.locator(".chakra-accordion");
      await expect(accordionContainer).toBeVisible();

      await expect(page.getByTestId("tab-feeds")).toBeVisible();
      await expect(page.getByTestId("tab-articles")).toBeVisible();
      await expect(page.getByTestId("tab-other")).toBeVisible();
    });

    test("should show feeds items when feeds accordion is open", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      // Feeds is open by default
      const feedItems = menuItems.filter(item => item.category === "feeds");
      for (const item of feedItems) {
        await expect(page.getByTestId("bottom-sheet-menu").getByText(item.label)).toBeVisible();
      }

      const otherItems = menuItems.filter(item => item.category === "other");
      for (const item of otherItems) {
        await expect(page.getByTestId("bottom-sheet-menu").getByText(item.label)).not.toBeVisible();
      }
    });

    test("should show other items when other accordion is active", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();

      // Click other accordion
      await page.getByTestId("tab-other").click();
      await expect(page.getByTestId("tab-other")).toHaveAttribute("aria-expanded", "true");

      const otherItems = menuItems.filter(item => item.category === "other");
      for (const item of otherItems) {
        await expect(page.getByTestId("bottom-sheet-menu").getByText(item.label)).toBeVisible();
      }

      const feedItems = menuItems.filter(item => item.category === "feeds");
      for (const item of feedItems) {
        await expect(page.getByTestId("bottom-sheet-menu").getByText(item.label)).not.toBeVisible();
      }
    });

    test("should have slide-in from bottom animation when opening", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      const bottomSheet = page.getByTestId("bottom-sheet-menu");
      await expect(bottomSheet).toBeVisible();
      // Check transform or transition on the parent (Drawer.Positioner)
      const positioner = bottomSheet.locator('xpath=..');
      const transform = await positioner.evaluate(el => getComputedStyle(el).transform);
      const transition = await positioner.evaluate(el => getComputedStyle(el).transitionProperty);
      expect(transform === "none" ? transition : transform).not.toBe("none");
    });

    test("should close menu when clicking backdrop", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();
      await expect(page.getByTestId("bottom-sheet-menu")).toBeVisible();
      await page.getByTestId("modal-backdrop").click({ position: { x: 50, y: 50 }, force: true });
      await expect(page.getByTestId("bottom-sheet-menu")).not.toBeVisible();
      await expect(page.getByTestId("floating-menu-button")).toBeVisible();
    });

    test("should navigate correctly when menu item is clicked", async ({ page }) => {
      await page.getByTestId("floating-menu-button").click();

      const viewFeedsItem = page.getByTestId("bottom-sheet-menu").getByText("View Feeds");
      await viewFeedsItem.click();

      await expect(page).toHaveURL("/mobile/feeds");
      await expect(page.getByTestId("bottom-sheet-menu")).not.toBeVisible();
    });
  });
});
