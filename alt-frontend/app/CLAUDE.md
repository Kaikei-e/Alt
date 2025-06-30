# CLAUDE.md - alt-frontend

## Application Intent

This is a Next.js application that is a frontend for the alt-backend.
Mobile-first design and usage.

#### When Claude Generates Excessive Tests
```bash
# 1. Stop Claude immediately
claude "/clear"

# 2. Provide focused guidance
claude "You created too many UI tests. Our strategy is MINIMAL testing:

❌ REMOVE these types of tests:
- Separate tests for each CSS property
- Multiple similar interaction tests
- Individual loading/error state tests
- Separate color/size variation tests

✅ CONSOLIDATE into:
- ONE comprehensive test covering main functionality
- ONE responsive test (mobile + desktop)
- Maximum 3 tests per component

Refactor the existing tests following this minimal strategy."

# 3. Verify test count
find components -name "*.spec.ts" -exec grep -l "test(" {} \; | xargs grep -c "test("
# Should be ≤3 tests per component file
```

#### Emergency Test Cleanup
```typescript
// Template for Claude to consolidate over-testing
const CONSOLIDATION_PROMPT = `
You've created too many tests. Consolidate them using this pattern:

// ❌ BEFORE: 8 separate tests
test('should render')
test('should have glass styling')
test('should handle click')
test('should show loading')
test('should handle error')
test('should be accessible')
test('should work on mobile')
test('should work on desktop')

// ✅ AFTER: 2 comprehensive tests
test('should render and function correctly', async ({ page }) => {
  // Covers: rendering + styling + interaction + accessibility
});

test('should work across viewports', async ({ page }) => {
  // Covers: mobile + desktop responsive behavior
});

Apply this consolidation pattern to reduce test count.`;
```### Testing Standards

### Testing Architecture (2025) with Claude Code Safety

#### Test Strategy: UI vs Logic Separation
```
UIコンポーネント → Playwright (E2E/Integration) eg. FeedCard.tsx is FeedCard.spec.ts
ビジネスロジック → Vitest (Unit) eg. feedsApi.ts is feedsApi.test.ts
```

#### Test File Location and Organization
```
// UI Component tests - PLAYWRIGHT REQUIRED
components/
├── FeedCard/
│   ├── FeedCard.tsx
│   ├── FeedCard.test.ts         # Playwright E2E tests - PROTECTED
│   ├── FeedCard.logic.test.ts   # Vitest unit tests for logic - PROTECTED
│   ├── FeedCard.types.ts        # Type definitions
│   └── FeedCard.module.css      # Styles

// Business Logic tests - VITEST REQUIRED
lib/
├── api/
│   ├── feeds.ts
│   └── feeds.test.ts            # Vitest unit tests - PROTECTED
├── utils/
│   ├── formatters.ts
│   └── formatters.test.ts       # Vitest unit tests - PROTECTED
```

#### Claude Code Testing Workflow
```typescript
// 1. UI Component Testing with Playwright
claude "I need E2E tests for the FeedCard component using Playwright.
RULES:
- Use Playwright for ALL UI component testing
- Test user interactions and visual behavior
- Include accessibility testing with Playwright
- Cover responsive design scenarios
- DO NOT use React Testing Library for UI components
- Mark tests as PROTECTED from modification"

// 2. Business Logic Testing with Vitest
claude "I need unit tests for the feedsApi module using Vitest.
RULES:
- Use Vitest for ALL business logic testing
- Test pure functions and API interactions
- Mock external dependencies
- Cover edge cases and error handling
- DO NOT test UI rendering with Vitest
- Mark tests as PROTECTED from modification"
```

