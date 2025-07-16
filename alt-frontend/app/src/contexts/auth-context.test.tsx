import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AuthProvider, useAuth } from './auth-context';
import { authAPI } from '@/lib/api/auth-client';
import type { User } from '@/types/auth';

// Mock the auth API client
vi.mock('@/lib/api/auth-client', () => ({
  authAPI: {
    getCurrentUser: vi.fn(),
    initiateLogin: vi.fn(),
    completeLogin: vi.fn(),
    initiateRegistration: vi.fn(),
    completeRegistration: vi.fn(),
    logout: vi.fn(),
  },
}));

// Test component to access auth context
function TestComponent() {
  const { user, isAuthenticated, isLoading, error, login, register, logout } = useAuth();

  const handleLogin = async () => {
    try {
      await login('test@example.com', 'password123');
    } catch {
      // Error is already handled by the auth context
    }
  };

  const handleRegister = async () => {
    try {
      await register('test@example.com', 'password123', 'Test User');
    } catch {
      // Error is already handled by the auth context
    }
  };

  const handleLogout = async () => {
    try {
      await logout();
    } catch {
      // Error is already handled by the auth context
    }
  };

  return (
    <div>
      <div data-testid="loading">{isLoading ? 'loading' : 'not-loading'}</div>
      <div data-testid="authenticated">{isAuthenticated ? 'authenticated' : 'not-authenticated'}</div>
      <div data-testid="user">{user ? user.email : 'no-user'}</div>
      <div data-testid="error">{error || 'no-error'}</div>
      <button onClick={handleLogin}>Login</button>
      <button onClick={handleRegister}>Register</button>
      <button onClick={handleLogout}>Logout</button>
    </div>
  );
}

