import type {
  FullConfig,
  FullResult,
  Reporter,
  Suite,
  TestCase,
  TestResult,
} from "@playwright/test/reporter";
import { existsSync, mkdirSync, writeFileSync } from "fs";
import { join } from "path";

interface TestMetrics {
  totalTests: number;
  passedTests: number;
  failedTests: number;
  skippedTests: number;
  totalDuration: number;
  averageDuration: number;
  slowestTest: { name: string; duration: number } | null;
  fastestTest: { name: string; duration: number } | null;
  projectMetrics: Record<
    string,
    {
      passed: number;
      failed: number;
      skipped: number;
      duration: number;
    }
  >;
  browserMetrics: Record<
    string,
    {
      passed: number;
      failed: number;
      duration: number;
    }
  >;
}

class CustomReporter implements Reporter {
  private startTime: number = 0;
  private endTime: number = 0;
  private metrics: TestMetrics = {
    totalTests: 0,
    passedTests: 0,
    failedTests: 0,
    skippedTests: 0,
    totalDuration: 0,
    averageDuration: 0,
    slowestTest: null,
    fastestTest: null,
    projectMetrics: {},
    browserMetrics: {},
  };
  private testResults: Array<{
    name: string;
    status: string;
    duration: number;
    project: string;
    browser: string;
    error?: string;
  }> = [];

  onBegin(config: FullConfig, suite: Suite) {
    this.startTime = Date.now();
    console.log(`\n🚀 Starting Playwright tests with ${config.workers} workers`);
    console.log(
      `📋 Running ${suite.allTests().length} tests across ${config.projects.length} projects`
    );

    // Initialize project metrics
    config.projects.forEach((project) => {
      this.metrics.projectMetrics[project.name] = {
        passed: 0,
        failed: 0,
        skipped: 0,
        duration: 0,
      };
    });
  }

  onTestEnd(test: TestCase, result: TestResult) {
    this.metrics.totalTests++;

    const projectName = test.parent.project()?.name || "unknown";
    const browserName = this.extractBrowserName(projectName);
    const testName = `${test.parent.title} > ${test.title}`;
    const duration = result.duration;

    // Update basic metrics
    switch (result.status) {
      case "passed":
        this.metrics.passedTests++;
        this.metrics.projectMetrics[projectName].passed++;
        break;
      case "failed":
        this.metrics.failedTests++;
        this.metrics.projectMetrics[projectName].failed++;
        break;
      case "skipped":
        this.metrics.skippedTests++;
        this.metrics.projectMetrics[projectName].skipped++;
        break;
    }

    // Update duration metrics
    this.metrics.totalDuration += duration;
    this.metrics.projectMetrics[projectName].duration += duration;

    // Track slowest/fastest tests
    if (!this.metrics.slowestTest || duration > this.metrics.slowestTest.duration) {
      this.metrics.slowestTest = { name: testName, duration };
    }
    if (!this.metrics.fastestTest || duration < this.metrics.fastestTest.duration) {
      this.metrics.fastestTest = { name: testName, duration };
    }

    // Update browser metrics
    if (!this.metrics.browserMetrics[browserName]) {
      this.metrics.browserMetrics[browserName] = {
        passed: 0,
        failed: 0,
        duration: 0,
      };
    }
    this.metrics.browserMetrics[browserName].duration += duration;
    if (result.status === "passed") {
      this.metrics.browserMetrics[browserName].passed++;
    } else if (result.status === "failed") {
      this.metrics.browserMetrics[browserName].failed++;
    }

    // Store individual test result
    this.testResults.push({
      name: testName,
      status: result.status,
      duration,
      project: projectName,
      browser: browserName,
      error: result.error?.message,
    });

    // Real-time feedback
    const icon = result.status === "passed" ? "✅" : result.status === "failed" ? "❌" : "⏭️";
    const durationMs = `${duration.toFixed(0)}ms`;
    console.log(`${icon} ${testName} (${durationMs})`);

    if (result.status === "failed" && result.error) {
      console.log(`   💥 ${result.error.message?.split("\n")[0] || "Unknown error"}`);
    }
  }

  onEnd(result: FullResult) {
    this.endTime = Date.now();
    const totalRunTime = this.endTime - this.startTime;

    // Calculate average duration
    this.metrics.averageDuration =
      this.metrics.totalTests > 0 ? this.metrics.totalDuration / this.metrics.totalTests : 0;

    this.printSummary(totalRunTime, result.status);
    this.generateReports();
  }

  private extractBrowserName(projectName: string): string {
    if (projectName.includes("chrome")) return "Chrome";
    if (projectName.includes("firefox")) return "Firefox";
    if (projectName.includes("safari")) return "Safari";
    if (projectName.includes("webkit")) return "WebKit";
    return "Unknown";
  }