#### Playwright for UI Components (MANDATORY)
```typescript
// ✅ Playwright for UI component testing - REQUIRED
import { test, expect } from '@playwright/test';

// PROTECTED UI COMPONENT TESTS - CLAUDE: DO NOT MODIFY
test.describe('FeedCard Component - PROTECTED', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/components/feed-card');
  });

  test('should render with glass effect styling (PROTECTED)', async ({ page }) => {
    const feedCard = page.locator('[data-testid="feed-card"]');

    await expect(feedCard).toBeVisible();
    await expect(feedCard).toHaveClass(/glass/);

    // Verify glassmorphism visual properties
    const styles = await feedCard.evaluate(el => getComputedStyle(el));
    expect(styles.backdropFilter).toContain('blur');
  });

  test('should handle read status interaction (PROTECTED)', async ({ page }) => {
    const markReadButton = page.locator('button', { hasText: 'Mark as Read' });

    await expect(markReadButton).toBeVisible();
    await markReadButton.click();

    // Verify UI state change
    await expect(markReadButton).toHaveText('Read');
    await expect(markReadButton).toBeDisabled();
  });

  test('should be accessible (PROTECTED)', async ({ page }) => {
    // Accessibility# CLAUDE.md - TDD-First Coding Standards
*Version 1.0 - June 2025*

## Table of Contents
1. [Core Philosophy](#core-philosophy)
2. [Test-Driven Development Process](#test-driven-development-process)
3. [TypeScript Standards](#typescript-standards)
4. [React Best Practices](#react-best-practices)
5. [Next.js Architecture](#nextjs-architecture)
6. [Testing Standards](#testing-standards)
7. [Component Development](#component-development)
8. [State Management](#state-management)
9. [Performance Guidelines](#performance-guidelines)
10. [Code Quality](#code-quality)
11. [Security Practices](#security-practices)
12. [Documentation](#documentation)
13. [Playwright MCP Usage Rules](#playwright-mcp-usage-rules)

---

## Core Philosophy

### TDD as Default
Every feature begins with a test. No exceptions. Tests drive design decisions and ensure maintainability from day one.

### Principles
1. **Red-Green-Refactor**: The sacred cycle
2. **YAGNI**: You Aren't Gonna Need It
3. **KISS**: Keep It Simple, Stupid
4. **DRY**: Don't Repeat Yourself (but don't over-abstract)
5. **Fail Fast**: Surface errors immediately

---

## Test-Driven Development Process

### The TDD Cycle

#### 1. Red Phase - Write Failing Test
```typescript
// ❌ Start with a failing test
describe('StatsCard', () => {
  it('should display feed count', () => {
    const { getByText } = render(<StatsCard value={42} label="Total Feeds" />);
    expect(getByText('42')).toBeInTheDocument();
    expect(getByText('Total Feeds')).toBeInTheDocument();
  });
});
```

#### 2. Green Phase - Minimal Implementation
```typescript
// ✅ Write just enough code to pass
export const StatsCard = ({ value, label }: StatsCardProps) => {
  return (
    <div>
      <span>{value}</span>
      <span>{label}</span>
    </div>
  );
};
```

#### 3. Refactor Phase - Improve Design
```typescript
// 🔧 Refactor while tests stay green
export const StatsCard = ({ value, label, icon, description }: StatsCardProps) => {
  return (
    <Box className="glass" borderRadius="18px" p={5}>
      <VStack align="start" spacing={3}>
        <HStack>
          {icon && <Icon as={icon} />}
          <Text fontSize="sm">{label}</Text>
        </HStack>
        <Text fontSize="3xl" fontWeight="bold">{value}</Text>
        {description && <Text fontSize="sm">{description}</Text>}
      </VStack>
    </Box>
  );
};
```

### TDD Rules

1. **Write the test first** - Always
2. **One test at a time** - Focus on single behavior
3. **Minimal code** - Just enough to pass
4. **Refactor immediately** - Don't accumulate tech debt
5. **All tests must pass** - Before moving on

### Claude Code Quality Control

#### Preventing Low-Quality Code Generation
When using Claude Code with this TDD approach, follow these safeguards:

```bash
# Always start with test-first instruction
claude "Write tests for the FeedCard component first.
Do NOT implement the component yet.
Ensure tests fail initially to verify TDD cycle."

# Use extended thinking for complex problems
claude "Think harder about the edge cases for user authentication.
Write comprehensive tests covering:
- Valid login scenarios
- Invalid credentials
- Network failures
- Session expiration"

