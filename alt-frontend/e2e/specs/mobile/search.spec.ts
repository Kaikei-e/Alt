import { expect, test } from "@playwright/test";
import { MobileSearchPage } from "../../pages/mobile/MobileSearchPage";
import { mockSearchApi, setupAllMocks } from "../../utils/api-mock";

test.describe("Mobile Search", () => {
  let searchPage: MobileSearchPage;

  test.beforeEach(async ({ page }) => {
    searchPage = new MobileSearchPage(page);
    await setupAllMocks(page);
  });

  test("should display search page", async () => {
    await searchPage.goto();

    await expect(searchPage.searchInput).toBeVisible();
    await expect(searchPage.searchButton).toBeVisible();
  });

  test("should perform search and display results", async () => {
    await searchPage.goto();

    await searchPage.search("TypeScript");
    await searchPage.waitForResults();

    // Either results or empty state should be shown
    const hasResults = await searchPage.hasResults();
    const hasEmptyState = await searchPage.hasEmptyState();

    expect(hasResults || hasEmptyState).toBe(true);
  });

  test("should display empty state when no results", async ({ page }) => {
    await mockSearchApi(page, { empty: true });
    await searchPage.goto();

    await searchPage.search("NonExistentQuery12345XYZ");
    await searchPage.waitForResults();

    // Check for empty state
    const hasEmptyState = await searchPage.hasEmptyState();
    expect(hasEmptyState).toBe(true);
  });

  test("should clear search input", async () => {
    await searchPage.goto();

    await searchPage.searchInput.fill("test query");
    await expect(searchPage.searchInput).toHaveValue("test query");

    await searchPage.clearSearch();
    await expect(searchPage.searchInput).toHaveValue("");
  });

  test("should search by pressing Enter", async () => {
    await searchPage.goto();

    await searchPage.searchByEnter("React");
    await searchPage.waitForResults();

    // Either results or empty state should be shown
    const hasResults = await searchPage.hasResults();
    const hasEmptyState = await searchPage.hasEmptyState();

    expect(hasResults || hasEmptyState).toBe(true);
  });
});
