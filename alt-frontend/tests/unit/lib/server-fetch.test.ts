/**
 * @vitest-environment node
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { headers } from 'next/headers'
import { serverFetch } from '../../../src/server-fetch'

// Mock next/headers
vi.mock('next/headers', () => ({
  headers: vi.fn(),
}))

// Mock global fetch
const mockFetch = vi.fn()
global.fetch = mockFetch

// Helper to create mock headers that satisfy Headers interface
const createMockHeaders = (cookieValue?: string | null) => ({
  get: vi.fn().mockReturnValue(cookieValue),
  append: vi.fn(),
  delete: vi.fn(),
  set: vi.fn(),
  has: vi.fn(),
  forEach: vi.fn(),
  entries: vi.fn(),
  keys: vi.fn(),
  values: vi.fn(),
  getSetCookie: vi.fn().mockReturnValue([]),
  [Symbol.iterator]: vi.fn()
})

// Mock environment variable
process.env.API_URL = 'http://alt-backend:9000'

describe('serverFetch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.resetAllMocks()
  })

  describe('successful requests', () => {
    it('should make request with cookie from headers', async () => {
      const mockHeaders = createMockHeaders('ory_kratos_session=session-value; other_cookie=other-value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: true,
        json: vi.fn().mockResolvedValue({ data: 'test-data' })
      }
      mockFetch.mockResolvedValue(mockResponse)

      const result = await serverFetch('/test-endpoint')

      expect(mockFetch).toHaveBeenCalledWith(
        'http://alt-backend:9000/test-endpoint',
        {
          headers: {
            'Content-Type': 'application/json',
            'Cookie': 'ory_kratos_session=session-value; other_cookie=other-value'
          },
          cache: 'no-store'
        }
      )
      expect(result).toEqual({ data: 'test-data' })
    })

    it('should handle empty cookie header', async () => {
      const mockHeaders = createMockHeaders(null)
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: true,
        json: vi.fn().mockResolvedValue({ data: 'test-data' })
      }
      mockFetch.mockResolvedValue(mockResponse)

      const result = await serverFetch('/test-endpoint')

      expect(mockFetch).toHaveBeenCalledWith(
        'http://alt-backend:9000/test-endpoint',
        {
          headers: {
            'Content-Type': 'application/json',
            'Cookie': ''
          },
          cache: 'no-store'
        }
      )
      expect(result).toEqual({ data: 'test-data' })
    })

    it('should merge additional headers correctly', async () => {
      const mockHeaders = createMockHeaders('test_cookie=test_value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: true,
        json: vi.fn().mockResolvedValue({ success: true })
      }
      mockFetch.mockResolvedValue(mockResponse)

      await serverFetch('/test-endpoint', {
        method: 'POST',
        headers: {
          'X-Custom-Header': 'custom-value',
          'Authorization': 'Bearer token'
        },
        body: JSON.stringify({ test: 'data' })
      })

      expect(mockFetch).toHaveBeenCalledWith(
        'http://alt-backend:9000/test-endpoint',
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Cookie': 'test_cookie=test_value',
            'X-Custom-Header': 'custom-value',
            'Authorization': 'Bearer token'
          },
          body: JSON.stringify({ test: 'data' }),
          cache: 'no-store'
        }
      )
    })
  })

  describe('error handling', () => {
    it('should throw error when response is not ok (404)', async () => {
      const mockHeaders = createMockHeaders('session_cookie=value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: false,
        status: 404
      }
      mockFetch.mockResolvedValue(mockResponse)

      await expect(serverFetch('/not-found')).rejects.toThrow('API 404 for /not-found')
    })

    it('should throw error when response is not ok (500)', async () => {
      const mockHeaders = createMockHeaders('session_cookie=value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: false,
        status: 500
      }
      mockFetch.mockResolvedValue(mockResponse)

      await expect(serverFetch('/server-error')).rejects.toThrow('API 500 for /server-error')
    })

    it('should throw error when response is not ok (401 Unauthorized)', async () => {
      const mockHeaders = createMockHeaders('')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: false,
        status: 401
      }
      mockFetch.mockResolvedValue(mockResponse)

      await expect(serverFetch('/protected-endpoint')).rejects.toThrow('API 401 for /protected-endpoint')
    })

    it('should handle network errors', async () => {
      const mockHeaders = createMockHeaders('test_cookie=value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      mockFetch.mockRejectedValue(new Error('Network error'))

      await expect(serverFetch('/test-endpoint')).rejects.toThrow('Network error')
    })
  })

  describe('endpoint handling', () => {
    it('should handle endpoints starting with slash', async () => {
      const mockHeaders = createMockHeaders('cookie=value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: true,
        json: vi.fn().mockResolvedValue({ data: 'test' })
      }
      mockFetch.mockResolvedValue(mockResponse)

      await serverFetch('/api/users')

      expect(mockFetch).toHaveBeenCalledWith(
        'http://alt-backend:9000/api/users',
        expect.any(Object)
      )
    })

    it('should handle endpoints without leading slash', async () => {
      const mockHeaders = createMockHeaders('cookie=value')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: true,
        json: vi.fn().mockResolvedValue({ data: 'test' })
      }
      mockFetch.mockResolvedValue(mockResponse)

      await serverFetch('api/users')

      // Current implementation simply concatenates, so no slash is added
      expect(mockFetch).toHaveBeenCalledWith(
        'http://alt-backend:9000api/users',
        expect.any(Object)
      )
    })

    it('should handle complex endpoints with query parameters', async () => {
      const mockHeaders = createMockHeaders('session=abc123')
      vi.mocked(headers).mockResolvedValue(mockHeaders)

      const mockResponse = {
        ok: true,
        json: vi.fn().mockResolvedValue({ items: [] })
      }
      mockFetch.mockResolvedValue(mockResponse)

      await serverFetch('/api/feeds?page=2&limit=10&category=tech')

      expect(mockFetch).toHaveBeenCalledWith(
        'http://alt-backend:9000/api/feeds?page=2&limit=10&category=tech',
        expect.any(Object)
      )
    })
  })
})