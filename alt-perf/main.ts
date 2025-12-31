/**
 * alt-perf - Alt E2E Performance Measurement Tool
 *
 * CLI entry point for measuring performance through Nginx
 */
import { parseArgs } from "@std/cli/parse-args";
import { bold, cyan, dim, green, red, yellow } from "./src/utils/colors.ts";
import { configureLogger, error, info, section } from "./src/utils/logger.ts";
import { loadConfig } from "./src/config/loader.ts";
import { runScan } from "./src/commands/scan.ts";
import { runFlow } from "./src/commands/flow.ts";
import { runLoad } from "./src/commands/load.ts";

const VERSION = "1.0.0";

interface CliOptions {
  help: boolean;
  version: boolean;
  config: string;
  output: string;
  device: string;
  route: string;
  json: boolean;
  verbose: boolean;
  headless: boolean;
  // Load test specific
  duration: number;
  concurrency: number;
}

// Parse command line arguments
function parseCliArgs(): { command: string; options: CliOptions } {
  const args = parseArgs(Deno.args, {
    string: ["config", "output", "device", "route"],
    boolean: ["help", "version", "json", "verbose", "headless"],
    default: {
      config: "./config",
      headless: true,
      verbose: false,
      duration: 30,
      concurrency: 10,
    },
    alias: {
      h: "help",
      v: "version",
      c: "config",
      o: "output",
      d: "device",
      r: "route",
      V: "verbose",
    },
  });

  const command = args._[0]?.toString() || "help";
  const options: CliOptions = {
    help: args.help as boolean,
    version: args.version as boolean,
    config: args.config as string,
    output: args.output as string,
    device: args.device as string,
    route: args.route as string,
    json: args.json as boolean,
    verbose: args.verbose as boolean,
    headless: args.headless as boolean,
    duration: Number(args.duration) || 30,
    concurrency: Number(args.concurrency) || 10,
  };

  return { command, options };
}

// Show help message
function showHelp(): void {
  console.log(`
${bold("alt-perf")} - Alt E2E Performance Measurement Tool

${bold("USAGE:")}
  alt-perf <command> [options]

${bold("COMMANDS:")}
  ${cyan("scan")}     Scan all configured routes and measure Web Vitals
  ${cyan("flow")}     Execute user flow tests
  ${cyan("load")}     Run load tests against endpoints
  ${cyan("help")}     Show this help message

${bold("OPTIONS:")}
  ${green("-c, --config <path>")}    Path to config directory (default: ./config)
  ${green("-o, --output <path>")}    Output file for JSON results
  ${green("-d, --device <name>")}    Device profile (desktop-chrome, mobile-chrome, mobile-safari)
  ${green("-r, --route <path>")}     Specific route to test
  ${green("-V, --verbose")}          Enable verbose logging
  ${green("--headless")}             Run browser in headless mode (default: true)
  ${green("--json")}                 Output results as JSON only
  ${green("-h, --help")}             Show this help message
  ${green("-v, --version")}          Show version number

${bold("LOAD TEST OPTIONS:")}
  ${green("--duration <seconds>")}   Load test duration (default: 30)
  ${green("--concurrency <n>")}      Number of concurrent requests (default: 10)

${bold("EXAMPLES:")}
  ${dim("# Scan all routes")}
  alt-perf scan

  ${dim("# Scan with mobile device")}
  alt-perf scan -d mobile-chrome

  ${dim("# Scan specific route")}
  alt-perf scan -r /mobile/feeds

  ${dim("# Run user flow tests")}
  alt-perf flow

  ${dim("# 60-second load test with 20 concurrent requests")}
  alt-perf load --duration 60 --concurrency 20

  ${dim("# Output JSON report")}
  alt-perf scan -o reports/scan.json

${bold("ENVIRONMENT VARIABLES:")}
  ${yellow("PERF_TEST_EMAIL")}       Email for authenticated tests
  ${yellow("PERF_TEST_PASSWORD")}    Password for authenticated tests
  ${yellow("PERF_BASE_URL")}         Base URL (default: http://localhost)

${bold("CORE WEB VITALS THRESHOLDS:")}
  LCP  < 2.5s   ${green("Good")}  |  2.5s - 4.0s  ${yellow("Needs Improvement")}  |  > 4.0s  ${red("Poor")}
  INP  < 200ms  ${green("Good")}  |  200ms - 500ms  ${yellow("Needs Improvement")}  |  > 500ms  ${red("Poor")}
  CLS  < 0.1    ${green("Good")}  |  0.1 - 0.25  ${yellow("Needs Improvement")}  |  > 0.25  ${red("Poor")}
`);
}

// Show version
function showVersion(): void {
  console.log(`alt-perf v${VERSION}`);
}

// Main entry point
async function main(): Promise<void> {
  const { command, options } = parseCliArgs();

  // Handle help and version flags
  if (options.help || command === "help") {
    showHelp();
    Deno.exit(0);
  }

  if (options.version) {
    showVersion();
    Deno.exit(0);
  }

  // Configure logger
  configureLogger({
    level: options.verbose ? "debug" : "info",
    json: options.json,
    verbose: options.verbose,
  });

  // Load configuration
  let config;
  try {
    config = await loadConfig(options.config);
    if (options.verbose) {
      info("Configuration loaded", { baseUrl: config.baseUrl });
    }
  } catch (err) {
    error("Failed to load configuration", { error: String(err) });
    Deno.exit(1);
  }

  // Execute command
  try {
    switch (command) {
      case "scan":
        await runScan(config, options);
        break;

      case "flow":
        await runFlow(config, options);
        break;

      case "load":
        await runLoad(config, {
          ...options,
          duration: options.duration,
          concurrency: options.concurrency,
        });
        break;

      default:
        error(`Unknown command: ${command}`);
        showHelp();
        Deno.exit(1);
    }
  } catch (err) {
    error("Command failed", { error: String(err) });
    if (options.verbose && err instanceof Error) {
      console.error(err.stack);
    }
    Deno.exit(1);
  }
}

// Run main
main();