# Break down complex tasks
claude "Refactor the FeedCard component in these steps:
1. First, add tests for existing behavior
2. Then extract the read status logic
3. Finally, optimize the rendering
Do ONE step at a time and verify tests pass after each."
```

#### Protecting Existing Functionality
```markdown
## CLAUDE.md Configuration for Quality Control

### Code Generation Rules
- NEVER modify existing test files when fixing bugs
- ALWAYS run the full test suite before and after changes
- Use explicit TDD instructions to prevent implementation-first coding
- Require test coverage for all new features (minimum 80%)

### Quality Gates
- All generated code must pass TypeScript strict mode
- All generated code must pass existing linting rules
- Breaking changes require explicit approval and documentation
- Performance regressions must be identified and addressed

### Context Management
- Use /clear command between major refactoring sessions
- Provide essential project context through this CLAUDE.md file
- Limit scope to single components or features per session
```

#### Error Recovery Procedures
```bash
# Safety backup before major changes
git add . && git commit -m "Backup before Claude refactoring"

# Verification workflow
claude "Implement the failing tests, but first:
1. Confirm you understand the test requirements
2. Run tests to verify they fail
3. Implement minimal code to pass
4. Run full test suite to ensure no regressions"

# If Claude breaks existing functionality
git reset --hard HEAD^  # Rollback
claude "The tests are now failing. Fix only the failing tests
without modifying the test files themselves.
Explain what went wrong in the previous attempt."
```

---

## TypeScript Standards

### Type Safety First

#### Always Use Strict Mode
```typescript
// tsconfig.json
{
  "compilerOptions": {
    "strict": true,
    "noImplicitAny": true,
    "strictNullChecks": true,
    "strictFunctionTypes": true,
    "noImplicitThis": true,
    "alwaysStrict": true
  }
}
```

#### Prefer Interfaces for Component Props
```typescript
// ✅ Good
interface ButtonProps {
  variant: 'primary' | 'secondary';
  size?: 'sm' | 'md' | 'lg';
  onClick: () => void;
  children: React.ReactNode;
}

// ❌ Avoid type aliases for props
type ButtonProps = { ... }
```

#### Use Const Assertions
```typescript
// ✅ Immutable constants
const ROUTES = {
  HOME: '/',
  FEEDS: '/feeds',
  STATS: '/mobile/feeds/stats',
} as const;

type Route = typeof ROUTES[keyof typeof ROUTES];
```

### Advanced TypeScript Patterns (2025)

#### Mapped Types for Transformation
```typescript
// ✅ Transform existing types
type User = { name: string; age: number; email: string };
type UserReadOnly = { readonly [K in keyof User]: User[K] };
type UserPartial = { [K in keyof User]?: User[K] };
```

#### Template Literal Types
```typescript
// ✅ Dynamic type creation
type Color = "red" | "green" | "blue";
type ColorCode = `${Color}-color`; // 'red-color' | 'green-color' | 'blue-color'
```

#### Conditional Types for Smart APIs
```typescript
// ✅ Context-aware types
type IsString<T> = T extends string ? "yes" : "no";
type AsyncState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: Error };
```

#### Utility Types for Better DX
```typescript
// ✅ Leverage built-in utilities
type UserUpdate = Partial<Pick<User, 'name' | 'email'>>;
type UserRequired = Required<User>;
type UserKeys = keyof User;
```

### Type Patterns

#### Generic Components
```typescript
interface ListProps<T> {
  items: T[];
  renderItem: (item: T) => React.ReactNode;
  keyExtractor: (item: T) => string;
}

export function List<T>({ items, renderItem, keyExtractor }: ListProps<T>) {
  return (
    <>
      {items.map((item) => (
        <div key={keyExtractor(item)}>{renderItem(item)}</div>
      ))}
    </>
  );
}
```

#### Discriminated Unions for State
```typescript
type AsyncState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: Error };
```

---

## React Best Practices

### React 19 Features Integration

#### New Hooks Adoption
```typescript
// ✅ useActionState for form handling
import { useActionState } from 'react';

