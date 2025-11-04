import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Mobile Feed Search Page Object
 * Represents the /mobile/feeds/search page
 */
export class MobileFeedsSearchPage extends BasePage {
  // Locators
  readonly searchInput: Locator;
  readonly searchButton: Locator;
  readonly backButton: Locator;
  readonly clearButton: Locator;
  readonly resultsList: Locator;
  readonly noResultsMessage: Locator;
  readonly loadingIndicator: Locator;
  readonly filterButton: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.searchInput = page.getByRole("searchbox").or(page.getByPlaceholder(/search/i));
    this.searchButton = page.getByRole("button", { name: /search/i });
    this.backButton = page.getByRole("button", { name: /back/i });
    this.clearButton = page.getByRole("button", { name: /clear/i });
    this.resultsList = page.getByRole("list").filter({
      has: page.getByRole("article"),
    });
    this.noResultsMessage = page.getByText(/no results|not found/i);
    this.loadingIndicator = page.getByRole("status", { name: /loading/i });
    this.filterButton = page.getByRole("button", { name: /filter/i });
  }

  /**
   * Navigate to search page
   */
  async goto(): Promise<void> {
    await this.page.goto("/mobile/feeds/search");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.searchInput).toBeVisible();
  }

  /**
   * Perform search
   */
  async search(query: string): Promise<void> {
    await this.searchInput.fill(query);

    if ((await this.searchButton.count()) > 0) {
      await this.searchButton.click();
    } else {
      await this.searchInput.press("Enter");
    }

    await this.waitForLoadingToComplete();
  }

  /**
   * Clear search
   */
  async clearSearch(): Promise<void> {
    if ((await this.clearButton.count()) > 0) {
      await this.clearButton.click();
    } else {
      await this.searchInput.clear();
    }
  }

  /**
   * Go back
   */
  async goBack(): Promise<void> {
    await this.backButton.click();
    await this.page.waitForURL(/\/mobile\/feeds$/);
  }

  /**
   * Get results count
   */
  async getResultsCount(): Promise<number> {
    try {
      return await this.resultsList.getByRole("article").count();
    } catch {
      return 0;
    }
  }

  /**
   * Tap on result
   */
  async tapResult(index: number): Promise<void> {
    const results = this.resultsList.getByRole("article");
    await results.nth(index).click();
  }

  /**
   * Check if no results
   */
  async hasNoResults(): Promise<boolean> {
    try {
      await expect(this.noResultsMessage).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Open filter
   */
  async openFilter(): Promise<void> {
    if ((await this.filterButton.count()) > 0) {
      await this.filterButton.click();
    }
  }
}
