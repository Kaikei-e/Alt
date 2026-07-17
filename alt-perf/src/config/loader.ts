/**
 * Configuration loader for alt-perf
 */
import { parse as parseYaml } from "@std/yaml";
import * as path from "@std/path";
import type {
  AuthConfig,
  FlowsConfig,
  PerfConfig,
  RouteConfig,
  RoutesConfig,
  ThresholdsConfig,
} from "./schema.ts";
import { DEFAULT_THRESHOLDS } from "./schema.ts";

// Environment variable expansion
function expandEnvVars(value: string): string {
  return value.replace(/\$\{([^}]+)\}/g, (_match, varName) => {
    const envValue = Deno.env.get(varName);
    if (envValue === undefined) {
      console.warn(`Warning: Environment variable ${varName} is not set`);
      return "";
    }
    return envValue;
  });
}

// Recursively expand environment variables in an object
function expandEnvVarsInObject<T>(obj: T): T {
  if (typeof obj === "string") {
    return expandEnvVars(obj) as T;
  }
  if (Array.isArray(obj)) {
    return obj.map((item) => expandEnvVarsInObject(item)) as T;
  }
  if (obj !== null && typeof obj === "object") {
    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      result[key] = expandEnvVarsInObject(value);
    }
    return result as T;
  }
  return obj;
}

