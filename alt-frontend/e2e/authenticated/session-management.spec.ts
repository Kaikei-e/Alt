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

test.describe('Session Management', () => {
  test('should maintain session after browser refresh', async ({ page }) => {
    await login(page);
    
    // Navigate to a protected page
    await page.goto('/desktop/home');
    await expect(page).toHaveURL('/desktop/home');
    
    // Refresh the page
    await page.reload();
    
    // Should still be on the same page, not redirected to login
    await expect(page).toHaveURL('/desktop/home');
    await expect(page).not.toHaveURL(/\/auth\/login/);
  });

  test('should handle session cookies correctly', async ({ page }) => {
    await login(page);
    
    // Check that session cookie is set
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(cookie => cookie.name === 'ory_kratos_session');
    expect(sessionCookie).toBeDefined();
    expect(sessionCookie?.httpOnly).toBe(true);
  });

  test('should handle invalid session gracefully', async ({ page }) => {
    // Set an invalid session cookie
    await page.context().addCookies([{
      name: 'ory_kratos_session',
      value: 'invalid-session-id',
      domain: 'localhost',
      path: '/',
      httpOnly: true,
      secure: false,
      sameSite: 'Lax'
    }]);
    
    // Try to access a protected page
    await page.goto('/desktop/home');
    
    // Should redirect to login due to invalid session
    await page.waitForURL(/\/auth\/login\?flow=/);
    expect(page.url()).toMatch(/\/auth\/login\?flow=/);
  });

  test('should protect all desktop routes', async ({ page }) => {
    const protectedRoutes = [
      '/desktop/home',
      '/desktop/feeds',
      '/desktop/articles',
      '/desktop/settings',
      '/desktop/feeds/register',
      '/desktop/articles/search'
    ];
    
    for (const route of protectedRoutes) {
      await page.goto(route);
      
      // Should redirect to login
      await page.waitForURL(/\/auth\/login\?flow=/, { timeout: 5000 });
      expect(page.url()).toMatch(/\/auth\/login\?flow=/);
    }
  });

  test('should preserve return_to parameter for protected routes', async ({ page }) => {
    // Try to access a protected page directly
    await page.goto('/desktop/settings');
    
    // Should redirect to login with return_to parameter
    await page.waitForURL(/\/auth\/login\?flow=.*return_to=.*desktop%2Fsettings/);
    
    // Now log in
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    
    await page.getByLabel('Email').fill('test@example.com');
    await page.getByLabel('Password').fill('password123');
    await page.getByRole('button', { name: 'Sign in' }).click();
    
    // Should redirect back to the originally requested page
    await page.waitForURL('/desktop/settings', { timeout: 10000 });
    await expect(page).toHaveURL('/desktop/settings');
  });

  test('should handle concurrent sessions correctly', async ({ browser }) => {
    // Create two contexts (like two browser windows)
    const context1 = await browser.newContext();
    const context2 = await browser.newContext();
    
    const page1 = await context1.newPage();
    const page2 = await context2.newPage();
    
    try {
      // Log in on first page
      await login(page1);
      await page1.goto('/desktop/home');
      await expect(page1).toHaveURL('/desktop/home');
      
      // Second page should still require login
      await page2.goto('/desktop/home');
      await page2.waitForURL(/\/auth\/login\?flow=/);
      expect(page2.url()).toMatch(/\/auth\/login\?flow=/);
      
      // Log in on second page too
      await login(page2);
      await page2.goto('/desktop/feeds');
      await expect(page2).toHaveURL('/desktop/feeds');
      
      // Both sessions should remain valid
      await page1.reload();
      await expect(page1).toHaveURL('/desktop/home');
      
      await page2.reload();
      await expect(page2).toHaveURL('/desktop/feeds');
    } finally {
      await context1.close();
      await context2.close();
    }
  });
});