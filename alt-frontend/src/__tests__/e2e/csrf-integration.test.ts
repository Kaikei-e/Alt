// 🚨 CRITICAL: X22 Phase 6 - Comprehensive CSRF Integration E2E Tests
// End-to-end testing for CSRF token implementation and auto-recovery

import { describe, test, expect, beforeEach, afterEach } from 'vitest';
import { authAPI } from '@/lib/api/auth-client';
import { csrfAutoRecovery, withAutoRecovery } from '@/lib/auth/csrf-auto-recovery';

// Test configuration
const TEST_CONFIG = {
  testEmail: 'test@csrf-test.local',
  testPassword: 'TestPassword123!',
  testName: 'CSRF Test User',
  timeouts: {
    flowCreation: 5000,
    tokenExtraction: 3000,
    loginCompletion: 10000,
    autoRecovery: 15000,
  },
};

describe('CSRF Token Integration Tests', () => {
  let testFlowId: string;

  beforeEach(async () => {
    // Clear any existing auth state
    csrfAutoRecovery.clearActiveRecoveries();
  });

  afterEach(async () => {
    // Cleanup
    try {
      await authAPI.logout();
    } catch {
      // Ignore logout errors in tests
    }
  });

  describe('Phase 1: Basic CSRF Token Flow', () => {
    test('should create login flow and extract CSRF token', async () => {
      // 1. Create login flow
      const createResponse = await fetch('/api/auth/login', {
        method: 'POST',
        credentials: 'include',
      });

      expect(createResponse.ok).toBe(true);
      const createData = await createResponse.json();
      expect(createData.flow_id).toBeDefined();
      testFlowId = createData.flow_id;

      // 2. Get flow details to extract CSRF token
      const flowResponse = await fetch(`/api/auth/login/${testFlowId}`, {
        method: 'GET',
        credentials: 'include',
      });

      expect(flowResponse.ok).toBe(true);
      const flow = await flowResponse.json();

      // 3. Verify flow structure
      expect(flow.ui).toBeDefined();
      expect(flow.ui.nodes).toBeDefined();
      expect(Array.isArray(flow.ui.nodes)).toBe(true);

      // 4. Extract CSRF token from UI nodes
      const csrfNode = flow.ui.nodes.find((node: any) => 
        node.attributes?.name === 'csrf_token' && 
        node.attributes?.type === 'hidden'
      );

      expect(csrfNode).toBeDefined();
      expect(csrfNode.attributes?.value).toBeDefined();
      expect(typeof csrfNode.attributes.value).toBe('string');
      expect(csrfNode.attributes.value.length).toBeGreaterThan(30);

      console.log('✅ CSRF token extracted successfully:', {
        flowId: testFlowId,
        tokenLength: csrfNode.attributes.value.length,
        tokenPreview: `${csrfNode.attributes.value.substring(0, 8)}...${csrfNode.attributes.value.substring(csrfNode.attributes.value.length - 8)}`
      });
    }, TEST_CONFIG.timeouts.tokenExtraction);

    test('should include CSRF token in login request body', async () => {
      // 1. Create login flow
      const flow = await authAPI.initiateLogin();
      testFlowId = flow.id;

      // 2. Get flow details for CSRF token
      const flowResponse = await fetch(`/api/auth/login/${testFlowId}`, {
        credentials: 'include',
      });
      const flowData = await flowResponse.json();

      // 3. Extract CSRF token
      const csrfNode = flowData.ui.nodes.find((node: any) => 
        node.attributes?.name === 'csrf_token'
      );
      expect(csrfNode?.attributes?.value).toBeDefined();
      const csrfToken = csrfNode.attributes.value;

      // 4. Prepare login data with CSRF token
      const loginData = {
        method: 'password',
        identifier: TEST_CONFIG.testEmail,
        password: TEST_CONFIG.testPassword,
        csrf_token: csrfToken, // 🔑 CRITICAL: CSRF token inclusion
      };

      // 5. Submit login request
      const submitResponse = await fetch(`/api/auth/login/${testFlowId}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify(loginData),
      });

      // 6. Verify request structure (should not fail due to missing CSRF)
      console.log('📊 Login request details:', {
        flowId: testFlowId,
        status: submitResponse.status,
        hasCSRFToken: !!loginData.csrf_token,
        csrfTokenLength: loginData.csrf_token.length,
        requestBodyKeys: Object.keys(loginData),
      });

      // Note: We don't expect success due to test user not existing,
      // but we should not get CSRF-related 400/500 errors
      if (!submitResponse.ok) {
        const errorText = await submitResponse.text();
        expect(errorText).not.toMatch(/csrf.*not found/i);
        expect(errorText).not.toMatch(/csrf.*required/i);
        expect(errorText).not.toMatch(/csrf.*missing/i);
      }
    }, TEST_CONFIG.timeouts.loginCompletion);
  });

  describe('Phase 2: AuthAPI Integration', () => {
    test('should use enhanced auth API with CSRF token extraction', async () => {
      // Test the enhanced authAPI.completeLogin method
      try {
        const flow = await authAPI.initiateLogin();
        testFlowId = flow.id;

        // This should automatically extract and include CSRF token
        await authAPI.completeLogin(flow.id, TEST_CONFIG.testEmail, TEST_CONFIG.testPassword);
        
        // We don't expect this to succeed (test user doesn't exist),
        // but it should not fail due to CSRF issues
      } catch (error) {
        console.log('📊 AuthAPI integration test result:', {
          error: error instanceof Error ? error.message : String(error),
          isCSRFError: (error instanceof Error ? error.message : '').toLowerCase().includes('csrf'),
        });

        // Verify it's not a CSRF error
        const errorMessage = error instanceof Error ? error.message.toLowerCase() : '';
        expect(errorMessage).not.toMatch(/csrf.*not found/);
        expect(errorMessage).not.toMatch(/csrf.*required/);
        expect(errorMessage).not.toMatch(/csrf.*missing/);
      }
    }, TEST_CONFIG.timeouts.loginCompletion);
  });

  describe('Phase 3: Auto-Recovery System', () => {
    test('should automatically retry on CSRF errors', async () => {
      // Test the auto-recovery system
      const result = await withAutoRecovery.login(
        TEST_CONFIG.testEmail,
        TEST_CONFIG.testPassword
      );

      console.log('🔄 Auto-recovery test result:', {
        success: result.success,
        attempts: result.attempts,
        totalTime: result.totalTime,
        recoveryActions: result.recoveryActions,
        error: result.error?.message,
      });

      // Verify recovery system executed
      expect(result.attempts).toBeGreaterThan(0);
      expect(result.totalTime).toBeGreaterThan(0);
      expect(Array.isArray(result.recoveryActions)).toBe(true);

      // If failed, it should not be due to CSRF issues
      if (!result.success && result.error) {
        const errorMessage = result.error.message.toLowerCase();
        expect(errorMessage).not.toMatch(/csrf.*not found/);
        expect(errorMessage).not.toMatch(/csrf.*required/);
        expect(errorMessage).not.toMatch(/csrf.*missing/);
      }
    }, TEST_CONFIG.timeouts.autoRecovery);

    test('should provide detailed recovery statistics', async () => {
      const stats = csrfAutoRecovery.getRecoveryStats();

      expect(stats).toBeDefined();
      expect(typeof stats.activeRecoveries).toBe('number');
      expect(stats.configuration).toBeDefined();
      expect(typeof stats.configuration.maxRetries).toBe('number');
      expect(typeof stats.configuration.retryDelay).toBe('number');
      expect(typeof stats.configuration.enableLogging).toBe('boolean');

      console.log('📊 Recovery statistics:', stats);
    });
  });

  describe('Phase 4: Debug Endpoint Integration', () => {
    test('should validate CSRF submission through debug endpoint', async () => {
      // Test the debug validation endpoint
      const testPayload = {
        method: 'password',
        identifier: TEST_CONFIG.testEmail,
        password: TEST_CONFIG.testPassword,
        csrf_token: 'test-token-12345678901234567890123456789012', // 32+ chars
      };

      const debugResponse = await fetch('/api/auth/debug/csrf/validate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify(testPayload),
      });

      expect(debugResponse.ok).toBe(true);
      const diagnostic = await debugResponse.json();

      console.log('🔍 Debug endpoint validation:', diagnostic);

      // Verify diagnostic structure
      expect(diagnostic.analysis).toBeDefined();
      expect(diagnostic.csrf_analysis).toBeDefined();
      expect(diagnostic.compliance_check).toBeDefined();
      expect(diagnostic.recommendations).toBeDefined();
      expect(Array.isArray(diagnostic.recommendations)).toBe(true);

      // Verify CSRF analysis
      expect(diagnostic.csrf_analysis.csrf_token_present).toBe(true);
      expect(diagnostic.csrf_analysis.csrf_token_field).toBe('csrf_token');
      expect(diagnostic.csrf_analysis.csrf_token_length).toBe(testPayload.csrf_token.length);

      // Verify compliance check
      expect(diagnostic.compliance_check.overall_compliant).toBe(true);
    });

    test('should detect missing CSRF token through debug endpoint', async () => {
      // Test debug endpoint with missing CSRF token
      const incompletePayload = {
        method: 'password',
        identifier: TEST_CONFIG.testEmail,
        password: TEST_CONFIG.testPassword,
        // csrf_token is missing
      };

      const debugResponse = await fetch('/api/auth/debug/csrf/validate', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify(incompletePayload),
      });

      expect(debugResponse.status).toBe(422); // Unprocessable Entity
      const diagnostic = await debugResponse.json();

      console.log('🚨 Debug endpoint missing CSRF detection:', diagnostic);

      // Verify CSRF detection
      expect(diagnostic.csrf_analysis.csrf_token_present).toBe(false);
      expect(diagnostic.validation_result).toBe('CSRF_TOKEN_MISSING_OR_INVALID');
      expect(diagnostic.compliance_check.overall_compliant).toBe(false);
      expect(diagnostic.recommendations).toContain('🚨 Include csrf_token field in request body');
    });
  });

  describe('Phase 5: Error Handling and Edge Cases', () => {
    test('should handle expired flow gracefully', async () => {
      // Create a flow and wait for it to potentially expire (or use invalid flow)
      const expiredFlowId = 'expired-flow-id-12345';

      try {
        await authAPI.completeLogin(expiredFlowId, TEST_CONFIG.testEmail, TEST_CONFIG.testPassword);
      } catch (error) {
        console.log('⏰ Expired flow handling:', {
          error: error instanceof Error ? error.message : String(error),
        });

        // Should handle expired flow errors gracefully
        expect(error).toBeDefined();
        const errorMessage = error instanceof Error ? error.message.toLowerCase() : '';
        
        // Common expired flow error patterns
        const expiredPatterns = [
          'flow not found',
          'expired',
          'invalid flow',
          '404',
          '410',
        ];
        
        const hasExpiredPattern = expiredPatterns.some(pattern => 
          errorMessage.includes(pattern)
        );
        
        expect(hasExpiredPattern).toBe(true);
      }
    });

    test('should handle network errors with retry', async () => {
      // Test network error resilience
      const originalFetch = global.fetch;
      let callCount = 0;

      // Mock fetch to fail first time, succeed second time
      global.fetch = async (...args) => {
        callCount++;
        if (callCount === 1) {
          throw new Error('Network error - connection failed');
        }
        return originalFetch(...args);
      };

      try {
        const result = await withAutoRecovery.execute(
          () => authAPI.testConnection(),
          'network_test'
        );

        console.log('🌐 Network error handling:', {
          success: result.success,
          attempts: result.attempts,
          recoveryActions: result.recoveryActions,
        });

        // Should have retried
        expect(result.attempts).toBeGreaterThan(1);
        expect(callCount).toBeGreaterThan(1);

      } finally {
        // Restore original fetch
        global.fetch = originalFetch;
      }
    });
  });
});

describe('CSRF Performance and Load Tests', () => {
  test('should handle multiple concurrent login attempts', async () => {
    const concurrentLogins = 5;
    const startTime = performance.now();

    const loginPromises = Array.from({ length: concurrentLogins }, (_, index) => 
      withAutoRecovery.login(
        `test${index}@csrf-test.local`,
        TEST_CONFIG.testPassword
      )
    );

    const results = await Promise.allSettled(loginPromises);
    const endTime = performance.now();

    console.log('🚀 Concurrent login performance:', {
      totalTime: endTime - startTime,
      attempts: concurrentLogins,
      avgTimePerAttempt: (endTime - startTime) / concurrentLogins,
      successfulAttempts: results.filter(r => r.status === 'fulfilled').length,
      failedAttempts: results.filter(r => r.status === 'rejected').length,
    });

    // All attempts should complete (success or failure, but not hang)
    expect(results.length).toBe(concurrentLogins);
    
    // Should not take excessively long
    expect(endTime - startTime).toBeLessThan(30000); // 30 seconds max
  });

  test('should maintain performance under repeated CSRF token extraction', async () => {
    const iterations = 10;
    const times: number[] = [];

    for (let i = 0; i < iterations; i++) {
      const startTime = performance.now();
      
      try {
        const flow = await authAPI.initiateLogin();
        // Token extraction is done internally by completeLogin
        await authAPI.completeLogin(flow.id, `test${i}@csrf-test.local`, TEST_CONFIG.testPassword);
      } catch {
        // Expected to fail, we're measuring performance
      }
      
      const endTime = performance.now();
      times.push(endTime - startTime);
    }

    const avgTime = times.reduce((sum, time) => sum + time, 0) / times.length;
    const maxTime = Math.max(...times);
    const minTime = Math.min(...times);

    console.log('⚡ CSRF token extraction performance:', {
      iterations,
      avgTime: avgTime.toFixed(2),
      maxTime: maxTime.toFixed(2),
      minTime: minTime.toFixed(2),
      totalTime: times.reduce((sum, time) => sum + time, 0).toFixed(2),
    });

    // Performance expectations
    expect(avgTime).toBeLessThan(5000); // Average under 5 seconds
    expect(maxTime).toBeLessThan(10000); // No single attempt over 10 seconds
  });
});

// Helper functions for test utilities
function extractCSRFTokenFromFlow(flow: any): string | null {
  if (!flow?.ui?.nodes || !Array.isArray(flow.ui.nodes)) {
    return null;
  }

  const csrfNode = flow.ui.nodes.find((node: any) => 
    node?.attributes?.name === 'csrf_token' && 
    node?.attributes?.type === 'hidden'
  );

  return csrfNode?.attributes?.value || null;
}

function validateCSRFTokenFormat(token: string): boolean {
  return typeof token === 'string' && 
         token.length >= 32 && 
         token.length <= 200 &&
         /^[a-zA-Z0-9+/=_-]+$/.test(token);
}