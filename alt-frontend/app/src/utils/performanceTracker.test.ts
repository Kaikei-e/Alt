import { describe, it, expect, beforeEach, vi } from 'vitest';
import { SimplePerformanceTracker } from './performanceTracker';

describe('SimplePerformanceTracker', () => {
  let tracker: SimplePerformanceTracker;
  let consoleLogSpy: any;

  beforeEach(() => {
    tracker = new SimplePerformanceTracker();
    consoleLogSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('Metrics Recording', () => {
    it('should record performance metrics', () => {
      const metrics = {
        renderTime: 100,
        itemCount: 200,
        memoryUsage: 1024 * 1024 * 10 // 10MB
      };

      tracker.recordMetrics(metrics);

      expect(tracker.getAverageRenderTime()).toBe(100);
    });

    it('should limit history to maxHistory entries', () => {
      // 25個のメトリクスを記録（maxHistory=20を超える）
      for (let i = 0; i < 25; i++) {
        tracker.recordMetrics({
          renderTime: i * 10,
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      // 平均が最新の20個のみで計算されることを確認
      const expectedAverage = ((5 + 6 + 7 + 8 + 9 + 10 + 11 + 12 + 13 + 14 + 15 + 16 + 17 + 18 + 19 + 20 + 21 + 22 + 23 + 24) * 10) / 20;
      expect(tracker.getAverageRenderTime()).toBe(expectedAverage);
    });

    it('should log metrics in development mode', () => {
      vi.stubEnv('NODE_ENV', 'development');
      
      const metrics = {
        renderTime: 150,
        itemCount: 300,
        memoryUsage: 1024 * 1024 * 5
      };

      tracker.recordMetrics(metrics);

      expect(consoleLogSpy).toHaveBeenCalledWith('Performance metrics:', expect.objectContaining({
        renderTime: 150,
        itemCount: 300,
        memoryUsage: 1024 * 1024 * 5,
        timestamp: expect.any(Number)
      }));
    });

    it('should not log metrics in production mode', () => {
      vi.stubEnv('NODE_ENV', 'production');
      
      const metrics = {
        renderTime: 150,
        itemCount: 300,
        memoryUsage: 1024 * 1024 * 5
      };

      tracker.recordMetrics(metrics);

      expect(consoleLogSpy).not.toHaveBeenCalled();
    });
  });

  describe('Performance Analysis', () => {
    it('should calculate average render time correctly', () => {
      const renderTimes = [100, 200, 300, 400, 500];
      
      renderTimes.forEach(time => {
        tracker.recordMetrics({
          renderTime: time,
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      });

      const expectedAverage = renderTimes.reduce((sum, time) => sum + time, 0) / renderTimes.length;
      expect(tracker.getAverageRenderTime()).toBe(expectedAverage);
    });

    it('should return 0 for average when no metrics recorded', () => {
      expect(tracker.getAverageRenderTime()).toBe(0);
    });

    it('should detect performance regression', () => {
      // 最初の5個のメトリクス（古いデータ）
      for (let i = 0; i < 5; i++) {
        tracker.recordMetrics({
          renderTime: 100, // 平均100ms
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      // 中間のメトリクス
      for (let i = 0; i < 5; i++) {
        tracker.recordMetrics({
          renderTime: 100,
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      // 最新の5個のメトリクス（性能悪化）
      for (let i = 0; i < 5; i++) {
        tracker.recordMetrics({
          renderTime: 160, // 平均160ms（60%悪化）
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      expect(tracker.detectPerformanceRegression()).toBe(true);
    });

    it('should not detect regression with insufficient data', () => {
      // 10個未満のメトリクス
      for (let i = 0; i < 8; i++) {
        tracker.recordMetrics({
          renderTime: 100,
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      expect(tracker.detectPerformanceRegression()).toBe(false);
    });

    it('should not detect regression with stable performance', () => {
      // 安定したパフォーマンス（10個のメトリクス）
      for (let i = 0; i < 10; i++) {
        tracker.recordMetrics({
          renderTime: 100 + (i % 2 === 0 ? 5 : -5), // 95-105msの範囲
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      expect(tracker.detectPerformanceRegression()).toBe(false);
    });
  });

  describe('History Management', () => {
    it('should clear history', () => {
      tracker.recordMetrics({
        renderTime: 100,
        itemCount: 100,
        memoryUsage: 1024 * 1024
      });

      expect(tracker.getAverageRenderTime()).toBe(100);

      tracker.clearHistory();

      expect(tracker.getAverageRenderTime()).toBe(0);
    });

    it('should handle clearing empty history', () => {
      tracker.clearHistory();
      expect(tracker.getAverageRenderTime()).toBe(0);
    });
  });

  describe('Edge Cases', () => {
    it('should handle metrics with zero render time', () => {
      tracker.recordMetrics({
        renderTime: 0,
        itemCount: 100,
        memoryUsage: 1024 * 1024
      });

      expect(tracker.getAverageRenderTime()).toBe(0);
    });

    it('should handle metrics with negative values gracefully', () => {
      tracker.recordMetrics({
        renderTime: -10,
        itemCount: 100,
        memoryUsage: 1024 * 1024
      });

      expect(tracker.getAverageRenderTime()).toBe(-10);
    });

    it('should handle very large render times', () => {
      const largeTime = 1000000; // 1秒
      tracker.recordMetrics({
        renderTime: largeTime,
        itemCount: 100,
        memoryUsage: 1024 * 1024
      });

      expect(tracker.getAverageRenderTime()).toBe(largeTime);
    });
  });

  describe('Custom Configuration', () => {
    it('should respect custom maxHistory', () => {
      const customTracker = new SimplePerformanceTracker(5); // maxHistory=5
      
      // 7個のメトリクスを記録
      for (let i = 0; i < 7; i++) {
        customTracker.recordMetrics({
          renderTime: i * 10,
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      // 最新の5個のみで平均計算
      const expectedAverage = ((2 + 3 + 4 + 5 + 6) * 10) / 5;
      expect(customTracker.getAverageRenderTime()).toBe(expectedAverage);
    });

    it('should use default maxHistory when not specified', () => {
      const defaultTracker = new SimplePerformanceTracker();
      
      // 25個のメトリクスを記録
      for (let i = 0; i < 25; i++) {
        defaultTracker.recordMetrics({
          renderTime: i * 10,
          itemCount: 100,
          memoryUsage: 1024 * 1024
        });
      }

      // デフォルトの20個で平均計算
      const expectedAverage = ((5 + 6 + 7 + 8 + 9 + 10 + 11 + 12 + 13 + 14 + 15 + 16 + 17 + 18 + 19 + 20 + 21 + 22 + 23 + 24) * 10) / 20;
      expect(defaultTracker.getAverageRenderTime()).toBe(expectedAverage);
    });
  });
});