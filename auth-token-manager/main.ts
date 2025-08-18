/**
 * Main entry point for auth-token-manager
 * Automated OAuth token refresh system using refresh tokens only - Deno 2.0
 */

import { config } from "./src/utils/config.ts";
import { InoreaderTokenManager } from "./src/auth/oauth.ts";
import { K8sSecretManager } from "./src/k8s/secret-manager-simple.ts";
import {
  StructuredLogger,
} from "./src/utils/logger.ts";

// Initialize structured logging with sanitization
const logger = new StructuredLogger("auth-token-manager");
logger.info("Starting auth-token-manager v2.0.0 (refresh-token-only mode)");

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
      case "authorize":
        await runAuthorization();
        break;
      case "refresh":
        await runTokenRefresh();
        break;
      case "health":
        await runHealthCheck();
        break;
      case "validate":
        await runValidation();
        break;
      case "monitor":
        await runTokenMonitoring();
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

    const tokenManager = new InoreaderTokenManager(
      credentials,
      configOptions.network,
      configOptions.retry,
      configOptions.kubernetes_namespace,
      configOptions.secret_name,
    );

    logger.info("Initializing token manager");
    await tokenManager.initialize();

    logger.info("Refreshing access token");
    const result = await tokenManager.refreshAccessToken();

    if (!result.success || !result.tokens) {
      throw new Error(`Token refresh failed: ${result.error || 'Unknown error'}`);
    }

    logger.info("Storing tokens to Kubernetes secret");
    const secretManager = new K8sSecretManager(
      configOptions.kubernetes_namespace,
      configOptions.secret_name,
    );

    await secretManager.updateTokenSecret(result.tokens);

    logger.info("Token refresh completed successfully");

    // No cleanup required - browser automation removed
  } catch (error) {
    logger.error("Token refresh failed");
    throw error;
  }
}

