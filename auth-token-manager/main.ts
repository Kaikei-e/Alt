/**
 * Main entry point for auth-token-manager
 * DI container and CLI routing only
 */

import { config } from "./src/infra/config.ts";
import { DataSanitizer, initializeOTel, shutdownOTel, StructuredLogger } from "./src/infra/logger.ts";
import { EnvFileSecretManager } from "./src/gateway/env_file_secret_manager.ts";
import { FetchHttpClient } from "./src/gateway/fetch_http_client.ts";
import { InoreaderTokenClient } from "./src/gateway/inoreader_token_client.ts";
import { RefreshTokenUsecase } from "./src/usecase/refresh_token.ts";
import { HealthCheckUsecase } from "./src/usecase/health_check.ts";
import { MonitorTokenUsecase } from "./src/usecase/monitor_token.ts";
import { AuthorizeUsecase } from "./src/usecase/authorize.ts";
import { GetTokenUsecase } from "./src/usecase/get_token.ts";
import { OAuthServer } from "./src/handler/oauth_server.ts";
import { DaemonLoop } from "./src/handler/daemon.ts";
import { CliHandler } from "./src/handler/cli.ts";

// StructuredLogger constructor is now pure (no OTel side-effects at import time).
// The module-level `logger` singleton in each src file is therefore safe to import
// for all commands.  initializeOTel() is called explicitly below only for commands
// that export telemetry.  The health fast-path never calls it.

function emergencyLog(msg: string, detail: Record<string, unknown>): void {
  // Sanitize the full payload before serialising to stderr so that secrets
  // passed in `detail` (e.g. error messages containing token snippets) are
  // redacted by the same rules as structured log output.
  const sanitized = DataSanitizer.sanitize({ level: "error", msg, ...detail });
  console.error(JSON.stringify(sanitized));
}

async function main() {
  const command = Deno.args[0] || "daemon";

  // Health command: skip OTel entirely.  No RASK_INGEST_TOKEN_FILE read,
  // no --allow-net to the OTLP collector.  The shared `logger` singleton from
  // each imported module is still available – it just won't emit OTel spans.
  if (command === "health") {
    const configOptions = await config.loadConfig();
    const secretManager = new EnvFileSecretManager(
      configOptions.token_storage_path,
    );
    const healthUsecase = new HealthCheckUsecase(secretManager);
    const result = await healthUsecase.execute();
    if (result.status === "unhealthy") {
      emergencyLog("Health check failed - service is unhealthy", {});
      Deno.exit(1);
    }
    return;
  }

  // All other commands: initialise OTel BEFORE constructing the rest of the DI
  // graph.  If RASK_INGEST_TOKEN_FILE is absent or invalid this throws and the
  // process exits cleanly – no outbound traffic is attempted first.
  initializeOTel();

  const logger = new StructuredLogger("auth-token-manager");

  try {
    const configOptions = await config.loadConfig();

    if (!config.validateConfig()) {
      logger.error("Configuration validation failed");
      Deno.exit(1);
    }

    const credentials = config.getInoreaderCredentials();

    // Gateway layer
    const secretManager = new EnvFileSecretManager(
      configOptions.token_storage_path,
    );
    const httpClient = new FetchHttpClient(configOptions.network);
    const tokenClient = new InoreaderTokenClient(credentials, httpClient);

    // Usecase layer
    const refreshUsecase = new RefreshTokenUsecase(
      tokenClient,
      secretManager,
      httpClient,
      configOptions.network,
      configOptions.retry,
    );
    const healthUsecase = new HealthCheckUsecase(secretManager);
    const monitorUsecase = new MonitorTokenUsecase(secretManager);
    const authorizeUsecase = new AuthorizeUsecase(
      tokenClient,
      secretManager,
      credentials,
    );

    const getTokenUsecase = new GetTokenUsecase(secretManager);

    // Handler layer
    const oauthServer = new OAuthServer(
      authorizeUsecase,
      getTokenUsecase,
      credentials,
    );
    const daemon = new DaemonLoop(refreshUsecase, getTokenUsecase, oauthServer);
    const cli = new CliHandler(
      refreshUsecase,
      healthUsecase,
      monitorUsecase,
      authorizeUsecase,
      oauthServer,
      daemon,
    );

    await cli.run(Deno.args);
  } catch (error) {
    logger.error("Critical error during startup", {
      error: error instanceof Error ? error.message : String(error),
    });
    Deno.exit(1);
  }
}

// Error boundary – fires after main() so OTel may or may not be live.
globalThis.addEventListener("error", (event) => {
  emergencyLog("Unhandled error", {
    message: event.message,
    filename: event.filename,
    lineno: event.lineno,
  });
  shutdownOTel().finally(() => Deno.exit(1));
});

globalThis.addEventListener("unhandledrejection", (event) => {
  emergencyLog("Unhandled promise rejection", {
    reason: event.reason instanceof Error
      ? event.reason.message
      : String(event.reason),
  });
  shutdownOTel().finally(() => Deno.exit(1));
});

if (import.meta.main) {
  await main();
}
