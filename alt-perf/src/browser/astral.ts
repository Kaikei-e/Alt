/**
 * Browser automation wrapper using Astral (Deno-native Puppeteer)
 */
import { launch, type Browser, type Page } from "@astral/astral";
import { DEVICE_PROFILES, type DeviceProfile } from "../config/schema.ts";
import { debug, info } from "../utils/logger.ts";

export interface BrowserOptions {
  headless: boolean;
  viewport?: { width: number; height: number };
  userAgent?: string;
}

export interface SessionCookie {
  name: string;
  value: string;
  domain: string;
  path: string;
  httpOnly?: boolean;
  sameSite?: "Lax" | "Strict" | "None";
  expires?: number;
}

/**
 * Browser manager for performance testing
 */
export class BrowserManager {
  private browser: Browser | null = null;
  private options: BrowserOptions;

  constructor(options: BrowserOptions = { headless: true }) {
    this.options = options;
  }

  /**
   * Launch browser instance
   */
  async launch(): Promise<Browser> {
    if (this.browser) {
      return this.browser;
    }

    // Use system Chromium if available (for Docker/Alpine compatibility)
    const chromePath = Deno.env.get("CHROME_BIN") ||
      Deno.env.get("PUPPETEER_EXECUTABLE_PATH");

    debug("Launching browser", {
      headless: this.options.headless,
      path: chromePath || "bundled",
    });

    this.browser = await launch({
      headless: this.options.headless,
      path: chromePath,
      args: [
        "--no-sandbox",
        "--disable-setuid-sandbox",
        "--disable-dev-shm-usage",
        "--disable-gpu",
        "--disable-software-rasterizer",
        "--disable-breakpad",
        "--headless=new",
        "--single-process",
      ],
    });

    info("Browser launched");
    return this.browser;
  }

  /**
   * Create a new page with device profile
   */
  async createPage(
    deviceName: string = "desktop-chrome",
    cookies?: SessionCookie[]
  ): Promise<Page> {
    if (!this.browser) {
      await this.launch();
    }

    const device = DEVICE_PROFILES[deviceName] || DEVICE_PROFILES["desktop-chrome"];
    debug("Creating page", { device: device.name });

    const page = await this.browser!.newPage();

    // Set viewport
    await page.setViewportSize({
      width: device.viewport.width,
      height: device.viewport.height,
    });

    // Set cookies if provided
    if (cookies && cookies.length > 0) {
      await page.setCookies(
        cookies.map((cookie) => ({
          name: cookie.name,
          value: cookie.value,
          domain: cookie.domain,
          path: cookie.path,
          expires: cookie.expires ?? -1,
          size: cookie.name.length + cookie.value.length,
          httpOnly: cookie.httpOnly ?? false,
          secure: false,
          session: cookie.expires === undefined,
          sameSite: cookie.sameSite ?? "Lax",
          priority: "Medium" as const,
          sameParty: false,
          sourceScheme: "NonSecure" as const,
          sourcePort: 80,
        }))
      );
      debug("Cookies set", { count: cookies.length });
    }

    return page;
  }

  /**
   * Navigate to URL and wait for load
   */
  async navigateTo(
    page: Page,
    url: string,
    options: { waitFor?: string; timeout?: number } = {}
  ): Promise<{ loadTime: number }> {
    const startTime = performance.now();

    debug("Navigating to", { url });

    await page.goto(url, {
      waitUntil: "networkidle2",
    });

    // Wait for specific element if specified
    if (options.waitFor) {
      await page.waitForSelector(options.waitFor, { timeout: 10000 });
    }

    const loadTime = performance.now() - startTime;
    debug("Page loaded", { url, loadTime: `${loadTime.toFixed(0)}ms` });

    return { loadTime };
  }

  /**
   * Close browser instance
   */
  async close(): Promise<void> {
    if (this.browser) {
      debug("Closing browser");
      await this.browser.close();
      this.browser = null;
      info("Browser closed");
    }
  }

  /**
   * Get device profile by name
   */
  getDeviceProfile(name: string): DeviceProfile {
    return DEVICE_PROFILES[name] || DEVICE_PROFILES["desktop-chrome"];
  }

  /**
   * List available device profiles
   */
  listDeviceProfiles(): string[] {
    return Object.keys(DEVICE_PROFILES);
  }
}

/**
 * Create a configured browser manager
 */
export function createBrowserManager(options?: BrowserOptions): BrowserManager {
  return new BrowserManager(options);
}

// Re-export types
export type { Browser, Page };
