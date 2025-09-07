import { Page, Locator, expect } from '@playwright/test';

/**
 * Utility functions for improved wait conditions and test stability
 */

/**
 * Wait for element to be stable (not moving/changing)
 */
export async function waitForElementStable(element: Locator, timeout = 5000) {
  await element.waitFor({ state: 'visible', timeout });
  
  // Wait for element to be stable (no size changes)
  let previousBox = await element.boundingBox();
  let stableCount = 0;
  const requiredStableCount = 3;
  
  while (stableCount < requiredStableCount && timeout > 0) {
    await new Promise(resolve => setTimeout(resolve, 100));
    const currentBox = await element.boundingBox();
    
    if (previousBox && currentBox && 
        previousBox.x === currentBox.x && 
        previousBox.y === currentBox.y &&
        previousBox.width === currentBox.width &&
        previousBox.height === currentBox.height) {
      stableCount++;
    } else {
      stableCount = 0;
    }
    
    previousBox = currentBox;
    timeout -= 100;
  }
  
  return element;
}

/**
 * Wait for page to be fully loaded and interactive
 */
export async function waitForPageReady(page: Page, options: { timeout?: number; waitForSelector?: string } = {}) {
  const { timeout = 30000, waitForSelector } = options;
  
  // Wait for network to be idle
  await page.waitForLoadState('networkidle', { timeout });
  
  // Wait for DOM content to be loaded
  await page.waitForLoadState('domcontentloaded', { timeout });
  
  // Wait for specific selector if provided
  if (waitForSelector) {
    await page.waitForSelector(waitForSelector, { timeout, state: 'visible' });
  }
  
  // Wait for any pending JavaScript to complete
  await page.evaluate(() => {
    return new Promise(resolve => {
      if (document.readyState === 'complete') {
        resolve(void 0);
      } else {
        window.addEventListener('load', () => resolve(void 0));
      }
    });
  });
}

/**
 * Retry an operation with exponential backoff
 */
export async function retryOperation<T>(
  operation: () => Promise<T>,
  options: {
    maxRetries?: number;
    initialDelay?: number;
    maxDelay?: number;
    backoffFactor?: number;
  } = {}
): Promise<T> {
  const {
    maxRetries = 3,
    initialDelay = 100,
    maxDelay = 5000,
    backoffFactor = 2
  } = options;
  
  let lastError: Error;
  let delay = initialDelay;
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await operation();
    } catch (error) {
      lastError = error as Error;
      
      if (attempt === maxRetries) {
        throw lastError;
      }
      
      await new Promise(resolve => setTimeout(resolve, delay));
      delay = Math.min(delay * backoffFactor, maxDelay);
    }
  }
  
  throw lastError!;
}

/**
 * Wait for URL change with timeout
 */
export async function waitForUrlChange(page: Page, fromUrl: string, timeout = 10000): Promise<string> {
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeout) {
    const currentUrl = page.url();
    if (currentUrl !== fromUrl) {
      return currentUrl;
    }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  
  throw new Error(`URL did not change from ${fromUrl} within ${timeout}ms`);
}

/**
 * Wait for element to contain specific text
 */
export async function waitForTextContent(
  element: Locator,
  expectedText: string | RegExp,
  timeout = 10000
): Promise<void> {
  await expect(element).toContainText(expectedText, { timeout });
}

/**
 * Wait for form to be ready for interaction
 */
export async function waitForFormReady(page: Page, formSelector = 'form', timeout = 10000) {
  const form = page.locator(formSelector);
  await form.waitFor({ state: 'visible', timeout });
  
  // Wait for all form inputs to be ready
  const inputs = form.locator('input, select, textarea, button');
  const inputCount = await inputs.count();
  
  for (let i = 0; i < inputCount; i++) {
    const input = inputs.nth(i);
    await input.waitFor({ state: 'visible', timeout: 2000 });
  }
  
  // Wait for form to be stable
  await waitForElementStable(form, 2000);
  
  return form;
}

/**
 * Safe click that waits for element to be ready
 */
export async function safeClick(element: Locator, options: { timeout?: number; force?: boolean } = {}) {
  const { timeout = 10000, force = false } = options;
  
  await element.waitFor({ state: 'visible', timeout });
  await element.waitFor({ state: 'attached', timeout });
  
  if (!force) {
    // Wait for element to be enabled
    await expect(element).toBeEnabled({ timeout });
    
    // Wait for element to be stable
    await waitForElementStable(element, Math.min(timeout, 2000));
  }
  
  await element.click({ timeout, force });
}

