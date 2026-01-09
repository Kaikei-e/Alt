import { Locator, Page } from '@playwright/test';
import { BasePage } from '../BasePage';

export class MobileSearchPage extends BasePage {
  readonly searchInput: Locator;
  readonly searchButton: Locator;
  readonly searchResults: Locator;
  readonly searchMetadata: Locator;
  readonly searchCount: Locator;
  readonly emptyState: Locator;
  readonly emptyHeading: Locator;
  readonly articleCards: Locator;
  readonly loadingIndicator: Locator;

  constructor(page: Page) {
    super(page);
    this.searchInput = page.getByTestId('search-input');
    this.searchButton = page.getByTestId('search-button');
    this.searchResults = page.getByTestId('search-results');
    this.searchMetadata = page.getByTestId('search-metadata');
    this.searchCount = page.getByTestId('search-count');
    this.emptyState = page.getByTestId('search-empty-state');
    this.emptyHeading = page.getByTestId('search-empty-heading');
    this.articleCards = page.getByTestId('article-card');
    this.loadingIndicator = page.getByText('Searching articles...');
  }

  async goto(): Promise<void> {
    await this.page.goto('/mobile/articles/search');
    await this.waitForReady();
  }

  async waitForReady(): Promise<void> {
    await this.searchInput.waitFor({ state: 'visible', timeout: 15000 });
  }

  async search(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchButton.click();
  }

  async searchByEnter(query: string): Promise<void> {
    await this.searchInput.fill(query);
    await this.searchInput.press('Enter');
  }

  async waitForResults(): Promise<void> {
    // Wait for loading to finish
    await this.loadingIndicator.waitFor({ state: 'hidden', timeout: 15000 }).catch(() => {});
    // Wait for results or empty state
    await Promise.race([
      this.searchResults.waitFor({ state: 'visible', timeout: 15000 }),
      this.emptyState.waitFor({ state: 'visible', timeout: 15000 }),
    ]);
  }

  async hasResults(): Promise<boolean> {
    return await this.searchResults.isVisible();
  }

  async hasEmptyState(): Promise<boolean> {
    return await this.emptyState.isVisible();
  }

  async getResultsCount(): Promise<number> {
    return await this.articleCards.count();
  }

  async clearSearch(): Promise<void> {
    await this.searchInput.clear();
  }
}
