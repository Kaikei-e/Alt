import { test, expect } from '@playwright/test';

test.describe('Settings Page', () => {
  test.beforeEach(async ({ page }) => {
    // Mock GET profile
    await page.route('**/api/user/profile', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'user123',
          name: 'Original Name',
          email: 'test@example.com',
        }),
      });
    });

    // Mock PUT profile
    await page.route('**/api/user/profile', async (route) => {
      if (route.request().method() === 'PUT') {
        const postData = route.request().postDataJSON();
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            id: 'user123',
            name: postData.name,
            email: 'test@example.com',
          }),
        });
      } else {
        await route.continue();
      }
    });
  });

  test('should update user profile', async ({ page }) => {
    await page.goto('/desktop/settings');

    // Verify initial state
    await expect(page.locator('input#name')).toHaveValue('Original Name');

    // Update name
    await page.locator('input#name').fill('Updated Name');
    await page.getByRole('button', { name: /save|update/i }).click();

    // Verify success message
    await expect(page.getByText('Profile updated.')).toBeVisible();

    // Verify updated value (optional, if page reloads or updates state)
    await expect(page.getByLabel(/name/i)).toHaveValue('Updated Name');
  });
});
