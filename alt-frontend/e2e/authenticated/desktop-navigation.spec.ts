import { test, expect } from '@playwright/test';

// Helper function to log in
async function login(page: any) {
  await page.goto('/');
  
  // Wait for redirect to login page
  await page.waitForURL(/\/auth\/login\?flow=/);
  
  // Fill login form
  await expect(page.getByLabel('Email')).toBeVisible();
  await expect(page.getByLabel('Password')).toBeVisible();
  
  await page.getByLabel('Email').fill('test@example.com');
  await page.getByLabel('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in' }).click();
  
  // Wait for successful login redirect
  await page.waitForURL('/', { timeout: 10000 });
}

test.describe('Authenticated Desktop Navigation', () => {
  test('should access desktop home page after login', async ({ page }) => {
    await login(page);
    
    // Navigate to desktop home
    await page.goto('/desktop/home');
    await expect(page).toHaveURL('/desktop/home');
    
    // Check for typical desktop home elements
    // Note: These might need adjustment based on actual implementation
    await expect(page).toHaveTitle(/Alt/);
  });

  test('should access feeds page after login', async ({ page }) => {
    await login(page);
    
    // Navigate to feeds page
    await page.goto('/desktop/feeds');
    await expect(page).toHaveURL('/desktop/feeds');
    
    // Check page loads without error
    await expect(page).toHaveTitle(/Alt/);
  });

  test('should access articles page after login', async ({ page }) => {
    await login(page);
    
    // Navigate to articles page
    await page.goto('/desktop/articles');
    await expect(page).toHaveURL('/desktop/articles');
    
    // Check page loads without error
    await expect(page).toHaveTitle(/Alt/);
  });

  test('should access settings page after login', async ({ page }) => {
    await login(page);
    
    // Navigate to settings page
    await page.goto('/desktop/settings');
    await expect(page).toHaveURL('/desktop/settings');
    
    // Check page loads without error
    await expect(page).toHaveTitle(/Alt/);
  });

  test('should redirect to login when accessing protected pages without auth', async ({ page }) => {
    // Try to access protected page without authentication
    await page.goto('/desktop/home');
    
    // Should redirect to login
    await page.waitForURL(/\/auth\/login\?flow=/);
    expect(page.url()).toMatch(/\/auth\/login\?flow=/);
  });

  test('should maintain session across page navigation', async ({ page }) => {
    await login(page);
    
    // Navigate between different pages
    await page.goto('/desktop/home');
    await expect(page).toHaveURL('/desktop/home');
    
    await page.goto('/desktop/feeds');
    await expect(page).toHaveURL('/desktop/feeds');
    
    await page.goto('/desktop/articles');
    await expect(page).toHaveURL('/desktop/articles');
    
    // Should not be redirected to login
    await expect(page).not.toHaveURL(/\/auth\/login/);
  });

  test('should handle direct navigation to protected routes', async ({ page }) => {
    await login(page);
    
    // Test direct navigation with browser address bar simulation
    await page.goto('/desktop/feeds/register');
    await expect(page).toHaveURL('/desktop/feeds/register');
    
    // Should not redirect to login since user is authenticated
    await expect(page).not.toHaveURL(/\/auth\/login/);
  });
});