import type { ReactNode } from "react";
import { vi } from "vitest";

// Create a mock function that we'll control
const mockUseAuth = vi.fn();

// Mock the entire auth context module
vi.mock("@/contexts/auth-context", () => ({
  useAuth: mockUseAuth,
  AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

// Helper function to mock authenticated state
export const mockAuthenticatedState = () => {
  mockUseAuth.mockReturnValue({
    isAuthenticated: true,
    user: { id: "test-user", email: "test@example.com" },
    loading: false,
    error: null,
    login: vi.fn(),
    logout: vi.fn(),
    checkSession: vi.fn(),
  });
  return mockUseAuth;
};

// Helper function to mock unauthenticated state
export const mockUnauthenticatedState = () => {
  mockUseAuth.mockReturnValue({
    isAuthenticated: false,
    user: null,
    loading: false,
    error: null,
    login: vi.fn(),
    logout: vi.fn(),
    checkSession: vi.fn(),
  });
  return mockUseAuth;
};

// Helper function to mock loading state
export const mockLoadingState = () => {
  mockUseAuth.mockReturnValue({
    isAuthenticated: false,
    user: null,
    loading: true,
    error: null,
    login: vi.fn(),
    logout: vi.fn(),
    checkSession: vi.fn(),
  });
  return mockUseAuth;
};

// Test wrapper component for providing auth context
export const TestAuthProvider = ({
  children,
  authenticated = true,
}: {
  children: ReactNode;
  authenticated?: boolean;
}) => {
  if (authenticated) {
    mockAuthenticatedState();
  } else {
    mockUnauthenticatedState();
  }

  return <>{children}</>;
};