/**
 * Safe fill that ensures input is ready
 */
export async function safeFill(element: Locator, value: string, options: { timeout?: number } = {}) {
  const { timeout = 10000 } = options;
  
  await element.waitFor({ state: 'visible', timeout });
  await expect(element).toBeEnabled({ timeout });
  
  // Clear the field first
  await element.clear({ timeout });
  
  // Fill the value
  await element.fill(value, { timeout });
  
  // Verify the value was set correctly
  await expect(element).toHaveValue(value, { timeout: 2000 });
}

/**
 * Wait for authentication redirect with enhanced error handling
 */
export async function waitForAuthRedirect(
  page: Page,
  options: {
    timeout?: number;
    expectedFlow?: RegExp;
    debugLogging?: boolean;
  } = {}
): Promise<string> {
  const {
    timeout = 30000,
    expectedFlow = /\/auth\/login\?flow=/,
    debugLogging = false
  } = options;

  const mockPort = process.env.PW_MOCK_PORT || '4545';
  const startTime = Date.now();
  const startUrl = page.url();

  if (debugLogging) {
    console.log(`[waitForAuthRedirect] Starting from URL: ${startUrl}`);
    console.log(`[waitForAuthRedirect] Waiting for redirect to pattern: ${expectedFlow}`);
    console.log(`[waitForAuthRedirect] Using mock port: ${mockPort}`);
  }

  try {
    // First wait for potential redirect to mock auth server
    const authServerPattern = new RegExp(`localhost:${mockPort}.*login\\/browser`);
    
    // Check if we need to go through the mock auth server first
    if (!expectedFlow.test(startUrl)) {
      try {
        await page.waitForURL(authServerPattern, { timeout: timeout / 2 });
        if (debugLogging) {
          console.log(`[waitForAuthRedirect] Redirected to mock auth server: ${page.url()}`);
        }
      } catch (error) {
        if (debugLogging) {
          console.log(`[waitForAuthRedirect] No redirect to mock auth server, continuing...`);
        }
        // It's okay if we don't go through mock server, continue to final URL check
      }
    }

    // Wait for final redirect to the expected flow pattern
    await page.waitForURL(expectedFlow, { timeout: timeout / 2 });
    
    const finalUrl = page.url();
    if (debugLogging) {
      console.log(`[waitForAuthRedirect] Successfully redirected to: ${finalUrl}`);
      console.log(`[waitForAuthRedirect] Total time: ${Date.now() - startTime}ms`);
    }

    // Verify the page loaded properly
    await page.waitForLoadState('domcontentloaded', { timeout: 5000 });

    return finalUrl;
  } catch (error) {
    const currentUrl = page.url();
    const elapsedTime = Date.now() - startTime;
    
    console.error(`[waitForAuthRedirect] Failed after ${elapsedTime}ms`);
    console.error(`[waitForAuthRedirect] Current URL: ${currentUrl}`);
    console.error(`[waitForAuthRedirect] Expected pattern: ${expectedFlow}`);
    console.error(`[waitForAuthRedirect] Original error:`, error.message);
    
    // Add more context to the error
    const enhancedError = new Error(
      `Auth redirect failed: Expected URL matching ${expectedFlow}, but got ${currentUrl} after ${elapsedTime}ms. Original: ${error.message}`
    );
    enhancedError.stack = error.stack;
    throw enhancedError;
  }
}

/**
 * Wait for authentication flow to complete with success
 */
export async function waitForAuthComplete(
  page: Page,
  expectedDestination: string | RegExp,
  options: {
    timeout?: number;
    debugLogging?: boolean;
  } = {}
): Promise<void> {
  const { timeout = 20000, debugLogging = false } = options;
  
  if (debugLogging) {
    console.log(`[waitForAuthComplete] Waiting for completion, destination: ${expectedDestination}`);
  }

  // Wait for redirect to destination
  await page.waitForURL(expectedDestination, { timeout });
  
  // Ensure page is fully loaded
  await waitForPageReady(page, { timeout: 10000 });
  
  if (debugLogging) {
    console.log(`[waitForAuthComplete] Auth completed successfully: ${page.url()}`);
  }
}