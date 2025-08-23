/**
 * @vitest-environment node
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { NextRequest } from 'next/server'
import { middleware } from './middleware'

// Mock environment variables
process.env.NEXT_PUBLIC_APP_ORIGIN = 'https://curionoah.com'
process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL = 'https://id.curionoah.com'

describe('middleware', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('public paths', () => {
    it('should allow access to root path', () => {
      const request = new NextRequest('https://curionoah.com/')
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })

    it('should allow access to auth paths', () => {
      const request = new NextRequest('https://curionoah.com/auth/login')
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })

    it('should allow access to api paths', () => {
      const request = new NextRequest('https://curionoah.com/api/backend/test')
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })

    it('should allow access to _next paths', () => {
      const request = new NextRequest('https://curionoah.com/_next/static/test.js')
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })

    it('should allow access to static files', () => {
      const request = new NextRequest('https://curionoah.com/favicon.ico')
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })
  })

  describe('authenticated access', () => {
    it('should allow access when ory_kratos_session cookie exists', () => {
      const request = new NextRequest('https://curionoah.com/desktop/home')
      request.cookies.set('ory_kratos_session', 'test-session-value')
      
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })
  })

  describe('unauthenticated access with guard cookie', () => {
    it('should allow access when redirect guard cookie exists', () => {
      const request = new NextRequest('https://curionoah.com/desktop/home')
      request.cookies.set('alt_auth_redirect_guard', '1')
      
      const response = middleware(request)
      
      expect(response.status).toBe(200)
    })
  })

  describe('unauthenticated access without guard cookie', () => {
    it('should redirect to Kratos login and set guard cookie', () => {
      const request = new NextRequest('https://curionoah.com/desktop/home?test=123')
      
      const response = middleware(request)
      
      expect(response.status).toBe(307) // Next.js redirect status
      
      const location = response.headers.get('location')
      expect(location).toContain('https://id.curionoah.com/self-service/login/browser')
      expect(location).toContain('return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome%3Ftest%3D123')
      
      // Check that guard cookie is set
      const setCookieHeader = response.headers.get('set-cookie')
      expect(setCookieHeader).toContain('alt_auth_redirect_guard=1')
      expect(setCookieHeader).toContain('Domain=curionoah.com')
      expect(setCookieHeader).toContain('HttpOnly')
      expect(setCookieHeader).toContain('Secure')
      expect(setCookieHeader).toContain('SameSite=lax')
      expect(setCookieHeader).toContain('Max-Age=10')
    })

    it('should handle paths with search parameters correctly', () => {
      const request = new NextRequest('https://curionoah.com/desktop/feeds?category=tech&page=2')
      
      const response = middleware(request)
      
      const location = response.headers.get('location')
      expect(location).toContain('return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Ffeeds%3Fcategory%3Dtech%26page%3D2')
    })
  })

  describe('edge cases', () => {
    it('should handle empty search parameters', () => {
      const request = new NextRequest('https://curionoah.com/desktop/home?')
      
      const response = middleware(request)
      
      const location = response.headers.get('location')
      expect(location).toContain('return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fhome')
    })

    it('should handle paths without search parameters', () => {
      const request = new NextRequest('https://curionoah.com/desktop/settings')
      
      const response = middleware(request)
      
      const location = response.headers.get('location')
      expect(location).toContain('return_to=https%3A%2F%2Fcurionoah.com%2Fdesktop%2Fsettings')
    })
  })
})