function ContactForm() {
  const [state, submitAction, isPending] = useActionState(
    async (prevState, formData) => {
      try {
        await submitContactForm(formData);
        return { success: true, message: 'Form submitted!' };
      } catch (error) {
        return { success: false, error: error.message };
      }
    },
    { success: false }
  );

  return (
    <form action={submitAction}>
      <input name="email" type="email" required />
      <button type="submit" disabled={isPending}>
        {isPending ? 'Submitting...' : 'Submit'}
      </button>
      {state.error && <p className="error">{state.error}</p>}
    </form>
  );
}
```

#### useOptimistic for Better UX
```typescript
// ✅ Optimistic updates with useOptimistic
import { useOptimistic } from 'react';

function TodoList({ todos, addTodo }) {
  const [optimisticTodos, addOptimisticTodo] = useOptimistic(
    todos,
    (state, newTodo) => [...state, { ...newTodo, sending: true }]
  );

  async function handleAddTodo(formData) {
    const newTodo = { id: Date.now(), text: formData.get('text') };
    addOptimisticTodo(newTodo);
    await addTodo(newTodo);
  }

  return (
    <div>
      {optimisticTodos.map(todo => (
        <div key={todo.id} className={todo.sending ? 'pending' : ''}>
          {todo.text}
        </div>
      ))}
      <form action={handleAddTodo}>
        <input name="text" placeholder="Add todo..." />
        <button type="submit">Add</button>
      </form>
    </div>
  );
}
```

#### use() API for Data Fetching
```typescript
// ✅ New use() API for promises and context
import { use, Suspense } from 'react';

function UserProfile({ userPromise }) {
  const user = use(userPromise); // Suspends until promise resolves

  return (
    <div>
      <h1>{user.name}</h1>
      <p>{user.email}</p>
    </div>
  );
}

function App() {
  const userPromise = fetchUser(userId);

  return (
    <Suspense fallback={<div>Loading user...</div>}>
      <UserProfile userPromise={userPromise} />
    </Suspense>
  );
}
```

### Component Architecture

#### Function Components with Hooks (2025 Standard)
```typescript
// ✅ Modern function component pattern
import { useState, useEffect, useCallback } from 'react';
import { Box, Text, Button } from '@chakra-ui/react';
import { Feed } from '@/schema/feed';
import { feedsApi } from '@/lib/api';

interface FeedCardProps {
  feed: Feed;
  isReadStatus: boolean;
  setIsReadStatus: (status: boolean) => void;
}

export const FeedCard: FC<FeedCardProps> = ({
  feed,
  isReadStatus,
  setIsReadStatus
}) => {
  const [isLoading, setIsLoading] = useState(false);

  const handleReadStatus = useCallback(async (url: string) => {
    try {
      setIsLoading(true);
      await feedsApi.updateFeedReadStatus(url);
      setIsReadStatus(true);
    } catch (error) {
      console.error('Error updating feed read status', error);
    } finally {
      setIsLoading(false);
    }
  }, [setIsReadStatus]);

  if (isLoading) {
    return <Spinner size="lg" color="pink.400" />;
  }

  return (
    <Box className="glass" p={5} borderRadius="18px">
      <Text fontSize="lg" fontWeight="bold">{feed.title}</Text>
      <Text fontSize="sm" color="gray.500">{feed.description}</Text>
      <Button
        onClick={() => handleReadStatus(feed.url)}
        disabled={isReadStatus}
        size="sm"
        mt={3}
      >
        {isReadStatus ? 'Read' : 'Mark as Read'}
      </Button>
    </Box>
  );
};
```

#### Ref Handling (React 19 Simplified)
```typescript
// ✅ React 19: refs as props (no forwardRef needed)
interface InputProps {
  placeholder: string;
  ref?: React.RefObject<HTMLInputElement>;
}

function CustomInput({ placeholder, ref }: InputProps) {
  return <input ref={ref} placeholder={placeholder} />;
}

