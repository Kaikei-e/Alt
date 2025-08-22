import { test, expect } from '@playwright/test';

test.describe('Login Flow', () => {
  test('should complete full login flow', async ({ page }) => {
    // 未認証状態でアプリにアクセス
    await page.goto('/');
    
    // ミドルウェアによりKratosログインページにリダイレクトされる
    await expect(page).toHaveURL(/id\.curionoah\.com/);
    
    // Kratosがフローを作成してアプリに戻る
    await page.waitForURL(/\/auth\/login\?flow=/);
    
    // ログインフォームが表示される
    await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    
    // ログイン情報を入力
    await page.getByLabel('Email').fill('test@example.com');
    await page.getByLabel('Password').fill('password123');
    
    // ログイン送信
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    // 成功時はホームページにリダイレクト
    await expect(page).toHaveURL('/');
  });

  test('should show error for invalid credentials', async ({ page }) => {
    await page.goto('/auth/login?flow=test-flow-id');
    
    await page.getByLabel('Email').fill('wrong@example.com');
    await page.getByLabel('Password').fill('wrongpassword');
    await page.getByRole('button', { name: 'Sign In' }).click();
    
    await expect(page.getByText(/credentials are invalid/i)).toBeVisible();
  });

  test('should handle direct access to login page without flow', async ({ page }) => {
    // フローIDなしで直接ログインページにアクセス
    await page.goto('/auth/login');
    
    // Kratosのlogin/browserにリダイレクトされるべき
    await expect(page).toHaveURL(/id\.curionoah\.com.*login\/browser/);
  });

  test('should display loading state initially', async ({ page }) => {
    // モックしたKratosフローでページにアクセス
    await page.goto('/auth/login?flow=test-flow-id');
    
    // 最初にローディングが表示される（短時間）
    await expect(page.getByText(/loading/i)).toBeVisible({ timeout: 1000 });
  });
});