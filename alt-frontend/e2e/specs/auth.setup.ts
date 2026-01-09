import { test as setup, expect } from '@playwright/test';
import path from 'path';

const authFile = path.join(process.cwd(), 'e2e/.auth/user.json');

setup('authenticate', async ({ request, context }) => {
  console.log('[Auth Setup] Starting authentication...');

  // Wait for mock service to be healthy with retries
  const maxRetries = 10;
  let healthy = false;

  for (let i = 0; i < maxRetries; i++) {
    try {
      const healthResponse = await request.get('http://localhost:4545/v1/health');
      if (healthResponse.ok()) {
        healthy = true;
        console.log('[Auth Setup] Mock service is healthy');
        break;
      }
    } catch {
      console.log(
        `[Auth Setup] Waiting for mock service... (attempt ${i + 1}/${maxRetries})`,
      );
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }

  if (!healthy) {
    throw new Error('[Auth Setup] Mock service is not healthy after retries');
  }

  // Create a session on the mock server
  const response = await request.post('http://localhost:4545/debug/create-session');
  expect(response.ok()).toBeTruthy();
  console.log('[Auth Setup] Session created');

  // Get the set-cookie header
  const headers = response.headers();
  const setCookie = headers['set-cookie'];

  if (setCookie) {
    // Parse the cookie manually
    const match = setCookie.match(/ory_kratos_session=([^;]+)/);
    if (match) {
      const cookieValue = match[1];
      await context.addCookies([
        {
          name: 'ory_kratos_session',
          value: cookieValue,
          domain: 'localhost',
          path: '/',
          httpOnly: true,
          sameSite: 'Lax',
          expires: Date.now() / 1000 + 86400, // 24 hours
        },
      ]);
      console.log('[Auth Setup] Session cookie added');
    }
  }

  // Save storage state
  await context.storageState({ path: authFile });
  console.log('[Auth Setup] Storage state saved to:', authFile);
});
