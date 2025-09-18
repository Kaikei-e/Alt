# CLAUDE.md - Alt Frontend

## About This Application

The Alt frontend is a modern, mobile-first web application built with **Next.js 15** and **TypeScript**. It leverages the App Router for routing and state management and is designed to provide a responsive, accessible, and performant user experience. The project adheres to a strict **Test-Driven Development (TDD)** methodology.

- **Framework**: Next.js 15 (App Router)
- **Language**: TypeScript
- **Package Manager**: pnpm
- **Testing**: Vitest, React Testing Library, Playwright (for E2E)
- **State Management**: React Hooks + Context API
- **Styling**: CSS variables and a responsive design approach

## Test-Driven Development (TDD) First

TDD is not optional; it is the core of our development process. All new features and bug fixes must start with a failing test.

### The TDD Cycle: Red-Green-Refactor

1.  **Red**: Write a failing test that defines the desired functionality. This test will fail because the implementation does not yet exist.
2.  **Green**: Write the **minimum** amount of code necessary to make the test pass.
3.  **Refactor**: Improve the code's structure and readability without changing its external behavior. All tests must continue to pass.

### TDD Workflow in Action

**1. RED: Write a failing test for a new component.**

```tsx
// src/components/Button/Button.test.tsx
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import Button from "./Button"; // This import will fail

describe("Button", () => {
  it("should render with children", () => {
    render(<Button>Click Me</Button>);
    expect(
      screen.getByRole("button", { name: /click me/i }),
    ).toBeInTheDocument();
  });
});
```

**2. GREEN: Write the minimal component code to pass the test.**

```tsx
// src/components/Button/Button.tsx
import React from "react";

export default function Button({ children }: { children: React.ReactNode }) {
  return <button>{children}</button>;
}
```

**3. REFACTOR: Improve the component's implementation.**

```tsx
// src/components/Button/Button.tsx
import React from "react";

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  children: React.ReactNode;
}

export default function Button({ children, ...props }: ButtonProps) {
  const baseStyle =
    "px-4 py-2 font-semibold text-white bg-blue-500 rounded hover:bg-blue-600";
  return (
    <button className={baseStyle} {...props}>
      {children}
    </button>
  );
}
```

## Testing Strategy

### Component Testing

- **Client Components & Hooks**: Test these with `vitest` and `React Testing Library`. Use `user-event` to simulate user interactions. Test custom hooks with `renderHook`.
- **Server Components (Async)**: Due to limitations in `jsdom`, async Server Components should be tested with **E2E tests** (Playwright) to ensure they render correctly in a real browser environment.
- **UI Components**: Focus on testing behavior from a user's perspective. Assert that the component renders correctly and responds to user interactions as expected.

### API Route Handlers

Test API routes (Route Handlers) in a Node.js environment. You can call the exported functions directly with a mocked `Request` object.

```tsx
// app/api/items/route.test.ts
/**
 * @vitest-environment node
 */
import { GET } from "./route";
import { describe, it, expect } from "vitest";

describe("GET /api/items", () => {
  it("should return a list of items", async () => {
    const response = await GET();
    const body = await response.json();
    expect(response.status).toBe(200);
    expect(body).toEqual([{ id: 1, name: "Item 1" }]);
  });
});
```

### Mocking Dependencies

- **`next/navigation`**: Mock the `useRouter` and `usePathname` hooks when testing components that use them.
- **API Calls**: Mock `fetch` or your data-fetching library to provide consistent responses and avoid actual network requests in tests.

## Development Environment

### Setup

1.  **Install Dependencies**: `pnpm install`
2.  **Configure Vitest**: Create a `vitest.config.ts` file.
3.  **Test Setup**: Create a `src/tests/setup.ts` file to import global test utilities like `@testing-library/jest-dom`.

### Common Commands

```bash
# Run unit tests
pnpm test

# Run unit tests in watch mode
pnpm test:watch

# Run E2E tests
pnpm test:e2e

# Lint and format code
pnpm lint && pnpm fmt

# Start the development server
pnpm dev
```

## References

- [Next.js Testing Documentation](https://nextjs.org/docs/testing)
- [Vitest Documentation](https://vitest.dev/)
- [React Testing Library](https://testing-library.com/docs/react-testing-library/intro/)
- [Playwright Documentation](https://playwright.dev/docs/intro)
