# alt-frontend/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->
<!-- Use 'think' for basic analysis, 'ultrathink' for complex architectural decisions -->

## About alt-frontend

> For an up-to-date implementation summary (routing tree, testing matrix, build tooling), read `docs/alt-frontend.md`.

This is the **frontend service** of the Alt RSS reader platform, built with **Next.js 15**, **React 19**, **TypeScript 5.9**, and **Chakra UI**. The service follows Test-Driven Development (TDD) and implements modern React patterns with server-side rendering capabilities.

**Critical Guidelines:**

- **TDD First:** Always write failing tests BEFORE implementation
- **Type Safety:** Use TypeScript strictly, avoid `any` types
- **Performance:** Optimize for Core Web Vitals and user experience
- **Accessibility:** Follow WCAG 2.1 AA guidelines
- **Responsive Design:** Mobile-first approach with Chakra UI

## Architecture Overview

### Next.js 15 App Router Architecture

```
app/
├─ layout.tsx          # Root layout with providers
├─ page.tsx           # Root page (redirects to /home)
├─ home/              # Main dashboard
├─ desktop/           # Desktop-specific pages
├─ public/            # Public landing pages
├─ api/               # API routes
└─ (auth)/            # Auth-protected routes
```

### Component Architecture

```
src/
├─ components/        # Reusable UI components
│  ├─ desktop/       # Desktop-specific components
│  ├─ forms/         # Form components
│  └─ layout/        # Layout components
├─ contexts/         # React contexts
├─ hooks/           # Custom hooks
├─ utils/           # Utility functions
├─ providers/       # Context providers
└─ middleware.ts    # Next.js middleware
```

## Technology Stack

### Core Technologies

- **Next.js 15**: App Router, Server Components, Streaming
- **React 19**: Latest React features, concurrent rendering
- **TypeScript 5.9**: Strict type checking, latest language features
- **Chakra UI**: Component library with custom theme system
- **Vitest**: Unit testing framework
- **Playwright**: End-to-end testing

### Theme System

The application uses a custom three-theme system:

- **Vaporwave**: Neon colors for modern aesthetic
- **Liquid-Beige**: Earthy luxury theme
- **Alt-Paper**: Newspaper-inspired theme

### Development Tools

- **ESLint**: Code linting with strict rules
- **Prettier**: Code formatting
- **TypeScript**: Type checking
- **Husky**: Git hooks for quality gates

## TDD and Testing Strategy

### Test-Driven Development (TDD)

All development follows the Red-Green-Refactor cycle:

1. **Red**: Write a failing test
2. **Green**: Write minimal code to pass
3. **Refactor**: Improve code quality

### Testing Layers

#### Unit Tests (Vitest)

Unit tests are located in `tests/unit/` and follow these patterns:

```typescript
// Example component test with mocked API
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { FeedDetails } from '@/components/mobile/FeedDetails'
import { feedsApi } from '@/lib/api'

vi.mock('@/lib/api', () => ({
  feedsApi: {
    getFeedContentOnTheFly: vi.fn(),
    archiveContent: vi.fn(),
  },
}))

describe('FeedDetails', () => {
  it('auto-archives article when displaying content', async () => {
    const user = userEvent.setup()

    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: 'Article content',
      url: 'https://example.com',
    })

    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: 'article archived',
    })

    render(<FeedDetails feedURL="https://example.com" feedTitle="Test" />)

    await user.click(screen.getByText('Show Details'))

    await waitFor(() => {
      expect(feedsApi.archiveContent).toHaveBeenCalledWith(
        'https://example.com',
        'Test',
      )
    })
  })
})
```

**Unit Test Guidelines:**

- Use `vi.mock()` for API and external dependencies
- Use `userEvent` for realistic user interactions
- Use `waitFor()` for async operations
- Test both success and failure paths
- Ensure non-blocking error handling doesn't affect UX

#### Integration Tests (Vitest)

