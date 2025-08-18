import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { AuthAPIClient } from './auth-client';
import type { User, LoginFlow, RegistrationFlow } from '@/types/auth';

// Mock fetch globally
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Mock console methods to avoid noise in tests
const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

describe('AuthAPIClient', () => {
  let client: AuthAPIClient;

  beforeEach(() => {
    client = new AuthAPIClient();
    mockFetch.mockClear();
    consoleSpy.mockClear();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('constructor', () => {
    it('should initialize with default base URL', () => {
      const client = new AuthAPIClient();
      expect(client).toBeInstanceOf(AuthAPIClient);
    });

    it('should use environment variable for base URL if available', () => {
      const originalEnv = process.env.NEXT_PUBLIC_AUTH_SERVICE_URL;
      process.env.NEXT_PUBLIC_AUTH_SERVICE_URL = 'http://custom-auth:8080';
      
      const client = new AuthAPIClient();
      expect(client).toBeInstanceOf(AuthAPIClient);
      
      // Restore original env
      process.env.NEXT_PUBLIC_AUTH_SERVICE_URL = originalEnv;
    });
  });

  describe('initiateLogin', () => {
    it('should throw redirect error (current implementation)', async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, 'location', {
        value: { href: '' },
        writable: true,
      });

      await expect(client.initiateLogin()).rejects.toThrow('Login flow initiated via redirect');
    });
  });

  describe('completeLogin', () => {
    it('should throw redirect error (current implementation)', async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, 'location', {
        value: { href: '' },
        writable: true,
      });

      await expect(client.completeLogin('flow-123', 'test@example.com', 'password123'))
        .rejects.toThrow('Login redirected to Kratos');
    });
  });

  describe('initiateRegistration', () => {
    it('should throw redirect error (current implementation)', async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, 'location', {
        value: { href: '' },
        writable: true,
      });

      await expect(client.initiateRegistration()).rejects.toThrow('Registration flow initiated via redirect');
    });
  });

  describe('completeRegistration', () => {
    it('should throw redirect error (current implementation)', async () => {
      // Mock window.location for test environment
      Object.defineProperty(window, 'location', {
        value: { href: '' },
        writable: true,
      });

      await expect(client.completeRegistration('flow-456', 'newuser@example.com', 'password123', 'New User'))
        .rejects.toThrow('Registration redirected to Kratos');
    });
  });

  describe('logout', () => {
    it('should make POST request to logout endpoint', async () => {
      // Mock CSRF token request first
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({ data: { csrf_token: 'csrf-token-123' } }),
        })
        // Mock actual logout request
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({}),
        });

      await client.logout();

      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });

  describe('getCurrentUser', () => {
    it('should make GET request and return User when authenticated', async () => {
      const mockUser: User = {
        id: 'user-123',
        tenantId: 'tenant-456',
        email: 'test@example.com',
        role: 'user',
        createdAt: '2025-01-15T10:00:00Z',
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ data: mockUser }),
      });

      const result = await client.getCurrentUser();

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/auth\/validate$/),
        expect.objectContaining({
          method: 'GET',
          credentials: 'include',
        })
      );
      expect(result).toEqual(mockUser);
    });

    it('should return null when unauthenticated (401)', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        statusText: 'Unauthorized',
      });

      const result = await client.getCurrentUser();

      expect(result).toBeNull();
    });

    it('should throw error for other HTTP errors', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
      });

      await expect(client.getCurrentUser()).rejects.toThrow('Failed to get current user');
    });
  });

  describe('getCSRFToken', () => {
    it('should make POST request and return CSRF token', async () => {
      const mockCSRFToken = 'csrf-token-123';

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ data: { csrf_token: mockCSRFToken } }),
      });

      const result = await client.getCSRFToken();

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/auth\/csrf$/),
        expect.objectContaining({
          method: 'POST',
          credentials: 'include',
        })
      );
      expect(result).toBe(mockCSRFToken);
    });

    it('should return null and log warning on error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
      });

      const result = await client.getCSRFToken();

      expect(result).toBeNull();
      expect(consoleSpy).toHaveBeenCalledWith(
        'Failed to get CSRF token:',
        expect.any(Error)
      );
    });
  });

  describe('updateProfile', () => {
    it('should make PUT request with profile data and return updated User', async () => {
      const mockUser: User = {
        id: 'user-123',
        tenantId: 'tenant-456',
        email: 'test@example.com',
        name: 'Updated Name',
        role: 'user',
        createdAt: '2025-01-15T10:00:00Z',
      };

      // Mock CSRF token request first
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({ data: { csrf_token: 'csrf-token-123' } }),
        })
        // Mock actual profile update request
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({ data: mockUser }),
        });

      const profileUpdate = { name: 'Updated Name' };
      const result = await client.updateProfile(profileUpdate);

      expect(result).toEqual(mockUser);
    });
  });

  describe('getUserSettings', () => {
    it('should make GET request and return user settings', async () => {
      const mockSettings = { theme: 'dark', language: 'en' };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ data: mockSettings }),
      });

      const result = await client.getUserSettings();

      expect(mockFetch).toHaveBeenCalledWith(
        expect.stringMatching(/\/api\/auth\/settings$/),
        expect.objectContaining({
          method: 'GET',
          credentials: 'include',
        })
      );
      expect(result).toEqual(mockSettings);
    });
  });

  describe('updateUserSettings', () => {
    it('should make PUT request with settings data', async () => {
      // Mock CSRF token request first
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({ data: { csrf_token: 'csrf-token-123' } }),
        })
        // Mock actual settings update request
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({}),
        });

      const settings = { theme: 'light', language: 'ja' };
      await client.updateUserSettings(settings);

      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });

  describe('CSRF token integration', () => {
    it('should add CSRF token to unsafe HTTP methods', async () => {
      const mockCSRFToken = 'csrf-token-123';

      // Mock CSRF token request
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({ data: { csrf_token: mockCSRFToken } }),
        })
        // Mock actual request
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({}),
        });

      await client.logout();

      // Should have made CSRF token request first
      expect(mockFetch).toHaveBeenNthCalledWith(
        1,
        expect.stringMatching(/\/api\/auth\/csrf$/),
        expect.objectContaining({ method: 'POST' })
      );

      // Should have made logout request with CSRF token
      expect(mockFetch).toHaveBeenNthCalledWith(
        2,
        expect.stringMatching(/\/api\/auth\/logout$/),
        expect.objectContaining({
          headers: expect.objectContaining({
            'X-CSRF-Token': mockCSRFToken,
          }),
        })
      );
    });

    it('should proceed without CSRF token if retrieval fails', async () => {
      // Mock CSRF token failure
      mockFetch
        .mockResolvedValueOnce({
          ok: false,
          status: 500,
        })
        // Mock actual request
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve({}),
        });

      await client.logout();

      // Should still make the logout request without CSRF token
      expect(mockFetch).toHaveBeenNthCalledWith(
        2,
        expect.stringMatching(/\/api\/auth\/logout$/),
        expect.objectContaining({
          method: 'POST',
        })
      );
    });
  });
});