// Usage
function Parent() {
  const inputRef = useRef<HTMLInputElement>(null);

  return <CustomInput ref={inputRef} placeholder="Enter text..." />;
}
```

### Hooks Guidelines

#### Custom Hooks for Logic
```typescript
// ✅ Extract complex logic into custom hooks
export const useSSEConnection = (url: string) => {
  const [data, setData] = useState(null);
  const [error, setError] = useState<Error | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'error'>('connecting');

  useEffect(() => {
    const source = new EventSource(url);

    source.onmessage = (event) => {
      setData(JSON.parse(event.data));
      setStatus('connected');
    };

    source.onerror = (error) => {
      setError(new Error('SSE connection failed'));
      setStatus('error');
    };

    return () => source.close();
  }, [url]);

  return { data, error, status };
};
```

#### Rules of Hooks
1. Only call at top level
2. Only call from React functions
3. Consistent order across renders
4. Prefix with 'use'

---

## Next.js Architecture

### Pages Router Pattern (Production Focus)

#### Client-Side Data Fetching Approach
```typescript
// ✅ Pages Router with optimized data fetching
export default function FeedsPage() {
  const { feeds, isLoading, error } = useFeeds();

  if (isLoading) return <Spinner />;
  if (error) return <ErrorMessage error={error} />;

  return <FeedList feeds={feeds} />;
}
```

#### API Routes Pattern
```typescript
// pages/api/feeds/[id].ts
export default async function handler(
  req: NextApiRequest,
  res: NextApiResponse
) {
  const { id } = req.query;

  if (req.method === 'GET') {
    try {
      const feed = await getFeed(id as string);
      return res.status(200).json(feed);
    } catch (error) {
      return res.status(500).json({ error: 'Failed to fetch feed' });
    }
  }

  return res.status(405).json({ error: 'Method not allowed' });
}
```

### File Organization (2025 Structure)
```
frontend/app/
├── pages/                 # Next.js pages (Routes)
│   ├── index.tsx         # Home page
│   ├── feeds/            # Feed pages
│   ├── articles/         # Article pages
│   ├── mobile/           # Mobile-specific pages
│   │   └── feeds/
│   │       └── stats.tsx # Stats page
│   └── api/              # API routes
├── components/           # React components
│   ├── Feed/            # Feed-related components
│   ├── Article/         # Article components
│   └── common/          # Shared components
├── lib/                 # Libraries and utilities
│   ├── api.ts          # API client functions
│   └── utils.ts        # Helper functions
├── hooks/              # Custom React hooks
├── schema/             # TypeScript types/interfaces
├── styles/             # CSS and styling
│   └── globals.css     # Global styles
└── public/             # Static assets
```

### Data Fetching Patterns

#### Custom API Hook Pattern
```typescript
// hooks/useFeeds.ts
export const useFeeds = () => {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    const fetchFeeds = async () => {
      try {
        const data = await feedsApi.getAll();
        setFeeds(data);
      } catch (err) {
        setError(err as Error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchFeeds();
  }, []);

  return { feeds, isLoading, error };
};
```

#### API Client Pattern
```typescript
// lib/api/feeds.ts
export const feedsApi = {
  getAll: async (): Promise<Feed[]> => {
    const response = await fetch('/api/feeds');
    if (!response.ok) throw new Error('Failed to fetch feeds');
    return response.json();
  },

  updateReadStatus: async (url: string): Promise<void> => {
    const response = await fetch('/api/feeds/viewed-status', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    });
    if (!response.ok) throw new Error('Failed to update status');
  },
};
```

---

## Testing Standards

### Testing Architecture (2025)

#### Test File Location
```
// Tests live alongside components
components/
├── FeedCard/
│   ├── FeedCard.tsx
│   ├── FeedCard.test.tsx    # Unit tests
│   └── FeedCard.module.css
```

#### Modern Testing with Vitest & Testing Library
```typescript
// ✅ Vitest + React Testing Library
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChakraProvider } from '@chakra-ui/react';

const renderWithChakra = (ui: React.ReactElement) => {
  return render(
    <ChakraProvider>
      {ui}
    </ChakraProvider>
  );
};

