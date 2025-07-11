export interface FeatureFlags {
  enableVirtualization: boolean | 'auto';
  forceVirtualization: boolean;
  debugMode: boolean;
  virtualizationThreshold?: number;
}

export class FeatureFlagManager {
  private static instance: FeatureFlagManager;
  private flags: FeatureFlags;

  private constructor() {
    this.flags = this.loadFlags();
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