describe('AuthContext', () => {
  const mockUser: User = {
    id: 'user-123',
    tenantId: 'tenant-456',
    email: 'test@example.com',
    role: 'user',
    createdAt: '2025-01-15T10:00:00Z',
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('AuthProvider', () => {
    it('should initialize with loading state and check authentication on mount', async () => {
      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(mockUser);

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      // Should start in loading state
      expect(screen.getByTestId('loading')).toHaveTextContent('loading');
      expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');

      // Should check authentication status
      await waitFor(() => {
        expect(authAPI.getCurrentUser).toHaveBeenCalled();
      });

      // Should update to authenticated state
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
        expect(screen.getByTestId('user')).toHaveTextContent('test@example.com');
      });
    });

    it('should handle unauthenticated state', async () => {
      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(null);

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
        expect(screen.getByTestId('user')).toHaveTextContent('no-user');
      });
    });

    it('should handle authentication check error', async () => {
      const errorMessage = 'Network error';
      vi.mocked(authAPI.getCurrentUser).mockRejectedValue(new Error(errorMessage));

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
        expect(screen.getByTestId('error')).toHaveTextContent(errorMessage);
      });
    });
  });

  describe('useAuth hook', () => {
    it('should throw error when used outside AuthProvider', () => {
      // Mock console.error to prevent error output in tests
      const consoleError = vi.spyOn(console, 'error').mockImplementation((message, ...args) => {
        console.log(message, args);
      });

      expect(() => {
        render(<TestComponent />);
      }).toThrow('useAuth must be used within an AuthProvider');

      consoleError.mockRestore();
    });

    it('should handle successful login flow', async () => {
      const user = userEvent.setup();
      const mockLoginFlow = {
        id: 'flow-123',
        ui: { action: '/login', method: 'POST', nodes: [] },
        expiresAt: '2025-01-15T11:00:00Z',
      };

      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(null);
      vi.mocked(authAPI.initiateLogin).mockResolvedValue(mockLoginFlow);
      vi.mocked(authAPI.completeLogin).mockResolvedValue(mockUser);

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      // Wait for initial load
      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      // Click login button
      const loginButton = screen.getByText('Login');
      await user.click(loginButton);

      // Should call login API methods
      await waitFor(() => {
        expect(authAPI.initiateLogin).toHaveBeenCalled();
        expect(authAPI.completeLogin).toHaveBeenCalledWith('flow-123', 'test@example.com', 'password123');
      });

      // Should update to authenticated state
      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
        expect(screen.getByTestId('user')).toHaveTextContent('test@example.com');
      });
    });

    it('should handle login error', async () => {
      const user = userEvent.setup();
      const errorMessage = 'Invalid credentials';

      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(null);
      vi.mocked(authAPI.initiateLogin).mockRejectedValue(new Error(errorMessage));

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      const loginButton = screen.getByText('Login');
      await user.click(loginButton);

      // Wait for the error to be displayed in the UI
      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent(errorMessage);
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
      }, { timeout: 5000 });

      // Ensure the error is properly handled and doesn't cause unhandled rejections
      await new Promise(resolve => setTimeout(resolve, 100));
    });

    it('should handle successful registration flow', async () => {
      const user = userEvent.setup();
      const mockRegistrationFlow = {
        id: 'flow-456',
        ui: { action: '/register', method: 'POST', nodes: [] },
        expiresAt: '2025-01-15T11:00:00Z',
      };

      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(null);
      vi.mocked(authAPI.initiateRegistration).mockResolvedValue(mockRegistrationFlow);
      vi.mocked(authAPI.completeRegistration).mockResolvedValue(mockUser);

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      const registerButton = screen.getByText('Register');
      await user.click(registerButton);

      await waitFor(() => {
        expect(authAPI.initiateRegistration).toHaveBeenCalled();
        expect(authAPI.completeRegistration).toHaveBeenCalledWith(
          'flow-456',
          'test@example.com',
          'password123',
          'Test User'
        );
      });

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
        expect(screen.getByTestId('user')).toHaveTextContent('test@example.com');
      });
    });

    it('should handle registration error', async () => {
      const user = userEvent.setup();
      const errorMessage = 'Registration failed';

      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(null);
      vi.mocked(authAPI.initiateRegistration).mockRejectedValue(new Error(errorMessage));

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('loading')).toHaveTextContent('not-loading');
      });

      const registerButton = screen.getByText('Register');
      await user.click(registerButton);

      // Wait for the error to be displayed in the UI
      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent(errorMessage);
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
      }, { timeout: 5000 });

      // Ensure the error is properly handled and doesn't cause unhandled rejections
      await new Promise(resolve => setTimeout(resolve, 100));
    });

    it('should handle successful logout', async () => {
      const user = userEvent.setup();

      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(mockUser);
      vi.mocked(authAPI.logout).mockResolvedValue();

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      // Wait for initial authenticated state
      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      });

      const logoutButton = screen.getByText('Logout');
      await user.click(logoutButton);

      await waitFor(() => {
        expect(authAPI.logout).toHaveBeenCalled();
      });

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('not-authenticated');
        expect(screen.getByTestId('user')).toHaveTextContent('no-user');
      });
    });

    it('should handle logout error', async () => {
      const user = userEvent.setup();
      const errorMessage = 'Logout failed';

      vi.mocked(authAPI.getCurrentUser).mockResolvedValue(mockUser);
      vi.mocked(authAPI.logout).mockRejectedValue(new Error(errorMessage));

      render(
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      );

      await waitFor(() => {
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      });

      const logoutButton = screen.getByText('Logout');
      await user.click(logoutButton);

      // Wait for the error to be displayed in the UI
      await waitFor(() => {
        expect(screen.getByTestId('error')).toHaveTextContent(errorMessage);
        // Should remain authenticated on logout failure
        expect(screen.getByTestId('authenticated')).toHaveTextContent('authenticated');
      }, { timeout: 5000 });

      // Ensure the error is properly handled and doesn't cause unhandled rejections
      await new Promise(resolve => setTimeout(resolve, 100));
    });
  });
});