describe('FeedCard', () => {
  it('should render with glass effect', () => {
    const mockFeed = { id: '1', title: 'Test', description: 'Test' };
    renderWithChakra(<FeedCard feed={mockFeed} />);

    expect(screen.getByText('Test')).toBeInTheDocument();
  });

  it('should handle read status update', async () => {
    const user = userEvent.setup();
    const onStatusChange = vi.fn();

    renderWithChakra(
      <FeedCard
        feed={mockFeed}
        isReadStatus={false}
        setIsReadStatus={onStatusChange}
      />
    );

    await user.click(screen.getByRole('button', { name: /mark as read/i }));
    expect(onStatusChange).toHaveBeenCalledWith(true);
  });
});
```

#### API Mocking Pattern
```typescript
// ✅ Mock the API module
vi.mock('@/lib/api', () => ({
  feedsApi: {
    getAll: vi.fn(() => Promise.resolve(mockFeeds)),
    updateFeedReadStatus: vi.fn(() => Promise.resolve()),
  },
}));
```

---

## Component Development

### Component Checklist

- [ ] Write failing test first
- [ ] Define TypeScript interface
- [ ] Implement minimal component
- [ ] Add accessibility attributes
- [ ] Handle loading/error states
- [ ] Add proper styling
- [ ] Document props
- [ ] Test edge cases
- [ ] Refactor for clarity

### Accessibility First

```typescript
// ✅ Accessible component
export const Button: FC<ButtonProps> = ({
  children,
  onClick,
  disabled,
  ariaLabel
}) => {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      aria-label={ariaLabel}
      aria-disabled={disabled}
      role="button"
      tabIndex={disabled ? -1 : 0}
    >
      {children}
    </button>
  );
};
```

---

## State Management

### State Hierarchy (2025)

1. **Local State**: useState for component-specific
2. **Server State**: React Query/SWR for server data
3. **URL State**: For shareable app state
4. **Context**: For cross-component sharing

### State Management Rules

```typescript
// ✅ Colocate state with usage
function Component() {
  const [localState, setLocalState] = useState(0);
  // State used only here
}

// ✅ Lift state when shared
function Parent() {
  const [sharedState, setSharedState] = useState(0);
  return (
    <>
      <ChildA state={sharedState} />
      <ChildB setState={setSharedState} />
    </>
  );
}

// ✅ Use URL for shareable state
const searchParams = useSearchParams();
const filter = searchParams.get('filter') || 'all';
```

### Server State with React Query
```typescript
// ✅ Server state management
import { useQuery } from '@tanstack/react-query';

export const useFeeds = () => {
  return useQuery({
    queryKey: ['feeds'],
    queryFn: () => feedsApi.getAll(),
    staleTime: 5 * 60 * 1000, // 5 minutes
  });
};
```

---

## Performance Guidelines

### Core Web Vitals Targets (2025)

- **LCP**: < 2.5s
- **FID**: < 100ms
- **CLS**: < 0.1
- **INP**: < 200ms

### Optimization Techniques

#### Code Splitting
```typescript
// ✅ Dynamic imports for heavy components
const HeavyComponent = dynamic(() => import('./HeavyComponent'), {
  loading: () => <Skeleton />,
  ssr: false,
});
```

#### Smart Memoization (React 19 Compiler Aware)
```typescript
// ✅ Let React 19 compiler handle optimization
// No need for manual useMemo/useCallback in most cases
function ExpensiveComponent({ data }) {
  // React 19 compiler optimizes automatically
  const result = computeExpensiveValue(data);

  return <div>{result}</div>;
}
```

#### Image Optimization
```typescript
import Image from 'next/image';

// ✅ Use Next.js Image with priority
<Image
  src="/hero.png"
  alt="Hero image"
  width={800}
  height={400}
  priority
  placeholder="blur"
/>
```

---

## Code Quality

### Linting & Formatting (2025)

#### ESLint Configuration
```json
{
  "extends": [
    "next/core-web-vitals",
    "plugin:@typescript-eslint/recommended-type-checked",
    "plugin:react-hooks/recommended"
  ],
  "rules": {
    "@typescript-eslint/no-unused-vars": "error",
    "@typescript-eslint/no-floating-promises": "error",
    "react/no-array-index-key": "warn",
    "no-console": ["warn", { "allow": ["warn", "error"] }]
  }
}
```

#### Prettier Settings
```json
{
  "semi": true,
  "singleQuote": true,
  "tabWidth": 2,
  "trailingComma": "es5",
  "printWidth": 100
}
```

### Git Commit Convention
```
feat: add React 19 useActionState integration
fix: resolve memory leak in useSSEConnection
test: add coverage for optimistic updates
refactor: extract animation logic to hook
docs: update component API documentation
```

---

## Security Practices

### Input Validation
```typescript
// ✅ Validate all inputs
const userSchema = z.object({
  email: z.string().email(),
  age: z.number().min(0).max(120),
});

