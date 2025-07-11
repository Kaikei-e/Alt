import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { FeatureFlags, shouldUseVirtualization, FeatureFlagManager } from './featureFlags';

describe('Feature Flags', () => {
  beforeEach(() => {
    // Reset environment variables for each test
    vi.stubEnv('NEXT_PUBLIC_ENABLE_VIRTUALIZATION', 'auto');
    vi.stubEnv('NEXT_PUBLIC_FORCE_VIRTUALIZATION', 'false');
    vi.stubEnv('NEXT_PUBLIC_VIRTUALIZATION_THRESHOLD', '200');
    vi.stubEnv('NODE_ENV', 'test');
  });

  afterEach(() => {
    vi.unstubAllEnvs();
    // Clear localStorage mock
    if (typeof window !== 'undefined') {
      localStorage.clear();
    }
  });

  describe('shouldUseVirtualization', () => {
    it('should disable virtualization when flag is off', () => {
      const flags: FeatureFlags = {
        enableVirtualization: false,
        forceVirtualization: false,
        debugMode: false
      };
      
      expect(shouldUseVirtualization(1000, flags)).toBe(false);
    });

    it('should use performance threshold when flag is auto', () => {
      const flags: FeatureFlags = {
        enableVirtualization: 'auto',
        forceVirtualization: false,
        debugMode: false,
        virtualizationThreshold: 200
      };
      
      // Below threshold should be disabled
      expect(shouldUseVirtualization(50, flags)).toBe(false);
      expect(shouldUseVirtualization(150, flags)).toBe(false);
      
      // At or above threshold should be enabled
      expect(shouldUseVirtualization(200, flags)).toBe(true);
      expect(shouldUseVirtualization(500, flags)).toBe(true);
    });

    it('should force virtualization when forceVirtualization is true', () => {
      const flags: FeatureFlags = {
        enableVirtualization: false,
        forceVirtualization: true,
        debugMode: false
      };
      
      expect(shouldUseVirtualization(10, flags)).toBe(true);
      expect(shouldUseVirtualization(1, flags)).toBe(true);
    });

    it('should enable virtualization when flag is explicitly true', () => {
      const flags: FeatureFlags = {
        enableVirtualization: true,
        forceVirtualization: false,
        debugMode: false
      };
      
      expect(shouldUseVirtualization(10, flags)).toBe(true);
      expect(shouldUseVirtualization(500, flags)).toBe(true);
    });

    it('should use custom threshold when provided', () => {
      const flags: FeatureFlags = {
        enableVirtualization: 'auto',
        forceVirtualization: false,
        debugMode: false,
        virtualizationThreshold: 100
      };
      
      expect(shouldUseVirtualization(50, flags)).toBe(false);
      expect(shouldUseVirtualization(100, flags)).toBe(true);
      expect(shouldUseVirtualization(150, flags)).toBe(true);
    });
  });

  describe('FeatureFlagManager', () => {
    beforeEach(() => {
      // Reset singleton instance
      (FeatureFlagManager as any).instance = undefined;
    });

    it('should create singleton instance', () => {
      const instance1 = FeatureFlagManager.getInstance();
      const instance2 = FeatureFlagManager.getInstance();
      
      expect(instance1).toBe(instance2);
    });

    it('should load flags from environment variables', () => {
      vi.stubEnv('NEXT_PUBLIC_ENABLE_VIRTUALIZATION', 'true');
      vi.stubEnv('NEXT_PUBLIC_FORCE_VIRTUALIZATION', 'true');
      vi.stubEnv('NEXT_PUBLIC_VIRTUALIZATION_THRESHOLD', '300');
      vi.stubEnv('NODE_ENV', 'development');

      const manager = FeatureFlagManager.getInstance();
      const flags = manager.getFlags();

      expect(flags.enableVirtualization).toBe('true');
      expect(flags.forceVirtualization).toBe(true);
      expect(flags.virtualizationThreshold).toBe(300);
      expect(flags.debugMode).toBe(true);
    });

    it('should default to auto mode and 200 threshold', () => {
      const manager = FeatureFlagManager.getInstance();
      const flags = manager.getFlags();

      expect(flags.enableVirtualization).toBe('auto');
      expect(flags.forceVirtualization).toBe(false);
      expect(flags.virtualizationThreshold).toBe(200);
    });

    it('should update flags dynamically', () => {
      const manager = FeatureFlagManager.getInstance();
      
      manager.updateFlags({
        enableVirtualization: true,
        forceVirtualization: true
      });

      const flags = manager.getFlags();
      expect(flags.enableVirtualization).toBe(true);
      expect(flags.forceVirtualization).toBe(true);
    });

    it('should not modify original flags object', () => {
      const manager = FeatureFlagManager.getInstance();
      const flags1 = manager.getFlags();
      const flags2 = manager.getFlags();
      
      flags1.enableVirtualization = true;
      
      expect(flags2.enableVirtualization).not.toBe(true);
    });
  });

  describe('Integration with shouldUseVirtualization', () => {
    beforeEach(() => {
      (FeatureFlagManager as any).instance = undefined;
    });

    it('should use FeatureFlagManager when no flags provided', () => {
      vi.stubEnv('NEXT_PUBLIC_ENABLE_VIRTUALIZATION', 'auto');
      vi.stubEnv('NEXT_PUBLIC_VIRTUALIZATION_THRESHOLD', '150');

      // Should use threshold from environment
      expect(shouldUseVirtualization(100)).toBe(false);
      expect(shouldUseVirtualization(150)).toBe(true);
    });

    it('should override with provided flags', () => {
      vi.stubEnv('NEXT_PUBLIC_ENABLE_VIRTUALIZATION', 'auto');
      vi.stubEnv('NEXT_PUBLIC_VIRTUALIZATION_THRESHOLD', '150');

      const customFlags: FeatureFlags = {
        enableVirtualization: false,
        forceVirtualization: false,
        debugMode: false
      };

      // Should use custom flags, not environment
      expect(shouldUseVirtualization(200, customFlags)).toBe(false);
    });
  });
});