import { test as setup, expect } from '@playwright/test';
import path from 'path';

const authFile = path.join(process.cwd(), 'e2e/.auth/user.json');

setup('authenticate', async ({ request, context }) => {
  // Ensure mock service is running
  try {
    await request.get('http://localhost:4545/v1/health').catch(() => null);
  } catch (e) {
    console.warn('Mock service might not be running, attempting to proceed...');
  }

  // Directly create a session on the mock server
  const response = await request.post('http://localhost:4545/debug/create-session');
  expect(response.ok()).toBeTruthy();

  // Get the set-cookie header
  const headers = response.headers();
  const setCookie = headers['set-cookie'];

  if (setCookie) {
    // Parse the cookie manually since we are using APIRequestContext
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
    }
  }

  // Save storage state
  await context.storageState({ path: authFile });
});