function validateUser(data: unknown) {
  return userSchema.parse(data);
}
```

### API Security
```typescript
// ✅ Secure API routes
export default async function handler(req: NextApiRequest, res: NextApiResponse) {
  // Validate method
  if (req.method !== 'POST') {
    return res.status(405).json({ error: 'Method not allowed' });
  }

  // Validate auth
  const token = req.headers.authorization?.replace('Bearer ', '');
  if (!token || !validateToken(token)) {
    return res.status(401).json({ error: 'Unauthorized' });
  }

  // Validate input
  try {
    const validatedData = schema.parse(req.body);
    // Process request...
  } catch (error) {
    return res.status(400).json({ error: 'Invalid input' });
  }
}
```

---

## Documentation

### Component Documentation
```typescript
/**
 * StatsCard - Displays a statistic with icon and description
 *
 * @example
 * <StatsCard
 *   icon={FiRss}
 *   label="Total Feeds"
 *   value={42}
 *   description="Active RSS feeds"
 * />
 */
interface StatsCardProps {
  /** Icon component to display */
  icon?: React.ComponentType;
  /** Label for the statistic */
  label: string;
  /** Numeric value to display */
  value: number;
  /** Optional description text */
  description?: string;
}
```

### README Structure
```markdown
# Component Name

## Overview
Brief description of component purpose

## Usage
\`\`\`tsx
import { Component } from '@/components/Component';

<Component prop="value" />
\`\`\`

## Props
| Prop | Type | Default | Description |
|------|------|---------|-------------|
| prop | string | - | Description |

## Examples
[Interactive examples]

## Testing
How to test this component
```

## Playwright MCP Usage Rules

### Absolute Prohibitions

1. **No code execution in any form**
   Disallow any browser or environment control via external scripts or subprocesses (e.g., Python, JavaScript, Bash, `subprocess` calls). ([modelcontextprotocol.io][1])

### Allowed Operations

2. **Only direct MCP tool invocations**
   Permit exclusively official Playwright MCP commands such as `playwright:browser_navigate` and `playwright:browser_screenshot`. ([en.wikipedia.org][3])

### Error Handling

3. **Immediate error reporting**
   On any failure, surface the error message verbatim—do not search for workarounds or execute alternative approaches. ([medium.com][4])

[1]: https://modelcontextprotocol.io/specification/draft/basic/security_best_practices?utm_source=chatgpt.com "Security Best Practices - Model Context Protocol"
[2]: https://medium.com/%40karlink/automating-web-testing-with-playwright-mcp-8d9424647817?utm_source=chatgpt.com "Automating Web Testing with Playwright MCP | by Karlin K - Medium"
[3]: https://en.wikipedia.org/wiki/Model_Context_Protocol?utm_source=chatgpt.com "Model Context Protocol"
[4]: https://medium.com/%40karlink/automating-web-testing-with-playwright-mcp-8d9424647817 "Automating Web Testing with Playwright MCP | by Karlin K | May, 2025 | Medium"


---

## Appendix: Quick Reference

### TDD Checklist with Claude Code Safety
- [ ] Write test describing desired behavior (Human defines requirements)
- [ ] **Use Playwright for UI components, Vitest for business logic**
- [ ] Run test and see it fail (Verify Claude understands correctly)
- [ ] Write minimal code to pass (Claude implements with constraints)
- [ ] Run test and see it pass (Automated verification)
- [ ] Refactor if needed (Claude improves while maintaining tests)
- [ ] All tests still pass (Final safety check)

