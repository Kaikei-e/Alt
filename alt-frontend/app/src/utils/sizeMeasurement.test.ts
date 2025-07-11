import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { SizeMeasurementManager } from './sizeMeasurement';

// Mock DOM API
Object.defineProperty(HTMLElement.prototype, 'getBoundingClientRect', {
  value: vi.fn(() => ({ width: 100, height: 200, top: 0, left: 0, right: 100, bottom: 200 })),
  writable: true,
});

Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
  get: vi.fn(() => document.body),
  configurable: true,
});

describe('SizeMeasurementManager', () => {
  let manager: SizeMeasurementManager;
  let mockElement: HTMLElement;
  let onError: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    onError = vi.fn();
    manager = new SizeMeasurementManager(onError);
    mockElement = document.createElement('div');
    
    // Reset mocks
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('measureElement', () => {
    it('should measure element dimensions correctly', async () => {
      const result = await manager.measureElement(mockElement, 'test-key');

      expect(result).toEqual({
        width: 100,
        height: 200,
        timestamp: expect.any(Number)
      });
    });

    it('should cache measurements for 5 seconds', async () => {
      const getBoundingClientRectSpy = vi.spyOn(mockElement, 'getBoundingClientRect');
      
      // First measurement
      await manager.measureElement(mockElement, 'test-key');
      expect(getBoundingClientRectSpy).toHaveBeenCalledTimes(1);

      // Second measurement within 5 seconds - should use cache
      await manager.measureElement(mockElement, 'test-key');
      expect(getBoundingClientRectSpy).toHaveBeenCalledTimes(1);
    });

    it('should remeasure after cache expires', async () => {
      const getBoundingClientRectSpy = vi.spyOn(mockElement, 'getBoundingClientRect');
      
      // First measurement
      await manager.measureElement(mockElement, 'test-key');
      expect(getBoundingClientRectSpy).toHaveBeenCalledTimes(1);

      // Mock time passing (6 seconds)
      vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 6000);

      // Second measurement after cache expires
      await manager.measureElement(mockElement, 'test-key');
      expect(getBoundingClientRectSpy).toHaveBeenCalledTimes(2);
    });

    it('should return null for elements without offsetParent', async () => {
      Object.defineProperty(mockElement, 'offsetParent', {
        get: () => null,
        configurable: true
      });

      const result = await manager.measureElement(mockElement, 'test-key');
      expect(result).toBeNull();
    });

    it('should skip measurement if already pending', async () => {
      const getBoundingClientRectSpy = vi.spyOn(mockElement, 'getBoundingClientRect');
      
      // Start two measurements simultaneously
      const promise1 = manager.measureElement(mockElement, 'test-key');
      const promise2 = manager.measureElement(mockElement, 'test-key');

      await Promise.all([promise1, promise2]);
      
      // Should only measure once
      expect(getBoundingClientRectSpy).toHaveBeenCalledTimes(1);
    });

    it('should handle measurement errors gracefully', async () => {
      vi.spyOn(mockElement, 'getBoundingClientRect').mockImplementation(() => {
        throw new Error('Measurement error');
      });

      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      const result = await manager.measureElement(mockElement, 'test-key');
      
      expect(result).toBeNull();
      expect(consoleSpy).toHaveBeenCalledWith('Size measurement failed:', expect.any(Error));
      expect(onError).not.toHaveBeenCalled(); // Should not call onError for first few errors
      
      consoleSpy.mockRestore();
    });

    it('should call onError after too many errors', async () => {
      vi.spyOn(mockElement, 'getBoundingClientRect').mockImplementation(() => {
        throw new Error('Measurement error');
      });

      const consoleSpy = vi.spyOn(console, 'warn').mockImplementation(() => {});

      // Trigger 11 errors (maxErrors = 10)
      for (let i = 0; i < 11; i++) {
        await manager.measureElement(mockElement, `test-key-${i}`);
      }

      expect(onError).toHaveBeenCalledWith(new Error('Too many measurement errors'));
      
      consoleSpy.mockRestore();
    });
  });

  describe('clearCache', () => {
    it('should clear measurement cache', async () => {
      await manager.measureElement(mockElement, 'test-key');
      
      manager.clearCache();
      
      const getBoundingClientRectSpy = vi.spyOn(mockElement, 'getBoundingClientRect');
      await manager.measureElement(mockElement, 'test-key');
      
      expect(getBoundingClientRectSpy).toHaveBeenCalledTimes(1);
    });
  });

  describe('getEstimatedSize', () => {
    it('should return base height for short content', () => {
      const result = manager.getEstimatedSize(10);
      expect(result).toBe(144); // Base height + 1 line (10 chars / 50 chars per line = 1 line)
    });

    it('should add height for longer content', () => {
      const result = manager.getEstimatedSize(100); // 100 characters = 2 lines
      expect(result).toBe(120 + (2 * 24)); // Base height + 2 lines * line height
    });

    it('should handle very long content', () => {
      const result = manager.getEstimatedSize(1000); // Very long content
      expect(result).toBe(120 + (20 * 24)); // Base height + 20 lines * line height
    });

    it('should handle zero content length', () => {
      const result = manager.getEstimatedSize(0);
      expect(result).toBe(120); // Base height + 0 lines (Math.ceil(0 / 50) = 0)
    });
  });
});