  private printSummary(totalRunTime: number, status: string) {
    const { totalTests, passedTests, failedTests, skippedTests } = this.metrics;

    console.log("\n" + "=".repeat(60));
    console.log("🎯 TEST EXECUTION SUMMARY");
    console.log("=".repeat(60));

    // Overall results
    console.log(`📊 Overall: ${passedTests}/${totalTests} passed`);
    console.log(`⏱️  Total time: ${(totalRunTime / 1000).toFixed(2)}s`);
    console.log(`📈 Average test time: ${this.metrics.averageDuration.toFixed(0)}ms`);

    if (failedTests > 0) {
      console.log(`❌ Failed: ${failedTests}`);
    }
    if (skippedTests > 0) {
      console.log(`⏭️  Skipped: ${skippedTests}`);
    }

    // Performance insights
    if (this.metrics.slowestTest) {
      console.log(
        `🐌 Slowest: ${this.metrics.slowestTest.name} (${this.metrics.slowestTest.duration.toFixed(0)}ms)`
      );
    }
    if (this.metrics.fastestTest) {
      console.log(
        `⚡ Fastest: ${this.metrics.fastestTest.name} (${this.metrics.fastestTest.duration.toFixed(0)}ms)`
      );
    }

    // Project breakdown
    console.log("\n📋 Project Breakdown:");
    Object.entries(this.metrics.projectMetrics).forEach(([project, metrics]) => {
      const total = metrics.passed + metrics.failed + metrics.skipped;
      if (total > 0) {
        const passRate = ((metrics.passed / total) * 100).toFixed(1);
        console.log(
          `  ${project}: ${metrics.passed}/${total} (${passRate}%) - ${(metrics.duration / 1000).toFixed(1)}s`
        );
      }
    });

    // Browser breakdown
    console.log("\n🌐 Browser Breakdown:");
    Object.entries(this.metrics.browserMetrics).forEach(([browser, metrics]) => {
      const total = metrics.passed + metrics.failed;
      if (total > 0) {
        const passRate = ((metrics.passed / total) * 100).toFixed(1);
        console.log(
          `  ${browser}: ${metrics.passed}/${total} (${passRate}%) - ${(metrics.duration / 1000).toFixed(1)}s`
        );
      }
    });

    console.log("=".repeat(60));

    const finalStatus =
      status === "passed"
        ? "✅ ALL TESTS PASSED!"
        : status === "failed"
          ? "❌ SOME TESTS FAILED!"
          : "⚠️  TESTS INTERRUPTED!";

    console.log(finalStatus);
    console.log("=".repeat(60));
  }

  private generateReports() {
    const reportDir = "test-results/reports";

    if (!existsSync(reportDir)) {
      mkdirSync(reportDir, { recursive: true });
    }

    // Generate JSON report
    const report = {
      timestamp: new Date().toISOString(),
      metrics: this.metrics,
      results: this.testResults,
      summary: {
        executionTime: this.endTime - this.startTime,
        passRate: (this.metrics.passedTests / this.metrics.totalTests) * 100,
        averageTestTime: this.metrics.averageDuration,
      },
    };

    writeFileSync(join(reportDir, "test-metrics.json"), JSON.stringify(report, null, 2));

    // Generate simple HTML dashboard
    this.generateHtmlReport(reportDir, report);

    console.log(`📈 Reports generated in: ${reportDir}`);
    console.log(`📊 View metrics: file://${process.cwd()}/${reportDir}/dashboard.html`);
  }

  private generateHtmlReport(reportDir: string, report: any) {
    const html = `
<!DOCTYPE html>
<html>
<head>
    <title>Playwright Test Metrics Dashboard</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 40px; }
        .metric { display: inline-block; margin: 10px; padding: 20px; border: 1px solid #ddd; border-radius: 8px; min-width: 200px; }
        .passed { border-color: #28a745; background: #f8fff9; }
        .failed { border-color: #dc3545; background: #fff5f5; }
        .chart-container { margin: 20px 0; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
        .status-passed { color: #28a745; font-weight: bold; }
        .status-failed { color: #dc3545; font-weight: bold; }
        .status-skipped { color: #ffc107; font-weight: bold; }
    </style>
</head>
<body>
    <h1>🎯 Playwright Test Dashboard</h1>
    <p><strong>Generated:</strong> ${report.timestamp}</p>
    
    <div class="metrics">
        <div class="metric ${report.metrics.failedTests === 0 ? "passed" : "failed"}">
            <h3>Overall Results</h3>
            <p><strong>${report.metrics.passedTests}</strong> / ${report.metrics.totalTests} passed</p>
            <p>Pass Rate: <strong>${report.summary.passRate.toFixed(1)}%</strong></p>
        </div>
        
        <div class="metric">
            <h3>⏱️ Performance</h3>
            <p>Total Time: <strong>${(report.summary.executionTime / 1000).toFixed(2)}s</strong></p>
            <p>Avg Test Time: <strong>${report.summary.averageTestTime.toFixed(0)}ms</strong></p>
        </div>
        
        <div class="metric">
            <h3>🐌 Slowest Test</h3>
            <p><strong>${report.metrics.slowestTest?.name || "N/A"}</strong></p>
            <p>${report.metrics.slowestTest?.duration.toFixed(0) || 0}ms</p>
        </div>
    </div>

    <h2>📊 Project Results</h2>
    <table>
        <tr><th>Project</th><th>Passed</th><th>Failed</th><th>Skipped</th><th>Duration</th></tr>
        ${Object.entries(report.metrics.projectMetrics)
          .map(
            ([project, metrics]: [string, any]) => `
            <tr>
                <td>${project}</td>
                <td class="status-passed">${metrics.passed}</td>
                <td class="status-failed">${metrics.failed}</td>
                <td class="status-skipped">${metrics.skipped}</td>
                <td>${(metrics.duration / 1000).toFixed(2)}s</td>
            </tr>
        `
          )
          .join("")}
    </table>

    <h2>🧪 Individual Test Results</h2>
    <table>
        <tr><th>Test</th><th>Status</th><th>Duration</th><th>Project</th><th>Browser</th><th>Error</th></tr>
        ${report.results
          .map(
            (test: any) => `
            <tr>
                <td>${test.name}</td>
                <td class="status-${test.status}">${test.status.toUpperCase()}</td>
                <td>${test.duration.toFixed(0)}ms</td>
                <td>${test.project}</td>
                <td>${test.browser}</td>
                <td>${test.error ? test.error.substring(0, 100) + "..." : ""}</td>
            </tr>
        `
          )
          .join("")}
    </table>
</body>
</html>`;

    writeFileSync(join(reportDir, "dashboard.html"), html);
  }
}

export default CustomReporter;