// Thrown only when the config file itself is missing, so callers can fall
// back to defaults for that case while still surfacing YAML syntax errors,
// permission errors, etc. instead of silently masking a misconfiguration.
class ConfigFileNotFoundError extends Error {
  constructor(filePath: string) {
    super(`Configuration file not found: ${filePath}`);
    this.name = "ConfigFileNotFoundError";
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isRouteConfig(value: unknown): value is RouteConfig {
  if (!isRecord(value)) return false;
  return typeof value.path === "string" && typeof value.name === "string";
}

function isRouteConfigArray(value: unknown): value is RouteConfig[] {
  return Array.isArray(value) && value.every(isRouteConfig);
}

function isRoutesConfig(value: unknown): value is RoutesConfig {
  if (!isRecord(value)) return false;
  const groups = ["public", "desktop", "mobile", "sveltekit", "api"] as const;
  for (const key of groups) {
    if (value[key] !== undefined && !isRouteConfigArray(value[key])) {
      return false;
    }
  }
  return true;
}

function isThresholdConfig(value: unknown): value is { good: number; poor: number } {
  return (
    isRecord(value) &&
    typeof value.good === "number" &&
    typeof value.poor === "number"
  );
}

function isThresholdsConfig(value: unknown): value is ThresholdsConfig {
  if (!isRecord(value) || !isRecord(value.vitals) || !isRecord(value.scoring)) {
    return false;
  }
  const vitals = value.vitals;
  for (const key of ["lcp", "inp", "cls", "fcp", "ttfb"] as const) {
    if (!isThresholdConfig(vitals[key])) return false;
  }
  return typeof value.scoring.passThreshold === "number";
}

function isFlowsConfig(value: unknown): value is FlowsConfig {
  if (!isRecord(value)) return false;
  if (value.flows === undefined) return true;
  return Array.isArray(value.flows);
}

// Load YAML file with optional schema predicate
async function loadYamlFile<T>(
  filePath: string,
  guard?: (value: unknown) => value is T,
): Promise<T> {
  try {
    const content = await Deno.readTextFile(filePath);
    const parsed: unknown = parseYaml(content);
    if (guard && !guard(parsed)) {
      throw new Error(`Invalid configuration schema in ${filePath}`);
    }
    return expandEnvVarsInObject(parsed as T);
  } catch (error) {
    if (error instanceof Deno.errors.NotFound) {
      throw new ConfigFileNotFoundError(filePath);
    }
    throw error;
  }
}

// Default routes configuration
function getDefaultRoutes(): RoutesConfig {
  return {
    public: [
      { path: "/", name: "Root Redirect" },
      { path: "/auth/login", name: "Login Page" },
      { path: "/auth/register", name: "Registration Page" },
      { path: "/public/landing", name: "Public Landing" },
    ],
    desktop: [
      { path: "/desktop/home", name: "Desktop Home", requiresAuth: true },
      { path: "/desktop/feeds", name: "Desktop Feeds", requiresAuth: true },
      {
        path: "/desktop/feeds/register",
        name: "Desktop Feed Registration",
        requiresAuth: true,
      },
      { path: "/desktop/articles", name: "Desktop Articles", requiresAuth: true },
      {
        path: "/desktop/articles/search",
        name: "Desktop Article Search",
        requiresAuth: true,
      },
      { path: "/desktop/settings", name: "Desktop Settings", requiresAuth: true },
    ],
    mobile: [
      {
        path: "/mobile/feeds",
        name: "Mobile Feeds",
        requiresAuth: true,
        priority: "high",
      },
      {
        path: "/mobile/feeds/manage",
        name: "Mobile Feed Management",
        requiresAuth: true,
      },
      { path: "/mobile/feeds/swipe", name: "Mobile Swipe View", requiresAuth: true },
      {
        path: "/mobile/feeds/favorites",
        name: "Mobile Favorites",
        requiresAuth: true,
      },
      { path: "/mobile/feeds/stats", name: "Mobile Stats", requiresAuth: true },
      {
        path: "/mobile/feeds/viewed",
        name: "Mobile Viewed Articles",
        requiresAuth: true,
      },
      {
        path: "/mobile/articles/view",
        name: "Mobile Article View",
        requiresAuth: true,
      },
      {
        path: "/mobile/articles/search",
        name: "Mobile Article Search",
        requiresAuth: true,
      },
      {
        path: "/mobile/recap/morning-letter",
        name: "Morning Letter",
        requiresAuth: true,
      },
      { path: "/mobile/recap/7days", name: "7-Day Recap", requiresAuth: true },
    ],
    sveltekit: [
      { path: "/sv/", name: "SvelteKit Root" },
      { path: "/sv/auth/login", name: "SvelteKit Login" },
    ],
    api: [
      { path: "/api/health", name: "Frontend Health", type: "api" },
      { path: "/api/backend/v1/health", name: "Backend Health", type: "api" },
    ],
  };
}

// Default auth configuration
function getDefaultAuth(): AuthConfig {
  const baseUrl = Deno.env.get("PERF_BASE_URL") || "http://localhost";
  return {
    enabled: true,
    kratosUrl: `${baseUrl}/ory`,
    authHubUrl: `${baseUrl}/api/auth`,
    credentials: {
      // Support PERF_TEST_*, TEST_EMAIL/TEST_PASSWORD environment variables
      email: Deno.env.get("PERF_TEST_EMAIL") || Deno.env.get("TEST_EMAIL") || Deno.env.get("TEST_USER") || "",
      password: Deno.env.get("PERF_TEST_PASSWORD") || Deno.env.get("TEST_PASSWORD") || "",
    },
  };
}

// Load routes configuration
export async function loadRoutesConfig(configDir: string): Promise<RoutesConfig> {
  const routesPath = path.join(configDir, "routes.yaml");
  try {
    const config = await loadYamlFile<{ routes: RoutesConfig }>(
      routesPath,
      (value): value is { routes: RoutesConfig } =>
        isRecord(value) &&
        (value.routes === undefined || isRoutesConfig(value.routes)),
    );
    return config.routes || getDefaultRoutes();
  } catch (error) {
    if (!(error instanceof ConfigFileNotFoundError)) {
      throw error;
    }
    console.log("Using default routes configuration");
    return getDefaultRoutes();
  }
}

// Load thresholds configuration
export async function loadThresholdsConfig(
  configDir: string
): Promise<ThresholdsConfig> {
  const thresholdsPath = path.join(configDir, "thresholds.yaml");
  try {
    return await loadYamlFile<ThresholdsConfig>(thresholdsPath, isThresholdsConfig);
  } catch (error) {
    if (!(error instanceof ConfigFileNotFoundError)) {
      throw error;
    }
    console.log("Using default thresholds configuration");
    return DEFAULT_THRESHOLDS;
  }
}

// Load flows configuration
export async function loadFlowsConfig(configDir: string): Promise<FlowsConfig> {
  const flowsPath = path.join(configDir, "flows.yaml");
  try {
    return await loadYamlFile<FlowsConfig>(flowsPath, isFlowsConfig);
  } catch (error) {
    if (!(error instanceof ConfigFileNotFoundError)) {
      throw error;
    }
    console.log("No flows configuration found");
    return { flows: [] };
  }
}

// Load full configuration
export async function loadConfig(configDir: string): Promise<PerfConfig> {
  const [routes, thresholds, flows] = await Promise.all([
    loadRoutesConfig(configDir),
    loadThresholdsConfig(configDir),
    loadFlowsConfig(configDir),
  ]);

  return {
    baseUrl: Deno.env.get("PERF_BASE_URL") || "http://localhost",
    devices: ["desktop-chrome", "mobile-chrome"],
    auth: getDefaultAuth(),
    routes,
    thresholds,
    flows,
  };
}

// Export for testing
export { expandEnvVars, expandEnvVarsInObject };
