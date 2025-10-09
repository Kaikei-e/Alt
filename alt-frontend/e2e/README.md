# Alt Frontend E2E Tests

This directory contains end-to-end (E2E) tests for the Alt frontend application using [Playwright](https://playwright.dev/).

## 📁 Directory Structure

```
e2e/
├── README.md                          # This file
├── fixtures/                          # Test fixtures
│   ├── authenticated.fixture.ts       # Authenticated user fixture
│   ├── desktop.fixture.ts             # Desktop device fixture
│   └── mobile.fixture.ts              # Mobile device fixture
│
├── page-objects/                      # Page Object Model (POM)
│   ├── base.page.ts                   # Base page class
│   ├── auth/                          # Authentication pages
│   ├── desktop/                       # Desktop pages
│   ├── mobile/                        # Mobile pages
│   └── public/                        # Public pages
│
├── specs/                             # Test specifications
│   ├── auth/                          # Authentication tests
│   ├── authenticated/                 # Authenticated user tests
│   ├── desktop/                       # Desktop-specific tests
│   ├── mobile/                        # Mobile-specific tests
│   ├── public/                        # Public page tests
│   └── e2e-flows/                     # End-to-end user flow tests
│
└── utils/                             # Utility functions
    ├── test-data.ts                   # Test data generators
    ├── api-mocks.ts                   # API mocking helpers
    ├── accessibility.ts               # Accessibility testing utilities
    └── performance.ts                 # Performance testing utilities
```

## 🚀 Getting Started

### Prerequisites

- Node.js 20+
- pnpm installed
- Docker Compose stack running (for backend services)

### Installation

```bash
# Install dependencies (if not already done)
pnpm install

# Install Playwright browsers
pnpm exec playwright install
```

### Running Tests

```bash
# Run all E2E tests
pnpm test:e2e

# Run tests in headed mode (see the browser)
pnpm exec playwright test --headed

# Run specific project
pnpm exec playwright test --project=authenticated-chrome

# Run specific test file
pnpm exec playwright test e2e/auth/login.spec.ts

# Debug mode
pnpm exec playwright test --debug

# UI mode (interactive)
pnpm exec playwright test --ui
```

### Viewing Reports

```bash
# Show HTML report
pnpm exec playwright show-report

# Open trace viewer for failed test
pnpm exec playwright show-trace trace.zip
```

## 🏗️ Architecture

### Page Object Model (POM)

All tests follow the Page Object Model pattern for better maintainability and reusability.

**Example:**

```typescript
// page-objects/desktop/feeds.page.ts
import { BasePage } from '../base.page';
import { Page, Locator } from '@playwright/test';

export class DesktopFeedsPage extends BasePage {
  readonly feedsList: Locator;
  readonly addFeedButton: Locator;

  constructor(page: Page) {
    super(page);
    this.feedsList = page.getByRole('list');
    this.addFeedButton = page.getByRole('button', { name: /add feed/i });
  }

  async goto(): Promise<void> {
    await this.page.goto('/desktop/feeds');
    await this.waitForLoad();
  }

  async waitForLoad(): Promise<void> {
    await this.waitForElement(this.feedsList);
  }

  async clickAddFeed(): Promise<void> {
    await this.addFeedButton.click();
  }
}
```

### Test Fixtures

We provide several test fixtures for common scenarios:

1. **Authenticated Fixture** - For tests requiring authentication
2. **Desktop Fixture** - For desktop-specific tests
3. **Mobile Fixture** - For mobile-specific tests

**Usage:**

```typescript
import { test, expect } from '../fixtures/authenticated.fixture';

test('authenticated user can view feeds', async ({ authenticatedPage }) => {
  await authenticatedPage.goto('/desktop/feeds');
  // ... test code
});
```

## 📝 Writing Tests

### Test Structure

```typescript
import { test, expect } from '@playwright/test';
import { DesktopFeedsPage } from '../../page-objects/desktop/feeds.page';

test.describe('Desktop Feeds Page', () => {
  let feedsPage: DesktopFeedsPage;

  test.beforeEach(async ({ page }) => {
    feedsPage = new DesktopFeedsPage(page);
    await feedsPage.goto();
  });

  test('should display feeds list', async () => {
    await expect(feedsPage.feedsList).toBeVisible();
  });

  test('should navigate to add feed page', async () => {
    await feedsPage.clickAddFeed();
    await expect(feedsPage.page).toHaveURL(/\/desktop\/feeds\/register/);
  });
});
```

### Best Practices

1. **Use semantic locators** - Prefer `getByRole`, `getByLabel` over `getByTestId`
2. **Leverage auto-waiting** - Playwright automatically waits for elements
3. **Test isolation** - Each test should be independent
4. **Use Page Objects** - Encapsulate page interactions in POMs
5. **Mock external APIs** - Use `api-mocks.ts` for consistent testing

### Locator Priority

```typescript
// ✅ Best: Semantic roles
page.getByRole('button', { name: 'Submit' })

// ✅ Good: Labels
page.getByLabel('Email address')

// ⚠️  OK: Test IDs (when semantic not available)
page.getByTestId('submit-button')

// ❌ Avoid: CSS selectors
page.locator('.btn-primary')
```

## 🛠️ Utilities

### Test Data

```typescript
import { createMockFeed, testUsers } from '../utils/test-data';

// Use predefined test users
const user = testUsers.validUser;

// Generate mock data
const feed = createMockFeed({ title: 'My Test Feed' });
```

### API Mocking

```typescript
import { mockFeedsApi, mockEmptyFeeds } from '../utils/api-mocks';

test('should handle empty feeds', async ({ page }) => {
  await mockEmptyFeeds(page);
  await page.goto('/desktop/feeds');
  await expect(page.getByText(/no feeds/i)).toBeVisible();
});
```

### Accessibility Testing

```typescript
import { checkPageA11y } from '../utils/accessibility';

test('should be accessible', async ({ page }) => {
  await page.goto('/desktop/feeds');
  await checkPageA11y(page, { level: 'AA' });
});
```

### Performance Testing

```typescript
import { measureWebVitals, assertWebVitals } from '../utils/performance';

test('should meet Core Web Vitals', async ({ page }) => {
  await page.goto('/desktop/feeds');
  const metrics = await measureWebVitals(page);
  assertWebVitals(metrics);
});
```

## 🎯 Test Projects

Playwright is configured with multiple projects in `playwright.config.ts`:

| Project | Purpose | Authentication | Test Match |
|---------|---------|----------------|------------|
| `setup` | Authentication setup | N/A | `tests/*.setup.ts` |
| `authenticated-chrome` | Authenticated tests (Chrome) | ✅ Yes | `e2e/authenticated/**/*.spec.ts` |
| `authenticated-firefox` | Authenticated tests (Firefox) | ✅ Yes | `e2e/authenticated/**/*.spec.ts` |
| `desktop-chrome` | Desktop tests | ✅ Yes | `e2e/desktop/**/*.spec.ts` |
| `auth-flow-chrome` | Auth flow tests (Chrome) | ❌ No | `e2e/auth/**/*.spec.ts` |
| `auth-flow-firefox` | Auth flow tests (Firefox) | ❌ No | `e2e/auth/**/*.spec.ts` |
| `error-scenarios` | Error handling tests | ❌ No | `e2e/errors/**/*.spec.ts` |
| `components` | Component tests | ✅ Yes | `e2e/components/**/*.spec.ts` |

## 🐛 Debugging

### Visual Debugging

```bash
# Run in headed mode
pnpm exec playwright test --headed

# Run with slow motion
pnpm exec playwright test --headed --slow-mo=1000

# Debug specific test
pnpm exec playwright test --debug e2e/auth/login.spec.ts
```

### Trace Viewer

Playwright automatically captures traces on test failures. View them with:

```bash
pnpm exec playwright show-report
# Click on failed test → View trace
```

### Screenshots and Videos

Failed tests automatically capture:
- Screenshots (on failure)
- Videos (on failure)
- Traces (on failure)

Find them in `test-results/` directory.

## ⚙️ Configuration

### Environment Variables

E2E tests use environment variables from `.env.test`:

```bash
# Playwright configuration
PLAYWRIGHT_BASE_URL=http://localhost:3010
PW_MOCK_PORT=4545
PW_APP_PORT=3010

# Test configuration
CI=false
```

### Playwright Config

Main configuration is in `playwright.config.ts` at the root:

```typescript
export default defineConfig({
  testDir: './',
  timeout: 30 * 1000,
  retries: process.env.CI ? 2 : 1,
  workers: process.env.CI ? 2 : 10,
  // ... more config
});
```

## 📊 Coverage

### Test Coverage Goals

- **Page Coverage**: 95%+ of user-facing pages
- **User Flows**: All critical user journeys
- **Error Scenarios**: Common error states
- **Accessibility**: WCAG 2.1 AA compliance
- **Performance**: Core Web Vitals thresholds

### Current Coverage

| Category | Pages Tested | Coverage |
|----------|--------------|----------|
| Authentication | 2/5 | 40% |
| Desktop | 0/8 | 0% |
| Mobile | 0/7 | 0% |
| Public | 0/1 | 0% |
| E2E Flows | 0/3 | 0% |

## 🔄 CI/CD Integration

Tests run automatically in CI/CD pipeline via GitHub Actions.

### Running in CI

```bash
# CI mode (2 retries, 2 workers)
CI=true pnpm test:e2e
```

### Parallel Execution

Tests are configured for parallel execution:
- **Local**: 10 workers
- **CI**: 2 workers

## 📚 Resources

- [Playwright Documentation](https://playwright.dev/docs/intro)
- [Playwright Best Practices](https://playwright.dev/docs/best-practices)
- [Page Object Model Guide](https://playwright.dev/docs/pom)
- [Accessibility Testing](https://playwright.dev/docs/accessibility-testing)
- [Web Vitals](https://web.dev/vitals/)

## 🤝 Contributing

### Adding New Tests

1. Create Page Object in `page-objects/[category]/`
2. Extend `BasePage` class
3. Use semantic locators (prefer `getByRole`)
4. Create spec file in `specs/[category]/`
5. Add appropriate project in `playwright.config.ts` if needed

### Test Naming Convention

```typescript
// ✅ Good
test('should display feeds list')
test('should navigate to add feed page')

// ❌ Bad
test('test1')
test('feeds')
```

### Updating Tests

1. Run tests locally before committing
2. Update snapshots if UI changed: `pnpm exec playwright test --update-snapshots`
3. Ensure all tests pass
4. Check HTML report for detailed results

## 💡 Tips

- Use `page.pause()` to pause execution and inspect
- Use `test.only()` to run single test during development
- Use `test.skip()` to skip tests temporarily
- Check console logs with `page.on('console', msg => console.log(msg.text()))`
- Enable verbose logging: `DEBUG=pw:api pnpm test:e2e`

## 🆘 Troubleshooting

### Common Issues

**Tests timing out**
- Increase timeout in test or config
- Check if backend services are running
- Verify network connectivity

**Authentication failures**
- Ensure mock auth service is running
- Check storage state file exists: `playwright/.auth/user.json`
- Verify setup project ran successfully

**Flaky tests**
- Use auto-waiting instead of fixed waits
- Ensure proper test isolation
- Check for race conditions

**Element not found**
- Verify locator strategy
- Check if element exists in DOM
- Use `--headed` mode to visually inspect

## 📞 Support

For issues or questions:
1. Check this README
2. Review Playwright documentation
3. Check existing test examples
4. Ask in team chat

---

**Last Updated**: 2025-10-09
**Version**: 1.0
