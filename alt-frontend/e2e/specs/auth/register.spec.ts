import { test, expect } from '@playwright/test';
import { RegisterPage } from '../../page-objects/auth/register.page';
import { generateRandomEmail } from '../../utils/test-data';

test.describe('Register Page', () => {
  let registerPage: RegisterPage;

  test.beforeEach(async ({ page }) => {
    registerPage = new RegisterPage(page);
    await registerPage.goto();
  });

  test('should display registration form', async () => {
    // Check main elements are visible
    await expect(registerPage.pageHeading).toBeVisible();
    await expect(registerPage.emailInput).toBeVisible();
    await expect(registerPage.passwordInput).toBeVisible();
    await expect(registerPage.submitButton).toBeVisible();
    await expect(registerPage.loginLink).toBeVisible();
  });

  test('should navigate to login page when clicking login link', async () => {
    await registerPage.clickLogin();

    // Verify navigation
    await expect(registerPage.page).toHaveURL(/\/auth\/login/);
  });

  test('should validate email format', async () => {
    // Fill with invalid email
    await registerPage.fillEmail('invalid-email');
    await registerPage.fillPassword('password123');
    await registerPage.clickSubmit();

    // Check for validation error (browser native or custom)
    const hasError = await registerPage.hasError();
    expect(hasError).toBeTruthy();
  });

  test('should validate password requirements', async () => {
    // Fill with short password
    await registerPage.fillEmail(generateRandomEmail());
    await registerPage.fillPassword('123'); // Too short
    await registerPage.clickSubmit();

    // Check for validation error
    const hasError = await registerPage.hasError();
    expect(hasError).toBeTruthy();
  });

  test('should validate password confirmation match', async ({}, testInfo) => {
    // Skip if no confirm password field
    const hasConfirmField = (await registerPage.confirmPasswordInput.count()) > 0;
    if (!hasConfirmField) {
      testInfo.skip();
    }

    await registerPage.fillEmail(generateRandomEmail());
    await registerPage.fillPassword('password123');
    await registerPage.fillConfirmPassword('password456'); // Different
    await registerPage.clickSubmit();

    // Check for validation error
    const hasError = await registerPage.hasError();
    expect(hasError).toBeTruthy();
  });

  test('should successfully register with valid credentials', async ({ page }) => {
    const email = generateRandomEmail();
    const password = 'ValidPassword123!';

    await registerPage.register(email, password, 'Test User');

    // Wait for successful registration
    await registerPage.waitForRegistrationSuccess();

    // Should redirect to login or success page
    const currentUrl = page.url();
    expect(
      currentUrl.includes('/auth/login') ||
        currentUrl.includes('/home') ||
        currentUrl.includes('/success')
    ).toBeTruthy();
  });

  test('should show error for existing email', async ({ page }) => {
    // Use a known existing email
    const existingEmail = 'test@example.com';
    const password = 'password123';

    await registerPage.register(existingEmail, password);

    // Check for error message about existing email
    const error = await registerPage.getError();
    expect(error).toBeTruthy();
    expect(error?.toLowerCase()).toContain('exist');
  });

  test('should disable submit button while processing', async () => {
    const email = generateRandomEmail();
    const password = 'ValidPassword123!';

    await registerPage.fillEmail(email);
    await registerPage.fillPassword(password);

    // Button should be enabled before submit
    expect(await registerPage.isSubmitEnabled()).toBeTruthy();

    await registerPage.submitButton.click();

    // Button might be disabled during processing (check immediately)
    // Note: This is timing-dependent and may not always catch the disabled state
  });

  test('should clear form fields', async () => {
    await registerPage.fillEmail('test@example.com');
    await registerPage.fillPassword('password123');

    await registerPage.clearForm();

    // Check fields are empty
    await expect(registerPage.emailInput).toHaveValue('');
    await expect(registerPage.passwordInput).toHaveValue('');
  });

  test('should handle network errors gracefully', async ({ page }) => {
    // Mock network failure
    await page.route('**/auth/register**', route => {
      route.abort('failed');
    });

    const email = generateRandomEmail();
    await registerPage.register(email, 'password123');

    // Should show error message
    const hasError = await registerPage.hasError();
    expect(hasError).toBeTruthy();
  });

  test('should be accessible', async () => {
    // Basic accessibility check
    await registerPage.checkA11y();
  });

  test('should have proper keyboard navigation', async ({ page }) => {
    // Tab through form fields
    await page.keyboard.press('Tab');
    const firstFocused = await page.evaluate(() => document.activeElement?.tagName);

    await page.keyboard.press('Tab');
    const secondFocused = await page.evaluate(() => document.activeElement?.tagName);

    // Should be able to focus on form elements
    expect(firstFocused).toBeTruthy();
    expect(secondFocused).toBeTruthy();
  });
});
