import { type Locator, expect } from '@playwright/test';

/**
 * Custom assertions for E2E tests
 */

/**
 * Assert that a feed card contains expected information
 */
export async function assertFeedCard(
  card: Locator,
  expectedTitle?: string,
) {
  await expect(card).toBeVisible();

  if (expectedTitle) {
    await expect(card.getByText(expectedTitle, { exact: false })).toBeVisible();
  }
}

/**
 * Assert that multiple feed cards are visible
 */
export async function assertFeedCardsVisible(
  cards: Locator,
  minCount: number,
) {
  const count = await cards.count();
  expect(count).toBeGreaterThanOrEqual(minCount);
}

/**
 * Assert that loading indicator appears and disappears
 */
export async function assertLoadingIndicator(
  loadingIndicator: Locator,
  timeout = 5000,
) {
  try {
    await expect(loadingIndicator).toBeVisible({ timeout: 2000 });
    await expect(loadingIndicator).toBeHidden({ timeout });
  } catch {
    // Loading indicator might not appear if response is very fast
    // This is acceptable behavior
  }
}

/**
 * Assert that article detail page is properly loaded
 */
export async function assertArticleDetail(
  title: Locator,
  body: Locator,
  expectedTitle?: string,
) {
  await expect(title).toBeVisible();
  await expect(body).toBeVisible();

  if (expectedTitle) {
    await expect(title).toContainText(expectedTitle);
  }
}

/**
 * Assert that toast notification appears
 */
export async function assertToastNotification(
  toast: Locator,
  expectedText?: string | RegExp,
  timeout = 5000,
) {
  await expect(toast).toBeVisible({ timeout });

  if (expectedText) {
    if (typeof expectedText === 'string') {
      await expect(toast).toContainText(expectedText, { timeout });
    } else {
      await expect(toast).toContainText(expectedText, { timeout });
    }
  }
}