```typescript
// Example API route test
import { createMocks } from "node-mocks-http";
import handler from "../api/feeds";

describe("/api/feeds", () => {
  it("returns feeds for authenticated user", async () => {
    const { req, res } = createMocks({
      method: "GET",
      headers: { authorization: "Bearer valid-token" },
    });

    await handler(req, res);

    expect(res._getStatusCode()).toBe(200);
    expect(JSON.parse(res._getData())).toHaveProperty("feeds");
  });
});
```

#### End-to-End Tests (Playwright)

E2E tests use the **Page Object Model (POM)** pattern for maintainability. Page Objects are located in `tests/pages/`.

```typescript
// Page Object Model (tests/pages/HomePage.ts)
import { Page, Locator } from "@playwright/test";
import { BasePage } from "./BasePage";

export class HomePage extends BasePage {
  readonly addFeedButton: Locator;
  readonly feedUrlInput: Locator;
  readonly submitButton: Locator;
  readonly feedList: Locator;

  constructor(page: Page) {
    super(page);
    // IMPORTANT: Always reference actual data-testid from implementation
    this.addFeedButton = page.getByTestId("add-feed-button");
    this.feedUrlInput = page.getByTestId("feed-url-input");
    this.submitButton = page.getByTestId("submit-button");
    this.feedList = page.getByTestId("feed-list");
  }

  async addFeed(url: string) {
    await this.addFeedButton.click();
    await this.feedUrlInput.fill(url);
    await this.submitButton.click();
  }
}

// E2E Test (tests/e2e/feeds.spec.ts)
import { test, expect } from "@playwright/test";
import { HomePage } from "../pages/HomePage";

test("user can add a new feed", async ({ page }) => {
  const homePage = new HomePage(page);
  await homePage.goto("/home");
  await homePage.addFeed("https://example.com/rss");

  await expect(homePage.feedList).toContainText("Example Feed");
});
```

**E2E Test Guidelines:**

- **Always use POM**: Create Page Objects in `tests/pages/` for all E2E tests
- **Verify Selectors**: Check actual `data-testid` values in component implementation before writing POM
- **Document Sources**: Add comments with file paths and line numbers for selectors
- **Extend BasePage**: Inherit from `BasePage` for common functionality
- **No Direct Selectors in Tests**: Use Page Object methods instead of raw selectors

### Development Workflow

1. **Start Development Server**: `pnpm dev`
2. **Run Tests**: `pnpm test` (unit), `pnpm test:e2e` (E2E)
3. **Lint and Format**: `pnpm lint`, `pnpm fmt`
4. **Type Check**: `pnpm type-check`
5. **Build**: `pnpm build`

## Component Patterns

### React 19 Patterns

- **Server Components**: Use for data fetching and static content
- **Client Components**: Use for interactivity and state management
- **Custom Hooks**: Extract reusable logic into custom hooks
- **Context API**: Use for global state management

### Article Management Patterns

#### Auto-Archive on Display

The `FeedDetails` component implements automatic article archiving when content is displayed:

```typescript
// When article content is successfully fetched and displayed
if (hasValidDetails) {
  setFeedDetails(details);

  // Auto-archive article to ensure DB persistence before summarization
  feedsApi.archiveContent(feedURL, feedTitle).catch((err) => {
    console.warn("Failed to auto-archive article:", err);
    // Don't block UI on archive failure
  });
}
```

**Key Design Decisions:**

1. **Timing**: Articles are archived immediately when displayed (not on user click)
2. **Non-blocking**: Archive failures are logged but don't prevent UI display
3. **Idempotency**: Backend handles duplicate archive requests gracefully
4. **Purpose**: Ensures articles exist in DB before AI summarization

**Why This Matters:**

- The `pre-processor` service requires articles to exist in the database before creating summaries
- The SQL query uses `WHERE EXISTS (SELECT 1 FROM articles WHERE id = $1)`
- Auto-archiving guarantees this precondition is met when users click "要約" (Summarize)
- Improves accuracy of "未要約記事数" (unsummarized article count) metrics

