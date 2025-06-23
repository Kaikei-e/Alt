# CODE_RULES.md - TDD-First Coding Standards
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

### Component Architecture

#### Function Components with Hooks
```typescript
// Alt pattern: Functional components with hooks
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
      {/* Component content */}
    </Box>
  );
};
```

#### CSS Modules + Tailwind + Chakra UI
```typescript
// Alt uses combination of styling approaches
import styles from './Component.module.css';

export const Component = () => {
  return (
    <Box
      className={`glass ${styles.customAnimation}`}  // CSS modules + global class
      bg="vaporwave.glass"                           // Chakra theme
      _hover={{ transform: 'translateY(-5px)' }}     // Chakra props
    >
      <Text className="vaporwave-text">Content</Text>
    </Box>
  );
};
```

### Hooks Guidelines

#### Custom Hooks for Logic
```typescript
// Extract complex logic into custom hooks
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

## Next.js Architecture (Alt Frontend Structure)

### Pages Router Pattern (Alt Implementation)

#### Client-Side First Approach
```typescript
// Alt uses Pages Router with client-side data fetching
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
    const feed = await getFeed(id);
    return res.status(200).json(feed);
  }

  return res.status(405).json({ error: 'Method not allowed' });
}
```

### Alt Frontend File Organization
```
alt-frontend/app/
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

### Data Fetching Patterns (Alt Style)

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
    const response = await fetch('/api/feeds/read-status', {
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

### Testing Standards (Alt Frontend Patterns)

#### Test File Location
```
// Tests live alongside components
components/
├── FeedCard/
│   ├── FeedCard.tsx
│   ├── FeedCard.test.tsx    # Unit tests
│   └── FeedCard.module.css
```

#### Testing with Chakra UI
```typescript
// Test with ChakraProvider wrapper
import { render } from '@testing-library/react';
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
    const { container } = renderWithChakra(
      <FeedCard feed={mockFeed} />
    );

    expect(container.querySelector('.glass')).toBeInTheDocument();
  });
});
```

#### API Mocking Pattern
```typescript
// Mock the API module used in Alt
jest.mock('@/lib/api', () => ({
  feedsApi: {
    getAll: jest.fn(() => Promise.resolve(mockFeeds)),
    updateFeedReadStatus: jest.fn(() => Promise.resolve()),
  },
}));
```

### State Management (Alt Patterns)

#### Local State for UI
```typescript
// Alt uses local state for component UI state
const [isLoading, setIsLoading] = useState(false);
const [isReadStatus, setIsReadStatus] = useState(false);
```

#### Custom Hooks for Data
```typescript
// Encapsulate data fetching in custom hooks
export const useFeeds = () => {
  const [feeds, setFeeds] = useState<Feed[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    feedsApi.getAll()
      .then(setFeeds)
      .finally(() => setIsLoading(false));
  }, []);

  return { feeds, isLoading };
};
```

#### SSE Connection Pattern
```typescript
// Server-Sent Events for real-time updates
export const useSSEStats = () => {
  const [stats, setStats] = useState({ feedCount: 0, unsummarizedCount: 0 });
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    const eventSource = new EventSource('/api/sse/stats');

    eventSource.onopen = () => setIsConnected(true);
    eventSource.onmessage = (event) => {
      setStats(JSON.parse(event.data));
    };
    eventSource.onerror = () => setIsConnected(false);

    return () => eventSource.close();
  }, []);

  return { ...stats, isConnected };
};
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

### State Hierarchy

1. **Local State**: useState for component-specific
2. **Context**: For cross-component sharing
3. **URL State**: For shareable app state
4. **Server Cache**: For server data (React Query/SWR)

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

---

## Performance Guidelines

### Core Web Vitals

Target metrics:
- **LCP**: < 2.5s
- **FID**: < 100ms
- **CLS**: < 0.1
- **INP**: < 200ms

### Optimization Techniques

#### Code Splitting
```typescript
// Dynamic imports for heavy components
const HeavyComponent = dynamic(() => import('./HeavyComponent'), {
  loading: () => <Skeleton />,
  ssr: false,
});
```

#### Memoization
```typescript
// ✅ Memoize expensive computations
const expensiveValue = useMemo(() => {
  return computeExpensiveValue(data);
}, [data]);

// ✅ Memoize callbacks
const handleClick = useCallback(() => {
  doSomething(id);
}, [id]);

// ❌ Don't over-memoize
const simpleValue = useMemo(() => a + b, [a, b]); // Unnecessary
```

#### Image Optimization
```typescript
import Image from 'next/image';

// ✅ Use Next.js Image
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

### Linting & Formatting

#### ESLint Configuration
```json
{
  "extends": [
    "next/core-web-vitals",
    "plugin:@typescript-eslint/recommended",
    "plugin:react-hooks/recommended"
  ],
  "rules": {
    "@typescript-eslint/no-unused-vars": "error",
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
feat: add SSE progress indicator
fix: resolve memory leak in useSSEConnection
test: add coverage for error states
refactor: extract animation logic to hook
docs: update component API documentation
```

### Alt Frontend Specific Patterns

#### Schema-Driven Development
```typescript
// Alt uses schema files for type definitions
// schema/feed.ts
export interface Feed {
  id: string;
  title: string;
  description: string;
  link: string;
  published: Date;
}

// schema/article.ts
export interface Article {
  id: string;
  feedId: string;
  title: string;
  content: string;
  summary?: string;
}
```

#### Glass Design System
```typescript
// Reuse the glass class throughout the app
<Box className="glass" borderRadius="18px" p={5}>
  {/* Content */}
</Box>

// CSS (globals.css)
.glass {
  background: var(--glass-bg);
  backdrop-filter: blur(10px);
  border: 1px solid var(--glass-border);
}
```

#### Vaporwave Button Pattern
```typescript
// Consistent button styling across Alt
<Button
  size="sm"
  borderRadius="full"
  bg="linear-gradient(45deg, #ff006e, #8338ec)"
  color="white"
  fontWeight="bold"
  border="1px solid rgba(255, 255, 255, 0.2)"
  _hover={{
    bg: "linear-gradient(45deg, #e6005c, #7129d4)",
    transform: "translateY(-1px)",
  }}
>
  Action
</Button>
```

#### Mobile-First Responsive
```typescript
// Alt prioritizes mobile experience
export default function MobileStatsPage() {
  return (
    <Box
      minH="100vh"
      p={4}
      // Mobile-specific padding with safe areas
      pb="calc(80px + env(safe-area-inset-bottom))"
    >
      {/* Mobile-optimized content */}
    </Box>
  );
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

---

## Appendix: Quick Reference

### TDD Checklist
- [ ] Write test describing desired behavior
- [ ] Run test and see it fail
- [ ] Write minimal code to pass
- [ ] Run test and see it pass
- [ ] Refactor if needed
- [ ] All tests still pass

### Performance Checklist
- [ ] Lazy load heavy components
- [ ] Optimize images with next/image
- [ ] Minimize client-side JavaScript
- [ ] Use Server Components when possible
- [ ] Profile with React DevTools

### Security Checklist
- [ ] Validate all inputs
- [ ] Sanitize user content
- [ ] Use HTTPS everywhere
- [ ] Implement proper auth
- [ ] Keep dependencies updated

---

*This is a living document. Update as new patterns emerge and tools evolve.*