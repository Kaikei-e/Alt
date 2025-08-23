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

  test('should handle expired flow (410) and automatically redirect to new flow', async ({ page }) => {
    // Mock expired flow response from Kratos
    await page.route('**/self-service/login/flows?id=expired-flow-id', route => {
      route.fulfill({
        status: 410,
        contentType: 'application/json',
        body: JSON.stringify({
          error: {
            id: 'self_service_flow_expired',
            code: 410,
            status: 'Gone',
            message: 'The login flow expired 1.234 minutes ago. Please try again.'
          }
        })
      });
    });

    // Mock new flow creation
    await page.route('**/self-service/login/browser**', route => {
      const url = new URL(route.request().url());
      const returnTo = url.searchParams.get('return_to');
      
      // Redirect back to login page with new flow
      route.fulfill({
        status: 303,
        headers: {
          'Location': `/auth/login?flow=new-flow-id&return_to=${returnTo}`
        }
      });
    });

    // Mock the new flow data
    await page.route('**/self-service/login/flows?id=new-flow-id', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'new-flow-id',
          ui: {
            action: '/self-service/login?flow=new-flow-id',
            method: 'POST',
            nodes: [
              {
                type: 'input',
                attributes: {
                  name: 'identifier',
                  type: 'email',
                  required: true,
                },
                messages: [],
              },
              {
                type: 'input',
                attributes: {
                  name: 'password',
                  type: 'password',
                  required: true,
                },
                messages: [],
              }
            ]
          }
        })
      });
    });

    // Start with an expired flow
    await page.goto('/auth/login?flow=expired-flow-id&return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome');

    // Should automatically redirect to new flow creation
    await expect(page).toHaveURL(/flow=new-flow-id/);
    
    // Should show the login form with the new flow
    await expect(page.getByRole('textbox')).toBeVisible({ timeout: 5000 });
  });

  test('should handle 410 during form submission and redirect to new flow', async ({ page }) => {
    // Mock successful initial flow
    await page.route('**/self-service/login/flows?id=valid-flow-id', route => {
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          id: 'valid-flow-id',
          ui: {
            action: '/self-service/login?flow=valid-flow-id',
            method: 'POST',
            nodes: [
              {
                type: 'input',
                attributes: {
                  name: 'csrf_token',
                  type: 'hidden',
                  value: 'csrf-token-value'
                },
                messages: [],
              },
              {
                type: 'input',
                attributes: {
                  name: 'identifier',
                  type: 'email',
                  required: true,
                },
                messages: [],
              },
              {
                type: 'input',
                attributes: {
                  name: 'password',
                  type: 'password',
                  required: true,
                },
                messages: [],
              }
            ]
          }
        })
      });
    });

    // Mock 410 error during form submission
    await page.route('**/self-service/login?flow=valid-flow-id', route => {
      if (route.request().method() === 'POST') {
        route.fulfill({
          status: 410,
          contentType: 'application/json',
          body: JSON.stringify({
            error: {
              id: 'self_service_flow_expired',
              code: 410,
              status: 'Gone'
            }
          })
        });
      }
    });

    // Mock redirect to new flow creation
    await page.route('**/self-service/login/browser**', route => {
      const url = new URL(route.request().url());
      const returnTo = url.searchParams.get('return_to') || 'https%3A%2F%2Fcurionoah.com%2F';
      
      route.fulfill({
        status: 303,
        headers: {
          'Location': `/auth/login?flow=recovery-flow-id&return_to=${returnTo}`
        }
      });
    });

    await page.goto('/auth/login?flow=valid-flow-id&return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fanalytics');

    // Wait for form to load
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();

    // Fill and submit the form
    await page.getByLabel('Email').fill('test@example.com');
    await page.getByLabel('Password').fill('password123');
    await page.getByRole('button', { name: /sign in/i }).click();

    // Should redirect to new flow with preserved return_to
    await expect(page).toHaveURL(/flow=recovery-flow-id.*return_to=.*desktop%2Fanalytics/);
  });
});