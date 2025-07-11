import { ErrorRecoveryManager } from './errorRecovery';

export interface FeatureFlags {
  enableVirtualization: boolean | 'auto';
  forceVirtualization: boolean;
  enableDynamicSizing: boolean | 'auto';
  debugMode: boolean;
  virtualizationThreshold?: number;
}

export class FeatureFlagManager {
  private static instance: FeatureFlagManager;
  private flags: FeatureFlags;
  private recovery: ErrorRecoveryManager;

  private constructor() {
    this.flags = this.loadFlags();
    this.recovery = new ErrorRecoveryManager();
  }

  static getInstance(): FeatureFlagManager {
    if (!FeatureFlagManager.instance) {
      FeatureFlagManager.instance = new FeatureFlagManager();
    }
    return FeatureFlagManager.instance;
  }

  private loadFlags(): FeatureFlags {
    // Load from environment variables
    const envFlags = {
      enableVirtualization: process.env.NEXT_PUBLIC_ENABLE_VIRTUALIZATION || 'auto',
      forceVirtualization: process.env.NEXT_PUBLIC_FORCE_VIRTUALIZATION === 'true',
      enableDynamicSizing: process.env.NEXT_PUBLIC_ENABLE_DYNAMIC_SIZING || 'auto',
      debugMode: process.env.NODE_ENV === 'development',
      virtualizationThreshold: parseInt(process.env.NEXT_PUBLIC_VIRTUALIZATION_THRESHOLD || '200')
    };

    // Override from localStorage in debug mode
    if (typeof window !== 'undefined' && envFlags.debugMode) {
      const localFlags = localStorage.getItem('featureFlags');
      if (localFlags) {
        try {
          return { ...envFlags, ...JSON.parse(localFlags) };
        } catch (error) {
          console.warn('Failed to parse feature flags from localStorage:', error);
        }
      }
    }

    return envFlags;
  }

  getFlags(): FeatureFlags {
    return { ...this.flags };
  }

  updateFlags(updates: Partial<FeatureFlags>): void {
    this.flags = { ...this.flags, ...updates };
    
    // Save to localStorage in debug mode
    if (this.flags.debugMode && typeof window !== 'undefined') {
      try {
        localStorage.setItem('featureFlags', JSON.stringify(updates));
      } catch (error) {
        console.warn('Failed to save feature flags to localStorage:', error);
      }
    }
  }

  recordError(errorType: string): void {
    this.recovery.recordError(errorType);
    
    // 自動的に仮想化を無効化
    if (this.recovery.shouldDisableVirtualization()) {
      this.updateFlags({ enableVirtualization: false });
    }
  }

  recordSuccess(): void {
    this.recovery.recordSuccess();
    
    // 十分な成功履歴があれば仮想化を再有効化
    if (this.recovery.getSuccessCount() >= 10 && this.recovery.canRetryNow()) {
      this.updateFlags({ enableVirtualization: 'auto' });
    }
  }

  getRecoveryStatus(): {
    errorCount: number;
    successCount: number;
    backoffTime: number;
    canRetry: boolean;
  } {
    return {
      errorCount: this.recovery.getErrorCount(),
      successCount: this.recovery.getSuccessCount(),
      backoffTime: this.recovery.getBackoffTime(),
      canRetry: this.recovery.canRetryNow()
    };
  }
}

export function shouldUseVirtualization(
  itemCount: number,
  flags?: FeatureFlags
): boolean {
  const featureFlags = flags || FeatureFlagManager.getInstance().getFlags();
  
  // Force virtualization takes highest priority
  if (featureFlags.forceVirtualization) {
    return true;
  }

  // Explicit disable
  if (featureFlags.enableVirtualization === false) {
    return false;
  }

  // Explicit enable
  if (featureFlags.enableVirtualization === true) {
    return true;
  }

  // Auto mode - use threshold
  if (featureFlags.enableVirtualization === 'auto') {
    const threshold = featureFlags.virtualizationThreshold || 200;
    return itemCount >= threshold;
  }

  // Default to false for unknown values
  return false;
}