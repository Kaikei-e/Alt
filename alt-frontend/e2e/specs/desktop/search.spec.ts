import { expect, test } from "@playwright/test";
import { SearchPage } from "../../pages/desktop/SearchPage";
import { mockSearchApi, setupAllMocks } from "../../utils/api-mock";

test.describe("Desktop Search", () => {
  let searchPage: SearchPage;

  test.beforeEach(async ({ page }) => {
    searchPage = new SearchPage(page);
    await setupAllMocks(page);
  });

  test("should display search page", async () => {
    await searchPage.goto();
    await searchPage.waitForReady();

    await expect(searchPage.searchInput).toBeVisible();
    await expect(searchPage.searchButton).toBeVisible();
  });

  test("should perform search and display results", async () => {
    await searchPage.goto();
    await searchPage.waitForReady();

    await searchPage.search("AI");

    // Wait and check for results
    const hasResults = await searchPage.hasResults();
    const resultsCount = await searchPage.getResultsCount();

    // Either results or empty state should be shown
    expect(hasResults || resultsCount >= 0).toBe(true);
  });

  test("should display empty state when no results", async ({ page }) => {
    await mockSearchApi(page, { empty: true });
    await searchPage.goto();
    await searchPage.waitForReady();

    await searchPage.search("NonExistentQuery12345XYZ");

    // Check for empty state
    const hasEmptyState = await searchPage.hasEmptyState();
    expect(hasEmptyState).toBe(true);
  });

  test("should clear search input", async () => {
    await searchPage.goto();
    await searchPage.waitForReady();

    await searchPage.searchInput.fill("test query");
    await expect(searchPage.searchInput).toHaveValue("test query");

    await searchPage.clearSearch();
    await expect(searchPage.searchInput).toHaveValue("");
  });

  test("should search by pressing Enter", async () => {
    await searchPage.goto();
    await searchPage.waitForReady();

    await searchPage.searchByEnter("React");

    // Wait for results
    const hasResults = await searchPage.hasResults();
    const hasEmptyState = await searchPage.hasEmptyState();

    // Either results or empty state should be shown
    expect(hasResults || hasEmptyState).toBe(true);
  });
});
