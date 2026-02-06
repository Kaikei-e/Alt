/**
 * CLI command handler - routes commands to appropriate usecases
 */

import type { RefreshTokenUsecase } from "../usecase/refresh_token.ts";
import type { HealthCheckUsecase } from "../usecase/health_check.ts";
import type { MonitorTokenUsecase } from "../usecase/monitor_token.ts";
import type { AuthorizeUsecase } from "../usecase/authorize.ts";
import { OAuthServer } from "./oauth_server.ts";
import { DaemonLoop } from "./daemon.ts";
import { logger } from "../infra/logger.ts";
import { config } from "../infra/config.ts";

export class CliHandler {
  constructor(
    private refreshUsecase: RefreshTokenUsecase,
    private healthUsecase: HealthCheckUsecase,
    private monitorUsecase: MonitorTokenUsecase,
    private authorizeUsecase: AuthorizeUsecase,
    private oauthServer: OAuthServer,
    private daemon: DaemonLoop,
  ) {}

  async run(args: string[]): Promise<void> {
    const command = args[0] || "daemon";

    switch (command) {
      case "authorize":
        await this.handleAuthorize();
        break;
      case "refresh":
        await this.handleRefresh();
        break;
      case "health":
        await this.handleHealth();
        break;
      case "validate":
        await this.handleValidate();
        break;
      case "monitor":
        await this.handleMonitor();
        break;
      case "daemon":
        await this.handleDaemon();
        break;
      case "help":
        this.showHelp();
        break;
      default:
        logger.error("Unknown command", { command });
        this.showHelp();
        Deno.exit(1);
    }
  }

  private async handleAuthorize(): Promise<void> {
    logger.info("Starting initial OAuth2 authorization flow");
    const { url, state } = this.authorizeUsecase.buildAuthorizationUrl();
    const credentials = config.getInoreaderCredentials();
    const redirectUrl = new URL(credentials.redirect_uri);

    logger.info("Open the following URL in your browser:");
    console.log(url);
    logger.info(
      `Listening for OAuth callback at ${credentials.redirect_uri}`,
    );

    const ac = new AbortController();
    const server = Deno.serve(
      {
        port: Number(redirectUrl.port || 80),
        hostname: "0.0.0.0",
        signal: ac.signal,
        onListen() {},
      },
      (req) => {
        const reqUrl = new URL(req.url);

        if (reqUrl.searchParams.has("error")) {
          const error = reqUrl.searchParams.get("error");
          const description = reqUrl.searchParams.get("error_description");
          logger.error("Authorization failed", { error, description });
          setTimeout(() => {
            ac.abort();
            Deno.exit(1);
          }, 100);
          return new Response(
            `Authorization failed: ${error} - ${description}`,
            { status: 400 },
          );
        }

        if (
          reqUrl.pathname === redirectUrl.pathname &&
          reqUrl.searchParams.has("code")
        ) {
          const returnedState = reqUrl.searchParams.get("state");
          if (returnedState !== state) {
            logger.error(
              "State parameter mismatch - possible CSRF attack",
            );
            setTimeout(() => {
              ac.abort();
              Deno.exit(1);
            }, 100);
            return new Response(
              "Security Error: State parameter mismatch",
              { status: 400 },
            );
          }

          const code = reqUrl.searchParams.get("code")!;

          (async () => {
            try {
              await this.authorizeUsecase.exchangeCodeAndStore(code);
              logger.info("OAuth2 flow completed and secret updated");
              setTimeout(() => {
                ac.abort();
                Deno.exit(0);
              }, 500);
            } catch (err) {
              logger.error("Error during token exchange", {
                error: err instanceof Error ? err.message : String(err),
              });
              Deno.exit(1);
            }
          })();

          return new Response(
            "Authorization successful! You may close this tab.",
            { status: 200 },
          );
        }

        return new Response("Invalid OAuth callback request", {
          status: 400,
        });
      },
    );
    await server.finished;
  }

  private async handleRefresh(): Promise<void> {
    const result = await this.refreshUsecase.execute();
    if (!result.success) {
      throw new Error(`Token refresh failed: ${result.error}`);
    }
    logger.info("Token refresh completed successfully");
  }

  private async handleHealth(): Promise<void> {
    const result = await this.healthUsecase.execute();
    if (result.status === "unhealthy") {
      logger.error("Health check failed - service is unhealthy");
      Deno.exit(1);
    }
  }

  private async handleValidate(): Promise<void> {
    logger.info("Running configuration validation");
    await config.loadConfig();
    logger.info("Configuration validation completed successfully");
  }

  private async handleMonitor(): Promise<void> {
    const result = await this.monitorUsecase.execute();
    if (result.alertLevel === "critical") {
      Deno.exit(2);
    } else if (result.alertLevel === "warning") {
      Deno.exit(1);
    }
  }

  private async handleDaemon(): Promise<void> {
    await this.daemon.start();
  }

  private showHelp(): void {
    console.log(`
Auth Token Manager v2.1.0
Automated OAuth token refresh system - Deno 2.x

USAGE:
  deno run --allow-all main.ts [COMMAND]

COMMANDS:
  authorize  Perform initial OAuth2 authorization flow
  refresh    Refresh OAuth tokens
  health     Run health check
  validate   Validate configuration
  monitor    Run token monitoring with alerting
  daemon     Run continuous daemon mode (default)
  help       Show this help message
`);
  }
}
