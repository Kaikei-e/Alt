/**
 * Network throttling and CPU throttling profiles
 * Uses Chrome DevTools Protocol for realistic network simulation
 */
import type { Page } from "@astral/astral";
import { debug, info, warn } from "../utils/logger.ts";

/**
 * Network condition definition
 */
export interface NetworkCondition {
  /** Display name */
  name: string;
  /** Download throughput in bytes per second */
  downloadThroughput: number;
  /** Upload throughput in bytes per second */
  uploadThroughput: number;
  /** Round-trip latency in milliseconds */
  latency: number;
  /** Whether the network is offline */
  offline?: boolean;
}

/**
 * CPU throttling rate
 * 1 = no throttling, 4 = 4x slowdown
 */
export interface CPUThrottling {
  /** Throttling rate */
  rate: number;
}

/**
 * Preset network conditions based on Chrome DevTools profiles
 */
export const NETWORK_PRESETS: Record<string, NetworkCondition> = {
  "fast-3g": {
    name: "Fast 3G",
    downloadThroughput: (1.5 * 1024 * 1024) / 8, // 1.5 Mbps
    uploadThroughput: (750 * 1024) / 8, // 750 Kbps
    latency: 100,
  },
  "slow-3g": {
    name: "Slow 3G",
    downloadThroughput: (780 * 1024) / 8, // 780 Kbps
    uploadThroughput: (330 * 1024) / 8, // 330 Kbps
    latency: 400,
  },
  "4g": {
    name: "4G LTE",
    downloadThroughput: (12 * 1024 * 1024) / 8, // 12 Mbps
    uploadThroughput: (2 * 1024 * 1024) / 8, // 2 Mbps
    latency: 50,
  },
  "wifi-slow": {
    name: "Slow WiFi",
    downloadThroughput: (5 * 1024 * 1024) / 8, // 5 Mbps
    uploadThroughput: (1 * 1024 * 1024) / 8, // 1 Mbps
    latency: 20,
  },
  "wifi-fast": {
    name: "Fast WiFi",
    downloadThroughput: (50 * 1024 * 1024) / 8, // 50 Mbps
    uploadThroughput: (10 * 1024 * 1024) / 8, // 10 Mbps
    latency: 5,
  },
  offline: {
    name: "Offline",
    downloadThroughput: 0,
    uploadThroughput: 0,
    latency: 0,
    offline: true,
  },
};

/**
 * CPU throttling presets
 */
export const CPU_PRESETS: Record<string, CPUThrottling> = {
  "no-throttle": { rate: 1 },
  "mid-tier-mobile": { rate: 4 },
  "low-end-mobile": { rate: 6 },
};

/**
 * Network controller for applying network and CPU conditions
 */
export class NetworkController {
  private currentCondition: NetworkCondition | null = null;
  private currentCPUThrottling: CPUThrottling | null = null;

  /**
   * Apply network conditions to the page using CDP
   */
  async applyConditions(page: Page, condition: NetworkCondition): Promise<void> {
    debug("Applying network conditions", {
      name: condition.name,
      download: `${((condition.downloadThroughput * 8) / 1024 / 1024).toFixed(1)} Mbps`,
      latency: `${condition.latency}ms`,
    });

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        // Enable Network domain
        await cdpSession.send("Network.enable", {});

        // Apply network conditions
        await cdpSession.send("Network.emulateNetworkConditions", {
          offline: condition.offline ?? false,
          latency: condition.latency,
          downloadThroughput: condition.downloadThroughput,
          uploadThroughput: condition.uploadThroughput,
        });

        this.currentCondition = condition;
        info(`Network condition applied: ${condition.name}`);
      } else {
        warn("CDP session not available, network throttling skipped");
      }
    } catch (error) {
      warn("Failed to apply network conditions", { error: String(error) });
      throw error;
    }
  }

  /**
   * Apply a preset network condition by name
   */
  async applyPreset(page: Page, presetName: string): Promise<void> {
    const condition = NETWORK_PRESETS[presetName];
    if (!condition) {
      throw new Error(`Unknown network preset: ${presetName}`);
    }
    await this.applyConditions(page, condition);
  }

  /**
   * Apply CPU throttling to the page using CDP
   */
  async applyCPUThrottling(page: Page, throttling: CPUThrottling): Promise<void> {
    debug("Applying CPU throttling", { rate: `${throttling.rate}x` });

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        await cdpSession.send("Emulation.setCPUThrottlingRate", {
          rate: throttling.rate,
        });

        this.currentCPUThrottling = throttling;
        info(`CPU throttling applied: ${throttling.rate}x slowdown`);
      } else {
        warn("CDP session not available, CPU throttling skipped");
      }
    } catch (error) {
      warn("Failed to apply CPU throttling", { error: String(error) });
      throw error;
    }
  }

  /**
   * Apply a preset CPU throttling by name
   */
  async applyCPUPreset(page: Page, presetName: string): Promise<void> {
    const throttling = CPU_PRESETS[presetName];
    if (!throttling) {
      throw new Error(`Unknown CPU preset: ${presetName}`);
    }
    await this.applyCPUThrottling(page, throttling);
  }

  /**
   * Clear all network conditions (reset to no throttling)
   */
  async clearConditions(page: Page): Promise<void> {
    debug("Clearing network conditions");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        // Reset network conditions
        await cdpSession.send("Network.emulateNetworkConditions", {
          offline: false,
          latency: 0,
          downloadThroughput: -1, // -1 means no throttling
          uploadThroughput: -1,
        });

        this.currentCondition = null;
        info("Network conditions cleared");
      }
    } catch (error) {
      warn("Failed to clear network conditions", { error: String(error) });
    }
  }

  /**
   * Clear CPU throttling
   */
  async clearCPUThrottling(page: Page): Promise<void> {
    debug("Clearing CPU throttling");

    try {
      const cdpSession = await this.getCDPSession(page);

      if (cdpSession) {
        await cdpSession.send("Emulation.setCPUThrottlingRate", { rate: 1 });
        this.currentCPUThrottling = null;
        info("CPU throttling cleared");
      }
    } catch (error) {
      warn("Failed to clear CPU throttling", { error: String(error) });
    }
  }

  /**
   * Clear all throttling (network and CPU)
   */
  async clearAll(page: Page): Promise<void> {
    await this.clearConditions(page);
    await this.clearCPUThrottling(page);
  }

  /**
   * Get the current network condition
   */
  getCurrentCondition(): NetworkCondition | null {
    return this.currentCondition;
  }

  /**
   * Get the current CPU throttling
   */
  getCurrentCPUThrottling(): CPUThrottling | null {
    return this.currentCPUThrottling;
  }

  /**
   * Get list of available network presets
   */
  static getNetworkPresets(): string[] {
    return Object.keys(NETWORK_PRESETS);
  }

  /**
   * Get list of available CPU presets
   */
  static getCPUPresets(): string[] {
    return Object.keys(CPU_PRESETS);
  }

  /**
   * Get CDP session from page (internal helper)
   */
  private async getCDPSession(page: Page): Promise<CDPSession | null> {
    try {
      // Access CDP session through Astral's internal API
      const session = await (
        page as unknown as { unsafelyGetCDPSession(): Promise<CDPSession> }
      ).unsafelyGetCDPSession?.();
      return session || null;
    } catch {
      return null;
    }
  }
}

/**
 * CDP session interface (minimal)
 */
interface CDPSession {
  send(method: string, params?: Record<string, unknown>): Promise<unknown>;
}

/**
 * Create a network controller instance
 */
export function createNetworkController(): NetworkController {
  return new NetworkController();
}
