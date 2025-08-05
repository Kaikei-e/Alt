import { describe, it, expect, beforeEach } from "vitest";
import { ErrorRecoveryManager } from "./errorRecovery";

describe("ErrorRecoveryManager", () => {
  let recovery: ErrorRecoveryManager;
  let mockTime: number;

  beforeEach(() => {
    mockTime = 0;
    recovery = new ErrorRecoveryManager({}, () => mockTime);
  });

  describe("Error Recording", () => {
    it("should disable virtualization after consecutive errors", () => {
      expect(recovery.shouldDisableVirtualization()).toBe(false);

      // 3回連続エラーを記録
      recovery.recordError("virtualization_error");
      recovery.recordError("virtualization_error");
      recovery.recordError("virtualization_error");

      expect(recovery.shouldDisableVirtualization()).toBe(true);
      expect(recovery.getErrorCount()).toBe(3);
    });

    it("should limit error history to 10 entries", () => {
      // 15回エラーを記録
      for (let i = 0; i < 15; i++) {
        recovery.recordError(`error_${i}`);
      }

      expect(recovery.getErrorCount()).toBe(10); // 最大10まで
    });

    it("should reset success history on error", () => {
      // まず成功を記録
      recovery.recordSuccess();
      recovery.recordSuccess();
      expect(recovery.getSuccessCount()).toBe(2);

      // エラーを記録
      recovery.recordError("test_error");
      expect(recovery.getSuccessCount()).toBe(0);
    });
  });

  describe("Success Recording", () => {
    it("should gradually re-enable after success streak", () => {
      // まずエラーを記録して無効化
      for (let i = 0; i < 3; i++) {
        recovery.recordError("virtualization_error");
      }
      expect(recovery.shouldDisableVirtualization()).toBe(true);

      // 成功を記録して復旧
      for (let i = 0; i < 10; i++) {
        recovery.recordSuccess();
      }
      expect(recovery.getSuccessCount()).toBe(10);

      // バックオフ期間を経過させる
      mockTime = 5000;
      expect(recovery.shouldDisableVirtualization()).toBe(false);
    });

    it("should reset error history on success", () => {
      recovery.recordError("test_error");
      recovery.recordError("test_error");
      expect(recovery.getErrorCount()).toBe(2);

      recovery.recordSuccess();
      expect(recovery.getErrorCount()).toBe(0);
    });

    it("should limit success history to 15 entries", () => {
      for (let i = 0; i < 20; i++) {
        recovery.recordSuccess();
      }

      expect(recovery.getSuccessCount()).toBe(15);
    });
  });

  describe("Backoff Mechanism", () => {
    it("should implement exponential backoff", () => {
      const recovery = new ErrorRecoveryManager({
        initialBackoff: 1000,
        maxBackoff: 60000,
      });

      expect(recovery.getBackoffTime()).toBe(1000);

      recovery.recordError("test_error");
      expect(recovery.getBackoffTime()).toBe(2000);

      recovery.recordError("test_error");
      expect(recovery.getBackoffTime()).toBe(4000);

      recovery.recordError("test_error");
      expect(recovery.getBackoffTime()).toBe(8000);
    });

    it("should respect max backoff limit", () => {
      const recovery = new ErrorRecoveryManager({
        initialBackoff: 1000,
        maxBackoff: 5000,
      });

      // 大量のエラーを記録
      for (let i = 0; i < 10; i++) {
        recovery.recordError("test_error");
      }

      expect(recovery.getBackoffTime()).toBe(5000); // maxBackoffを超えない
    });

    it("should reset backoff after sufficient success", () => {
      const recovery = new ErrorRecoveryManager({
        initialBackoff: 1000,
        recoveryThreshold: 3,
      });

      // エラーでバックオフを増加
      recovery.recordError("test_error");
      recovery.recordError("test_error");
      expect(recovery.getBackoffTime()).toBe(4000);

      // 十分な成功でリセット
      recovery.recordSuccess();
      recovery.recordSuccess();
      recovery.recordSuccess();

      expect(recovery.getBackoffTime()).toBe(1000);
    });
  });

  describe("Retry Logic", () => {
    it("should prevent retry during backoff period", () => {
      mockTime = 1000;
      recovery.recordError("test_error");
      expect(recovery.canRetryNow()).toBe(false);

      // バックオフ期間を経過
      mockTime = 1000 + 10001; // 10秒+1ms経過（最初のエラーで5秒→10秒にバックオフが倍になる）
      expect(recovery.canRetryNow()).toBe(true);
    });

    it("should disable virtualization during backoff", () => {
      mockTime = 1000;
      recovery.recordError("test_error");
      expect(recovery.shouldDisableVirtualization()).toBe(true);

      // バックオフ期間経過後
      mockTime = 1000 + 10001; // 10秒+1ms経過（最初のエラーで5秒→10秒にバックオフが倍になる）
      expect(recovery.shouldDisableVirtualization()).toBe(false);
    });
  });

  describe("Configuration", () => {
    it("should use custom configuration", () => {
      let customMockTime = 1000;
      const customRecovery = new ErrorRecoveryManager(
        {
          initialBackoff: 2000,
          maxBackoff: 10000,
          recoveryThreshold: 5,
          errorThreshold: 2,
        },
        () => customMockTime,
      );

      expect(customRecovery.getBackoffTime()).toBe(2000);

      // 1回のエラーではまだ無効化されない
      customRecovery.recordError("test_error");
      expect(customRecovery.shouldDisableVirtualization()).toBe(true); // バックオフ期間中は無効化

      // 時間経過後は有効化
      customMockTime = 1000 + 4001; // 4秒+1ms経過（最初のエラーで2秒→4秒にバックオフが倍になる）
      expect(customRecovery.shouldDisableVirtualization()).toBe(false);

      // 2回目のエラーで閾値到達
      customRecovery.recordError("test_error");
      expect(customRecovery.shouldDisableVirtualization()).toBe(true);
    });

    it("should use default configuration when not provided", () => {
      let defaultMockTime = 1000;
      const defaultRecovery = new ErrorRecoveryManager(
        {},
        () => defaultMockTime,
      );

      expect(defaultRecovery.getBackoffTime()).toBe(5000); // デフォルト値

      // 2回のエラーでは閾値に達しない（バックオフ期間経過後）
      defaultRecovery.recordError("test_error");
      defaultMockTime = 1000 + 10001; // 10秒+1ms経過（最初のエラーで5秒→10秒にバックオフが倍になる）
      defaultRecovery.recordError("test_error");
      defaultMockTime = 1000 + 30001; // 30秒+1ms経過（2回目のエラーで10秒→20秒にバックオフが倍になる）
      expect(defaultRecovery.shouldDisableVirtualization()).toBe(false);

      // 3回目のエラーで閾値到達
      defaultRecovery.recordError("test_error");
      expect(defaultRecovery.shouldDisableVirtualization()).toBe(true);
    });
  });

  describe("State Management", () => {
    it("should provide current recovery status", () => {
      mockTime = 1000;
      recovery.recordError("test_error");
      recovery.recordSuccess();
      recovery.recordSuccess();

      // エラー時刻から十分時間が経過
      mockTime = 1000 + 10000;

      const status = recovery.getRecoveryStatus();
      expect(status.errorCount).toBe(0); // 成功でリセット
      expect(status.successCount).toBe(2);
      expect(status.backoffTime).toBe(10000); // エラー後にバックオフが2倍になる
      expect(status.canRetry).toBe(true);
    });

    it("should reset all state", () => {
      recovery.recordError("test_error");
      recovery.recordSuccess();

      recovery.reset();

      expect(recovery.getErrorCount()).toBe(0);
      expect(recovery.getSuccessCount()).toBe(0);
      expect(recovery.getBackoffTime()).toBe(5000); // デフォルト値に戻る
      expect(recovery.canRetryNow()).toBe(true);
    });
  });

  describe("Edge Cases", () => {
    it("should handle no errors gracefully", () => {
      expect(recovery.shouldDisableVirtualization()).toBe(false);
      expect(recovery.canRetryNow()).toBe(true);
      expect(recovery.getErrorCount()).toBe(0);
      expect(recovery.getSuccessCount()).toBe(0);
    });

    it("should handle rapid error/success cycles", () => {
      // 高速でエラーと成功を繰り返す
      for (let i = 0; i < 5; i++) {
        recovery.recordError("rapid_error");
        recovery.recordSuccess();
      }

      // 最後は成功なので、エラーカウントは0
      expect(recovery.getErrorCount()).toBe(0);
      expect(recovery.getSuccessCount()).toBe(1); // 最後の成功のみ残る
    });
  });
});
