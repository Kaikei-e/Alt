/**
 * Main entry point for auth-token-manager
 * DI container and CLI routing only
 */

import { config } from "./src/infra/config.ts";
import { shutdownOTel, StructuredLogger } from "./src/infra/logger.ts";
import { EnvFileSecretManager } from "./src/gateway/env_file_secret_manager.ts";
import { FetchHttpClient } from "./src/gateway/fetch_http_client.ts";
import { InoreaderTokenClient } from "./src/gateway/inoreader_token_client.ts";
import { RefreshTokenUsecase } from "./src/usecase/refresh_token.ts";
import { HealthCheckUsecase } from "./src/usecase/health_check.ts";
import { MonitorTokenUsecase } from "./src/usecase/monitor_token.ts";
import { AuthorizeUsecase } from "./src/usecase/authorize.ts";
import { OAuthServer } from "./src/handler/oauth_server.ts";
import { DaemonLoop } from "./src/handler/daemon.ts";
import { CliHandler } from "./src/handler/cli.ts";

const logger = new StructuredLogger("auth-token-manager");

async function main() {
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

    // Handler layer
    const oauthServer = new OAuthServer(
      authorizeUsecase,
      secretManager,
      credentials,
    );
    const daemon = new DaemonLoop(refreshUsecase, secretManager, oauthServer);
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

// Error boundary
globalThis.addEventListener("error", () => {
  logger.error("Unhandled error");
  shutdownOTel();
  Deno.exit(1);
});

globalThis.addEventListener("unhandledrejection", () => {
  logger.error("Unhandled promise rejection");
  shutdownOTel();
  Deno.exit(1);
});

if (import.meta.main) {
  await main();
}
