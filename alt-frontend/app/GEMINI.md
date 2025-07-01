# GEMINI.md: alt-frontend Application

This document outlines the best practices and architectural standards for the `alt-frontend` Next.js application, following Gemini best practices as of July 2025. This application serves as the mobile-first frontend for the Alt RSS reader.

## 1. Core Philosophy

*   **Test-Driven Development (TDD)**: Every feature must begin with a test. No exceptions.
*   **Minimal Testing Strategy**: Avoid excessive tests. Focus on comprehensive tests that cover core functionality.

## 2. Testing Standards

### 2.1. Testing Architecture

*   **UI Components**: Use **Playwright** for E2E and integration testing (e.g., `FeedCard.spec.ts`).
*   **Business Logic**: Use **Vitest** for unit testing (e.g., `feedsApi.test.ts`).

### 2.2. Test File Organization

Tests should be located alongside the code they are testing.

```
components/
├── FeedCard/
│   ├── FeedCard.tsx
│   ├── FeedCard.spec.ts         # Playwright UI tests
│   └── FeedCard.logic.test.ts   # Vitest logic tests
```

### 2.3. Test Consolidation

Avoid creating numerous, small tests. Instead, consolidate tests into comprehensive scenarios.

*   **One comprehensive test** for main functionality (rendering, styling, interaction, accessibility).
*   **One responsive test** for mobile and desktop viewports.
*   A maximum of **3 tests per component** is recommended.

## 3. TypeScript and React

### 3.1. TypeScript Standards

*   **Strict Mode**: Always enable `strict` mode in `tsconfig.json`.
*   **Interfaces for Props**: Prefer interfaces over type aliases for component props.
*   **Utility Types**: Leverage built-in utility types like `Partial`, `Pick`, and `Required`.

### 3.2. React Best Practices (React 19+)

*   **`useActionState`**: Use for form handling to manage pending states and responses.
*   **`useOptimistic`**: Use for optimistic UI updates to improve user experience.
*   **`use()` API**: Use for data fetching within Suspense boundaries.
*   **Custom Hooks**: Extract complex or reusable logic into custom hooks.

## 4. Next.js Architecture

### 4.1. Data Fetching

*   Use custom hooks (e.g., `useFeeds`) for client-side data fetching.
*   Use API Routes for backend-for-frontend (BFF) logic.

### 4.2. File Organization

```
frontend/app/
├── pages/         # Routes
├── components/    # React components
├── lib/           # Utilities and API clients
├── hooks/         # Custom React hooks
├── schema/        # TypeScript types
└── styles/        # Global and component styles
```

## 5. State Management

*   **Local State**: Use `useState` for component-specific state.
*   **Server State**: Use a library like React Query or SWR for managing server data.
*   **URL State**: Use the URL for state that needs to be shareable.
*   **Context**: Use the Context API for state that needs to be shared across components.

## 6. Performance

*   **Code Splitting**: Use dynamic imports for large components.
*   **Image Optimization**: Use the `next/image` component for optimized image loading.
*   **Memoization**: Rely on the React 19 compiler for automatic memoization where possible.

## 7. Gemini Model Interaction

*   **TDD First**: Instruct Gemini to write tests before implementing the component.
*   **Break Down Tasks**: For complex components, break down the implementation into smaller, verifiable steps.
*   **Prevent Low-Quality Code**: Use "think harder" prompts for complex problems and provide clear constraints.