### Chakra UI Best Practices

- **Theme System**: Use theme tokens for consistent styling
- **Responsive Design**: Use Chakra's responsive props
- **Accessibility**: Leverage Chakra's built-in accessibility features
- **Custom Components**: Extend Chakra components when needed

### TypeScript Patterns

```typescript
// Strict typing for API responses
interface FeedResponse {
  id: string;
  title: string;
  description: string;
  url: string;
  lastUpdated: string;
}

// Generic API hook
function useApi<T>(endpoint: string): {
  data: T | null;
  loading: boolean;
  error: string | null;
} {
  // Implementation
}

// Component props with strict typing
interface FeedCardProps {
  feed: FeedResponse;
  onSelect: (feed: FeedResponse) => void;
  isSelected?: boolean;
}
```

## Performance Optimization

### Core Web Vitals

- **LCP**: Optimize images and critical resources
- **FID**: Minimize JavaScript execution time
- **CLS**: Prevent layout shifts with proper sizing

### Next.js Optimizations

- **Image Optimization**: Use `next/image` for automatic optimization
- **Code Splitting**: Leverage dynamic imports for route-based splitting
- **Caching**: Implement proper caching strategies
- **Streaming**: Use Suspense for progressive loading

## Security Considerations

### Authentication

- **Middleware**: Use Next.js middleware for route protection
- **Session Management**: Integrate with auth-hub for session validation
- **CSRF Protection**: Implement CSRF tokens for state-changing operations

### Data Protection

- **Input Validation**: Validate all user inputs
- **XSS Prevention**: Use React's built-in XSS protection
- **Content Security Policy**: Implement CSP headers

## API Integration

### Backend Communication

- **Base URL**: `http://localhost:9000` (development)
- **Authentication**: Session-based via auth-hub
- **Error Handling**: Centralized error handling with user-friendly messages

### API Routes

```typescript
// Example API route
export async function GET(request: Request) {
  try {
    const response = await fetch(`${process.env.BACKEND_URL}/v1/feeds`, {
      headers: {
        Authorization: `Bearer ${getToken(request)}`,
        "Content-Type": "application/json",
      },
    });

    if (!response.ok) {
      throw new Error("Failed to fetch feeds");
    }

    const data = await response.json();
    return Response.json(data);
  } catch (error) {
    return Response.json({ error: "Internal server error" }, { status: 500 });
  }
}
```

## Environment Configuration

### Required Environment Variables

```bash
# Backend API
NEXT_PUBLIC_BACKEND_URL=http://localhost:9000

# Authentication
NEXT_PUBLIC_AUTH_URL=http://localhost:8888

# Feature Flags
NEXT_PUBLIC_ENABLE_AI=true
NEXT_PUBLIC_ENABLE_ANALYTICS=false
```

## Deployment

### Docker Configuration

- **Base Image**: Node.js 20 Alpine
- **Port**: 3000
- **Health Check**: `/api/health`
- **Build**: Multi-stage build for optimization

### Production Considerations

- **Static Generation**: Use static generation where possible
- **CDN**: Configure CDN for static assets
- **Monitoring**: Integrate with observability stack
- **Security Headers**: Implement security headers

## Troubleshooting

### Common Issues

- **Build Failures**: Check TypeScript errors and dependencies
- **Runtime Errors**: Check browser console and server logs
- **Performance Issues**: Use Next.js built-in analytics
- **Authentication Issues**: Verify auth-hub integration

### Debug Commands

```bash
# Development server
pnpm dev

# Type checking
pnpm type-check

# Linting
pnpm lint

# Testing
pnpm test
pnpm test:e2e

# Build
pnpm build
```

## References

- [Next.js 15 Documentation](https://nextjs.org/docs)
- [React 19 Documentation](https://react.dev)
- [Chakra UI Documentation](https://chakra-ui.com)
- [Vitest Documentation](https://vitest.dev)
- [Playwright Documentation](https://playwright.dev)
