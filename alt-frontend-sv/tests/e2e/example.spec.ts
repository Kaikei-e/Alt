import { test, expect } from '@playwright/test';

test('health check returns 200 OK', async ({ page }) => {
  // Use an explicit local URL to avoid accidental redirects to protected prod hosts.
  const healthUrl = 'http://127.0.0.1:4174/sv/health';
  const response = await page.request.get(healthUrl);

  expect(response.status()).toBe(200);
  expect(await response.text()).toBe('OK');
});
