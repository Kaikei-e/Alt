/**
 * Main entry point for auth-token-manager
 * Automated OAuth token refresh system using Playwright + Deno 2.0
 */

import { config } from "./src/utils/config.ts";
import { InoreaderOAuthAutomator } from "./src/auth/oauth.ts";
import { K8sSecretManager } from "./src/k8s/secret-manager-simple.ts";
import {
  initializeLogging,
  StructuredLogger,
} from "./src/utils/logger.ts";

// Initialize structured logging with sanitization
await initializeLogging();
const logger = new StructuredLogger("auth-token-manager");
logger.info("Starting auth-token-manager");

async function main() {
  try {
    // Load and validate configuration
    await config.loadConfig();
    logger.info("Configuration loaded");

    if (!config.validateConfig()) {
      logger.error("Configuration validation failed");
      Deno.exit(1);
    }
    logger.info("Configuration validation successful");

    // Get command from arguments
    const command = Deno.args[0] || "health";

    switch (command) {
      case "refresh":
        await runTokenRefresh();
        break;
      case "health":
        await runHealthCheck();
        break;
      case "validate":
        await runValidation();
        break;
      case "help":
        showHelp();
        break;
      default:
        logger.error("Unknown command", { command });
        showHelp();
        Deno.exit(1);
    }
  } catch {
    logger.error("Critical error during startup");
    Deno.exit(1);
  }
}

async function runTokenRefresh() {
  logger.info("Starting token refresh");

  try {
    const configOptions = await config.loadConfig();
    const credentials = config.getInoreaderCredentials();

    const oauthAutomator = new InoreaderOAuthAutomator(
      credentials,
      configOptions.browser,
      configOptions.network,
      configOptions.retry,
    );

    logger.info("Initializing browser automation");
    await oauthAutomator.initializeBrowser();

    logger.info("Performing OAuth flow");
    const result = await oauthAutomator.performOAuth();

    if (!result.success || !result.tokens) {
      throw new Error("OAuth failed");
    }

    logger.info("Storing tokens to Kubernetes secret");
    const secretManager = new K8sSecretManager(
      configOptions.kubernetes_namespace,
      configOptions.secret_name,
    );

    await secretManager.updateTokenSecret(result.tokens);

    logger.info("Token refresh completed successfully");

    await oauthAutomator.cleanup();
  } catch (error) {
    logger.error("Token refresh failed");
    throw error;
  }
}

async function runHealthCheck() {
  logger.info("Running health check");

  try {
    const checks = {
      config_valid: config.validateConfig(),
      environment_ready: true,
      kubernetes_ready: false, // TODO: check K8s connectivity
      oauth_automation_ready: false, // TODO: check browser/playwright
    };

    const healthyChecks = Object.values(checks).filter(Boolean).length;
    const totalChecks = Object.keys(checks).length;

    let status: "healthy" | "degraded" | "unhealthy";
    if (healthyChecks === totalChecks) {
      status = "healthy";
    } else if (healthyChecks > 0) {
      status = "degraded";
    } else {
      status = "unhealthy";
    }

    logger.info("Health check completed", {
      status,
      passing: healthyChecks,
      total: totalChecks,
    });

    if (status === "unhealthy") {
      Deno.exit(1);
    }
  } catch (error) {
    logger.error("Health check failed");
    throw error;
  }
}

async function runValidation() {
  logger.info("Running configuration validation");

  try {
    await config.loadConfig();
    logger.info("Configuration validation completed successfully");
  } catch (error) {
    logger.error("Configuration validation failed");
    throw error;
  }
}

function showHelp() {
  console.log(`
🤖 Auth Token Manager v2.0.0
Automated OAuth token refresh system using Playwright + Deno 2.0

USAGE:
  deno run --allow-all main.ts [COMMAND]

COMMANDS:
  refresh    Refresh OAuth tokens (default)
  health     Run health check
  validate   Validate configuration
  help       Show this help message
`);
}

// Handle graceful shutdown
function setupSignalHandlers() {
  const signals: Deno.Signal[] = ["SIGINT", "SIGTERM"];

  for (const signal of signals) {
    Deno.addSignalListener(signal, () => {
      logger.info("Received termination signal", { signal });
      Deno.exit(0);
    });
  }
}

// Error boundary
globalThis.addEventListener("error", () => {
  logger.error("Unhandled error");
  Deno.exit(1);
});

globalThis.addEventListener("unhandledrejection", () => {
  logger.error("Unhandled promise rejection");
  Deno.exit(1);
});

// Setup signal handlers
setupSignalHandlers();

// Run main function
if (import.meta.main) {
  await main();
}

