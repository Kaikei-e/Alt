import { test, expect } from '@playwright/test';

test('health check returns 200 OK', async ({ page }) => {
  // Access the health endpoint
  // baseURL is configured as http://127.0.0.1:4173/sv/
  const response = await page.goto('health');

  // Verify status code is 200
  expect(response?.status()).toBe(200);

  // Verify response body is "OK"
  const text = await page.textContent('body');
  // Since the endpoint returns raw text, browsers might wrap it in a pre or body
  // Or we can check response.text() directly
  const responseText = await response?.text();
  expect(responseText).toBe('OK');
});
