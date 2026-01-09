import type { FullConfig } from '@playwright/test';

/**
 * Global setup for Playwright E2E tests
 * Runs once before all tests
 */
async function globalSetup(config: FullConfig): Promise<void> {
  console.log('[E2E Global Setup] Starting...');

  // Get base URL from config
  const baseUrl = config.projects[0]?.use?.baseURL || 'http://localhost:3000';
  console.log(`[E2E Global Setup] Base URL: ${baseUrl}`);

  // Health check for mock auth service
  const mockServiceUrl = 'http://localhost:4545/v1/health';
  const maxRetries = 10;
  let healthy = false;

  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch(mockServiceUrl);
      if (response.ok) {
        healthy = true;
        console.log('[E2E Global Setup] Mock service is healthy');
        break;
      }
    } catch {
      console.log(
        `[E2E Global Setup] Waiting for mock service... (attempt ${i + 1}/${maxRetries})`,
      );
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }

  if (!healthy) {
    console.warn(
      '[E2E Global Setup] Mock service not available, will be started by webServer config',
    );
  }

  // Optionally check if Next.js server is running
  try {
    const nextResponse = await fetch(baseUrl);
    if (nextResponse.ok) {
      console.log('[E2E Global Setup] Next.js server is running');
    }
  } catch {
    console.warn(
      '[E2E Global Setup] Next.js server not running. Please start with: pnpm dev',
    );
  }

  console.log('[E2E Global Setup] Complete');
}

export default globalSetup;