async function runHealthCheck() {
  logger.info("Running health check");

  try {
    const checks = {
      config_valid: false,
      environment_ready: false,
      kubernetes_ready: false,
      refresh_token_available: false,
      token_expiry_status: false,
    };

    // Check configuration
    checks.config_valid = config.validateConfig();
    
    // Check environment readiness
    checks.environment_ready = Boolean(
      Deno.env.get('INOREADER_CLIENT_ID') && 
      Deno.env.get('INOREADER_CLIENT_SECRET')
    );

    // Check Kubernetes connectivity
    try {
      const configOptions = await config.loadConfig();
      const secretManager = new K8sSecretManager(
        configOptions.kubernetes_namespace,
        configOptions.secret_name,
      );
      
      // Try to access the secret to test K8s connectivity
      const tokenData = await secretManager.getTokenSecret();
      checks.kubernetes_ready = true;
      
      // Check if refresh token exists and is valid
      if (tokenData && tokenData.refresh_token) {
        checks.refresh_token_available = tokenData.refresh_token.length > 10;
        
        // Check token expiry status
        if (tokenData.expires_at) {
          const expiresAt = new Date(tokenData.expires_at);
          const now = new Date();
          const timeUntilExpiry = expiresAt.getTime() - now.getTime();
          const oneHour = 60 * 60 * 1000;
          
          checks.token_expiry_status = timeUntilExpiry > oneHour;
          
          logger.info("Token expiry check", {
            expires_at: expiresAt.toISOString(),
            time_until_expiry_hours: Math.round(timeUntilExpiry / 1000 / 3600 * 10) / 10,
            needs_refresh_soon: timeUntilExpiry < oneHour,
          });
        }
      }
    } catch (error) {
      logger.warn("Kubernetes connectivity check failed", {
        error: error instanceof Error ? error.message : String(error),
      });
    }

    const healthyChecks = Object.values(checks).filter(Boolean).length;
    const totalChecks = Object.keys(checks).length;

    let status: "healthy" | "degraded" | "unhealthy";
    if (healthyChecks === totalChecks) {
      status = "healthy";
    } else if (healthyChecks >= 3) {  // At least config, env, and k8s should be working
      status = "degraded";
    } else {
      status = "unhealthy";
    }

    logger.info("Health check completed", {
      status,
      passing: healthyChecks,
      total: totalChecks,
      checks,
    });

    if (status === "unhealthy") {
      logger.error("Health check failed - service is unhealthy");
      Deno.exit(1);
    }
  } catch (error) {
    logger.error("Health check failed", {
      error: error instanceof Error ? error.message : String(error),
    });
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

/**
 * Perform initial OAuth2 authorization flow to obtain tokens.
 * Starts a local HTTP server to receive the authorization code,
 * exchanges it for tokens, and updates the Kubernetes secret.
 */
async function runAuthorization() {
  console.log("🔐 Starting initial OAuth2 authorization flow");
  // Load configuration and credentials
  await config.loadConfig();
  if (!config.validateConfig()) {
    console.error("Configuration validation failed");
    Deno.exit(1);
  }
  const credentials = config.getInoreaderCredentials();
  const redirectUrl = new URL(credentials.redirect_uri);
  // Construct authorization URL
  const authUrl = new URL("https://www.inoreader.com/oauth2/auth");
  authUrl.searchParams.set("client_id", credentials.client_id);
  authUrl.searchParams.set("redirect_uri", credentials.redirect_uri);
  authUrl.searchParams.set("response_type", "code");
  authUrl.searchParams.set("scope", "read write");
  console.log(`🔗 Open the following URL in your browser:
  ${authUrl.toString()}`);

  // Start HTTP server to capture the callback
  const listener = Deno.listen({
    hostname: redirectUrl.hostname,
    port: Number(redirectUrl.port || 80),
  });
  console.log(`🛡️  Listening for OAuth callback at ${credentials.redirect_uri}`);

  for await (const conn of listener) {
    const httpConn = Deno.serveHttp(conn);
    for await (const event of httpConn) {
      const reqUrl = new URL(event.request.url);
      if (reqUrl.pathname === redirectUrl.pathname && reqUrl.searchParams.has("code")) {
        const code = reqUrl.searchParams.get("code")!;
        event.respondWith(
          new Response("Authorization successful! You may close this tab.", {
            status: 200,
            headers: { "Content-Type": "text/plain" },
          }),
        );
        listener.close();
        // Exchange authorization code for tokens
        const tokenResp = await fetch("https://www.inoreader.com/oauth2/token", {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded" },
          body: new URLSearchParams({
            grant_type: "authorization_code",
            client_id: credentials.client_id,
            client_secret: credentials.client_secret,
            redirect_uri: credentials.redirect_uri,
            code,
          }),
        });
        if (!tokenResp.ok) {
          const errText = await tokenResp.text();
          console.error("Token exchange failed:", errText);
          Deno.exit(1);
        }
        const data = await tokenResp.json();
        if (!data.access_token || !data.refresh_token || !data.expires_in) {
          console.error("Invalid token response:", data);
          Deno.exit(1);
        }
        const expiresAt = new Date(Date.now() + Number(data.expires_in) * 1000);
        const tokens = {
          access_token: data.access_token,
          refresh_token: data.refresh_token,
          expires_at: expiresAt,
          token_type: data.token_type || "Bearer",
          scope: data.scope || "read write",
        };
        // Store tokens in Kubernetes secret
        const configOptions = await config.loadConfig();
        const secretManager = new K8sSecretManager(
          configOptions.kubernetes_namespace,
          configOptions.secret_name,
        );
        await secretManager.updateTokenSecret(tokens);
        console.log("✅ Initial OAuth2 flow completed and secret updated");
        Deno.exit(0);
      } else {
        event.respondWith(new Response("Invalid OAuth callback request", { status: 400 }));
      }
    }
  }
}

async function runTokenMonitoring() {
  logger.info("Running token monitoring and alerting check");

  try {
    const configOptions = await config.loadConfig();
    const secretManager = new K8sSecretManager(
      configOptions.kubernetes_namespace,
      configOptions.secret_name,
    );

    // Get current token data
    const tokenData = await secretManager.getTokenSecret();
    if (!tokenData) {
      logger.error("No token data found - OAuth setup required", {
        alert_level: "critical",
        action_required: "manual_oauth_setup",
      });
      Deno.exit(1);
    }

    const now = new Date();
    const updatedAt = tokenData.updated_at ? new Date(tokenData.updated_at) : null;
    const expiresAt = tokenData.expires_at ? new Date(tokenData.expires_at) : null;

    // Calculate time metrics
    let timeUntilExpiry = 0;
    let timeSinceUpdate = 0;
    
    if (expiresAt) {
      timeUntilExpiry = expiresAt.getTime() - now.getTime();
    }
    
    if (updatedAt) {
      timeSinceUpdate = now.getTime() - updatedAt.getTime();
    }

    // Determine alert levels
    const alerts = [];
    let alertLevel: "info" | "warning" | "critical" = "info";

    // Check token expiry - Enhanced thresholds for proactive management
    const oneHour = 60 * 60 * 1000;
    const thirtyMinutes = 30 * 60 * 1000;
    const fiveMinutes = 5 * 60 * 1000;
    const twoHours = 2 * 60 * 60 * 1000;  // 恒久対応: 2時間前の早期警告
    const sixHours = 6 * 60 * 60 * 1000;  // 恒久対応: 6時間前の事前通知

    if (timeUntilExpiry <= 0) {
      alerts.push("Token has already expired - immediate refresh required");
      alertLevel = "critical";
    } else if (timeUntilExpiry < fiveMinutes) {
      alerts.push("Token expires in less than 5 minutes - immediate refresh required");
      alertLevel = "critical";
    } else if (timeUntilExpiry < thirtyMinutes) {
      alerts.push("Token expires in less than 30 minutes - urgent refresh recommended");
      alertLevel = "critical";  // 恒久対応: 30分以内は critical に変更
    } else if (timeUntilExpiry < oneHour) {
      alerts.push("Token expires in less than 1 hour - refresh recommended");
      alertLevel = "warning";
    } else if (timeUntilExpiry < twoHours) {
      alerts.push("Token expires in less than 2 hours - proactive refresh recommended");
      if (alertLevel === "info") alertLevel = "warning";
    } else if (timeUntilExpiry < sixHours) {
      alerts.push("Token expires in less than 6 hours - preparation recommended");
    }

    // Check last update time
    const oneDayMs = 24 * 60 * 60 * 1000;
    const twelveHoursMs = 12 * 60 * 60 * 1000;
    const sixHoursMs = 6 * 60 * 60 * 1000;

    if (timeSinceUpdate > oneDayMs) {
      alerts.push("Token hasn't been updated in over 24 hours - system may be failing");
      alertLevel = "critical";
    } else if (timeSinceUpdate > twelveHoursMs) {
      alerts.push("Token hasn't been updated in over 12 hours - check CronJob status");
      if (alertLevel === "info") alertLevel = "warning";
    } else if (timeSinceUpdate > sixHoursMs) {
      alerts.push("Token hasn't been updated in over 6 hours - monitoring recommended");
    }

    // Check refresh token validity
    if (!tokenData.refresh_token || tokenData.refresh_token.length < 10) {
      alerts.push("Invalid or missing refresh token - manual OAuth setup required");
      alertLevel = "critical";
    }

    // Prepare monitoring data
    const monitoringData = {
      timestamp: now.toISOString(),
      alert_level: alertLevel,
      alerts: alerts,
      token_status: {
        has_access_token: Boolean(tokenData.access_token),
        has_refresh_token: Boolean(tokenData.refresh_token),
        expires_at: expiresAt?.toISOString() || null,
        updated_at: updatedAt?.toISOString() || null,
        time_until_expiry_hours: expiresAt ? Math.round(timeUntilExpiry / 1000 / 3600 * 100) / 100 : null,
        time_since_update_hours: updatedAt ? Math.round(timeSinceUpdate / 1000 / 3600 * 100) / 100 : null,
        needs_immediate_refresh: timeUntilExpiry < fiveMinutes,
        needs_refresh_soon: timeUntilExpiry < thirtyMinutes,
      },
      system_status: {
        kubernetes_accessible: true,
        secret_exists: true,
        configuration_valid: config.validateConfig(),
      },
    };

    // Log monitoring results
    if (alertLevel === "critical") {
      logger.error("Token monitoring - CRITICAL alerts detected", monitoringData);
    } else if (alertLevel === "warning") {
      logger.warn("Token monitoring - WARNING alerts detected", monitoringData);
    } else {
      logger.info("Token monitoring - All systems operational", monitoringData);
    }

    // Exit with appropriate code for monitoring systems
    if (alertLevel === "critical") {
      Deno.exit(2); // Critical exit code
    } else if (alertLevel === "warning") {
      Deno.exit(1); // Warning exit code
    }
    // Exit 0 for success (info level)

  } catch (error) {
    logger.error("Token monitoring failed", {
      alert_level: "critical",
      error: error instanceof Error ? error.message : String(error),
    });
    Deno.exit(2);
  }
}

function showHelp() {
  console.log(`
🤖 Auth Token Manager v2.0.0
Automated OAuth token refresh system using refresh tokens only - Deno 2.0

USAGE:
  deno run --allow-all main.ts [COMMAND]

  COMMANDS:
  authorize  Perform initial OAuth2 authorization flow
  refresh    Refresh OAuth tokens
  health     Run health check (default)
  validate   Validate configuration
  monitor    Run token monitoring with alerting
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
