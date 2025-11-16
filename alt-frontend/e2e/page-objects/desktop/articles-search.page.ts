import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../base.page";

/**
 * Desktop Articles Search Page Object
 * Represents the /desktop/articles/search page
 */
export class DesktopArticlesSearchPage extends BasePage {
  // Locators
  readonly pageHeading: Locator;
  readonly searchInput: Locator;
  readonly searchButton: Locator;
  readonly resultsList: Locator;
  readonly filterPanel: Locator;
  readonly feedFilter: Locator;
  readonly dateFilter: Locator;
  readonly statusFilter: Locator;
  readonly sortSelect: Locator;
  readonly noResultsMessage: Locator;
  readonly resultCount: Locator;
  readonly clearButton: Locator;
  readonly sidebar: Locator;

  constructor(page: Page) {
    super(page);

    // Initialize locators
    this.pageHeading = page.getByRole("heading", { name: /search/i });
    this.searchInput = page
      .getByRole("searchbox")
      .or(page.getByPlaceholder(/search/i));
    this.searchButton = page.getByRole("button", { name: /search/i });
    this.resultsList = page.getByRole("list").filter({
      has: page.getByRole("article"),
    });
    this.filterPanel = page
      .getByRole("region", { name: /filter/i })
      .or(page.locator('[data-testid="filter-panel"]'));
    this.feedFilter = page.getByLabel(/feed|source/i);
    this.dateFilter = page.getByLabel(/date|time/i);
    this.statusFilter = page.getByLabel(/status/i);
    this.sortSelect = page.getByLabel(/sort|order/i);
    this.noResultsMessage = page.getByText(
      /no results|not found|no articles found/i,
    );
    this.resultCount = page.locator('[data-testid="result-count"]');
    this.clearButton = page.getByRole("button", { name: /clear|reset/i });
    this.sidebar = page.getByRole("navigation", { name: /sidebar/i });
  }

  /**
   * Navigate to search page
   */
  async goto(): Promise<void> {
    await this.page.goto("/desktop/articles/search");
    await this.waitForLoad();
  }

  /**
   * Wait for page to be fully loaded
   */
  async waitForLoad(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.searchInput).toBeVisible();
  }

  /**
   * Perform search
   */
  async search(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchButton.click();
    await this.waitForLoadingToComplete();
  }

  /**
   * Perform search with Enter key
   */
  async searchWithEnter(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchInput.press("Enter");
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
   * Get search results count
   */
  async getResultsCount(): Promise<number> {
    try {
      return await this.resultsList.getByRole("article").count();
    } catch {
      return 0;
    }
  }

  /**
   * Get result count text
   */
  async getResultCountText(): Promise<string | null> {
    if ((await this.resultCount.count()) > 0) {
      return await this.resultCount.textContent();
    }
    return null;
  }

  /**
   * Click on result by index
   */
  async clickResult(index: number): Promise<void> {
    const results = this.resultsList.getByRole("article");
    await results.nth(index).click();
  }

  /**
   * Click on result by title
   */
  async clickResultByTitle(title: string): Promise<void> {
    const result = this.resultsList
      .getByRole("article")
      .filter({ hasText: title });
    await result.click();
  }

  /**
   * Apply feed filter
   */
  async filterByFeed(feedName: string): Promise<void> {
    if ((await this.feedFilter.count()) > 0) {
      await this.feedFilter.selectOption(feedName);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Apply date filter
   */
  async filterByDate(date: string): Promise<void> {
    if ((await this.dateFilter.count()) > 0) {
      await this.dateFilter.fill(date);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Apply status filter
   */
  async filterByStatus(status: "read" | "unread" | "all"): Promise<void> {
    if ((await this.statusFilter.count()) > 0) {
      await this.statusFilter.selectOption(status);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Sort results
   */
  async sortBy(option: string): Promise<void> {
    if ((await this.sortSelect.count()) > 0) {
      await this.sortSelect.selectOption(option);
      await this.waitForLoadingToComplete();
    }
  }

  /**
   * Check if no results message is shown
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
   * Get all result titles
   */
  async getResultTitles(): Promise<string[]> {
    const results = this.resultsList.getByRole("article");
    const headings = results.getByRole("heading");
    return await headings.allTextContents();
  }

  /**
   * Check if filter panel is visible
   */
  async isFilterPanelVisible(): Promise<boolean> {
    try {
      await expect(this.filterPanel).toBeVisible({ timeout: 2000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Open filter panel (if collapsible)
   */
  async openFilterPanel(): Promise<void> {
    if (!(await this.isFilterPanelVisible())) {
      const filterToggle = this.page.getByRole("button", {
        name: /filter|show filters/i,
      });

      if ((await filterToggle.count()) > 0) {
        await filterToggle.click();
      }
    }
  }

  /**
   * Get current search query
   */
  async getCurrentQuery(): Promise<string> {
    return await this.searchInput.inputValue();
  }
}
