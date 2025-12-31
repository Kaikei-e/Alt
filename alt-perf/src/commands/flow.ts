/**
 * Flow command - execute user flow tests
 */
import type { PerfConfig, FlowConfig, FlowStep } from "../config/schema.ts";
import { createBrowserManager, type Page } from "../browser/astral.ts";
import { getAuthCookies } from "../auth/kratos-session.ts";
import { createWebVitalsCollector } from "../measurement/vitals.ts";
import type { PerformanceReport, FlowResult, FlowStepResult } from "../report/types.ts";
import { printCliReport } from "../report/cli-reporter.ts";
import { saveJsonReport, printJsonReport } from "../report/json-reporter.ts";
import { info, error, debug, section } from "../utils/logger.ts";

interface FlowOptions {
  output?: string;
  json?: boolean;
  headless?: boolean;
  verbose?: boolean;
}

/**
 * Execute a single flow step
 */
async function executeStep(
  page: Page,
  step: FlowStep,
  baseUrl: string
): Promise<FlowStepResult> {
  const startTime = performance.now();
  const result: FlowStepResult = {
    action: step.action,
    target: step.selector || step.url,
    duration: 0,
    success: false,
  };

  try {
    switch (step.action) {
      case "navigate": {
        const url = step.url?.startsWith("http") ? step.url : `${baseUrl}${step.url}`;
        await page.goto(url, { waitUntil: "networkidle2" });
        if (step.waitFor) {
          await page.waitForSelector(step.waitFor, { timeout: 10000 });
        }
        break;
      }

      case "click": {
        if (!step.selector) throw new Error("Click requires selector");
        await page.waitForSelector(step.selector, { timeout: 10000 });
        const element = await page.$(step.selector);
        if (element) {
          await element.click();
        }
        if (step.waitFor === "navigation") {
          await page.waitForNavigation({ waitUntil: "networkidle2" });
        } else if (step.waitFor) {
          await page.waitForSelector(step.waitFor, { timeout: 10000 });
        }
        break;
      }

      case "fill":
      case "type": {
        if (!step.selector) throw new Error(`${step.action} requires selector`);
        if (step.value === undefined) throw new Error(`${step.action} requires value`);
        await page.waitForSelector(step.selector, { timeout: 10000 });
        const element = await page.$(step.selector);
        if (element) {
          // Clear and type
          await element.evaluate((el: HTMLInputElement) => { el.value = ""; });
          await element.type(step.value);
        }
        break;
      }

      case "scroll": {
        const amount = step.amount || 500;
        const direction = step.direction || "down";
        const deltaY = direction === "down" ? amount : -amount;
        await page.evaluate(`globalThis.scrollBy(0, ${deltaY})`);
        break;
      }

      case "swipe": {
        // Simulate swipe with scroll
        const swipeAmount = step.amount || 300;
        const repeat = step.repeat || 1;
        const delay = step.delay || 300;

        for (let i = 0; i < repeat; i++) {
          if (step.direction === "left" || step.direction === "right") {
            const deltaX = step.direction === "left" ? -swipeAmount : swipeAmount;
            await page.evaluate(`globalThis.scrollBy(${deltaX}, 0)`);
          } else {
            const deltaY = step.direction === "up" ? -swipeAmount : swipeAmount;
            await page.evaluate(`globalThis.scrollBy(0, ${deltaY})`);
          }
          await new Promise((resolve) => setTimeout(resolve, delay));
        }
        break;
      }

      case "wait": {
        const duration = step.duration || 1000;
        await new Promise((resolve) => setTimeout(resolve, duration));
        break;
      }

      default:
        throw new Error(`Unknown action: ${step.action}`);
    }

    result.success = true;
  } catch (err) {
    result.error = String(err);
    result.success = false;
  }

  result.duration = performance.now() - startTime;
  return result;
}

/**
 * Execute a complete flow
 */
async function executeFlow(
  flow: FlowConfig,
  config: PerfConfig,
  options: FlowOptions
): Promise<FlowResult> {
  const browser = createBrowserManager({
    headless: options.headless ?? true,
  });

  const device = flow.device || "desktop-chrome";
  const vitalsCollector = createWebVitalsCollector();

  const result: FlowResult = {
    name: flow.name,
    description: flow.description,
    device,
    steps: [],
    totalDuration: 0,
    passed: true,
  };

  const startTime = performance.now();

  try {
    await browser.launch();

    // Get auth cookies if needed
    let cookies: Awaited<ReturnType<typeof getAuthCookies>> = [];
    if (flow.requiresAuth && config.auth.enabled) {
      cookies = await getAuthCookies(config.auth);
    }

    const page = await browser.createPage(device, cookies);

    for (const step of flow.steps) {
      debug("Executing step", { action: step.action });
      const stepResult = await executeStep(page, step, config.baseUrl);

      // Collect vitals if step is marked for measurement
      if (step.measure && stepResult.success) {
        await vitalsCollector.inject(page);
        stepResult.vitals = await vitalsCollector.collect(page);
      }

      result.steps.push(stepResult);

      if (!stepResult.success) {
        result.passed = false;
        result.error = stepResult.error;
        break;
      }
    }

    await page.close();
  } catch (err) {
    result.passed = false;
    result.error = String(err);
  } finally {
    await browser.close();
  }

  result.totalDuration = performance.now() - startTime;
  return result;
}

/**
 * Run flow command
 */
export async function runFlow(config: PerfConfig, options: FlowOptions): Promise<void> {
  const startTime = performance.now();
  section("User Flow Tests");

  const flows = config.flows?.flows || [];

  if (flows.length === 0) {
    error("No flows configured. Create config/flows.yaml with flow definitions.");
    return;
  }

  info(`Executing ${flows.length} flow(s)`);

  const results: FlowResult[] = [];

  for (const flow of flows) {
    info(`Running flow: ${flow.name}`);
    const result = await executeFlow(flow, config, options);
    results.push(result);

    if (result.passed) {
      info(`Flow passed: ${flow.name}`);
    } else {
      error(`Flow failed: ${flow.name}`, { error: result.error });
    }
  }

  // Calculate summary
  const passedFlows = results.filter((r) => r.passed).length;

  // Generate report
  const report: PerformanceReport = {
    metadata: {
      timestamp: new Date().toISOString(),
      duration: performance.now() - startTime,
      toolVersion: "1.0.0",
      baseUrl: config.baseUrl,
      devices: [...new Set(results.map((r) => r.device))],
    },
    summary: {
      totalRoutes: flows.length,
      passedRoutes: passedFlows,
      failedRoutes: flows.length - passedFlows,
      overallScore: Math.round((passedFlows / flows.length) * 100),
      overallRating: passedFlows === flows.length ? "good" : "poor",
    },
    routes: [],
    flows: results,
    recommendations: [],
  };

  // Output report
  if (options.json) {
    printJsonReport(report);
  } else {
    printCliReport(report);
  }

  // Save to file if specified
  if (options.output) {
    await saveJsonReport(report, options.output);
    info(`Report saved to ${options.output}`);
  }
}