### Claude Code Quality Checklist
- [ ] Use explicit TDD instructions
- [ ] Apply "think harder" for complex problems
- [ ] Break large tasks into smaller chunks
- [ ] **MANDATORY: Use Playwright for ALL UI component tests**
- [ ] **MANDATORY: Use Vitest for ALL business logic tests**
- [ ] **LIMIT: Maximum 3 tests per UI component**
- [ ] **CONSOLIDATE: Combine similar test scenarios**
- [ ] Protect test files from modification
- [ ] Verify all tests pass after changes
- [ ] Run full test suite, not just changed tests
- [ ] Check TypeScript compilation
- [ ] Verify no new lint errors
- [ ] Confirm no breaking changes
- [ ] Review generated code manually

### Anti-Over-Testing Guidelines
```bash
# Pre-commit hook: Test count enforcement
#!/bin/sh
echo "🧪 Checking for excessive UI testing..."

# Count tests per component
for file in components/**/*.spec.ts; do
  test_count=$(grep -c "test(" "$file" 2>/dev/null || echo 0)
  if [ "$test_count" -gt 3 ]; then
    echo "❌ Too many tests in $file ($test_count tests)"
    echo "💡 Consolidate into maximum 3 comprehensive tests"
    exit 1
  fi
done

echo "✅ Test count limits respected!"
```

### Testing Tool Enforcement
```bash
# Pre-commit hook enforcement
#!/bin/sh
echo "🧪 Verifying testing tool compliance..."

# Check for UI component tests using wrong tool
if find components -name "*.test.tsx" | head -1 | grep -q .; then
  echo "❌ Found .test.tsx files in components directory"
  echo "💡 UI components must use Playwright (.spec.ts), not React Testing Library"
  exit 1
fi

# Check for business logic tests using wrong tool
if find lib -name "*.spec.ts" | head -1 | grep -q .; then
  echo "❌ Found .spec.ts files in lib directory"
  echo "💡 Business logic must use Vitest (.test.ts), not Playwright"
  exit 1
fi

# Run Playwright tests for UI components
echo "🎭 Running Playwright tests for UI components..."
npx playwright test
if [ $? -ne 0 ]; then
  echo "❌ Playwright E2E tests failed"
  exit 1
fi

# Run Vitest tests for business logic
echo "⚡ Running Vitest tests for business logic..."
pnpm run test:unit
if [ $? -ne 0 ]; then
  echo "❌ Vitest unit tests failed"
  exit 1
fi

echo "✅ All testing tool compliance checks passed!"
```

### React 19 Migration Checklist (Claude-Safe)
- [ ] Update to React 19 (Human approval required)
- [ ] Enable new JSX transform (Human configures)
- [ ] Replace forwardRef with ref props (Claude can automate)
- [ ] Adopt useActionState for forms (Claude implements with tests)
- [ ] Use useOptimistic for better UX (Claude adds with safety checks)
- [ ] Leverage React Compiler optimizations (Human enables, Claude adapts)

### Performance Checklist (Claude-Verified)
- [ ] Lazy load heavy components (Claude can implement)
- [ ] Optimize images with next/image (Claude can convert)
- [ ] Minimize client-side JavaScript (Claude can analyze)
- [ ] Use Server Components when possible (Human architecture decision)
- [ ] Profile with React DevTools (Human verification required)

### Security Checklist (Human-Verified)
- [ ] Validate all inputs (Claude can implement, human verifies)
- [ ] Sanitize user content (Human reviews implementation)
- [ ] Use HTTPS everywhere (Infrastructure decision)
- [ ] Implement proper auth (Human designs, Claude implements)
- [ ] Keep dependencies updated (Automated + human oversight)

#### Quality Recovery Workflow
```typescript
// Use this template to recover from Claude errors
const RECOVERY_PROMPT = `
The previous code generation had these issues:
- [List specific problems]

Please regenerate following these strict guidelines:
1. Use our existing TypeScript interfaces
2. Follow our established patterns in [similar component]
3. Maintain backward compatibility
4. Add proper error boundaries
5. Include comprehensive tests
6. Think harder about the implementation approach

Do NOT:
- Modify existing test files
- Change component interfaces
- Skip error handling
- Use deprecated patterns
`;
```

---

*This document evolves with our development practices and Claude Code capabilities. Always prioritize human judgment over AI suggestions, especially for critical system components and security-sensitive code.*