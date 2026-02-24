import { expect, type Locator, type Page } from "@playwright/test";
import { BasePage } from "../BasePage";

/**
 * Desktop Search Page Object
 * Uses user-facing selectors for better test stability
 */
export class SearchPage extends BasePage {
  readonly searchInput: Locator;
  readonly searchButton: Locator;
  readonly searchResults: Locator;
  readonly emptyState: Locator;
  readonly heading: Locator;
  readonly searchForm: Locator;
  readonly resultItems: Locator;

  constructor(page: Page) {
    super(page);
    // User-facing locators (following Playwright best practices)
    this.searchInput = page
      .getByPlaceholder(/search/i)
      .or(page.getByRole("searchbox"))
      .or(page.getByTestId("search-input"));
    this.searchButton = page
      .getByRole("button", { name: /search/i })
      .or(page.getByTestId("search-button"));
    this.searchResults = page.getByTestId("search-results");
    this.resultItems = page
      .getByTestId("search-result")
      .or(page.locator("article"))
      .or(page.locator('[class*="result"]'));
    this.emptyState = page
      .getByText(/no results|not found|見つかりません/i)
      .or(page.getByTestId("search-empty-state"));
    this.heading = page
      .getByRole("heading", { name: /search/i })
      .or(page.getByTestId("search-heading"));
    this.searchForm = page.locator("form");
  }

  /**
   * Navigate to search page
   */
  async goto(): Promise<void> {
    await this.navigateTo("/desktop/articles/search");
  }

  /**
   * Wait for page to be ready
   */
  async waitForReady(timeout = 10000): Promise<void> {
    await expect(this.searchInput).toBeVisible({ timeout });
  }

  /**
   * Perform a search
   */
  async search(query: string): Promise<void> {
    await expect(this.searchInput).toBeVisible();
    await this.searchInput.fill(query);
    await this.searchButton.click();

    // Wait for results or empty state
    await expect(this.resultItems.first().or(this.emptyState)).toBeVisible({
      timeout: 10000,
    });
  }

  /**
   * Search by pressing Enter
   */
  async searchByEnter(query: string): Promise<void> {
    await expect(this.searchInput).toBeVisible();
    await this.searchInput.fill(query);
    await this.searchInput.press("Enter");

    // Wait for results or empty state
    await expect(this.resultItems.first().or(this.emptyState)).toBeVisible({
      timeout: 10000,
    });
  }

  /**
   * Get search results count
   */
  async getResultsCount(): Promise<number> {
    // Wait a moment for results to render
    await this.page.waitForTimeout(500);
    return this.resultItems.count();
  }

  /**
   * Check if empty state is shown
   */
  async hasEmptyState(): Promise<boolean> {
    try {
      await expect(this.emptyState).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if results are shown
   */
  async hasResults(): Promise<boolean> {
    try {
      await expect(this.resultItems.first()).toBeVisible({ timeout: 5000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Clear search input
   */
  async clearSearch(): Promise<void> {
    await this.searchInput.clear();
  }

  /**
   * Get current search query value
   */
  async getSearchQuery(): Promise<string> {
    return (await this.searchInput.inputValue()) ?? "";
  }

  /**
   * Click on first search result
   */
  async clickFirstResult(): Promise<void> {
    await expect(this.resultItems.first()).toBeVisible();
    await this.resultItems.first().click();
  }

  /**
   * Get first result title
   */
  async getFirstResultTitle(): Promise<string> {
    const firstResult = this.resultItems.first();
    await expect(firstResult).toBeVisible();
    const heading = firstResult.locator('h2, h3, [class*="title"]').first();
    return (await heading.textContent()) ?? "";
  }
}
