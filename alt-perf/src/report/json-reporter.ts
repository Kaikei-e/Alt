/**
 * JSON report generator
 */
import type { PerformanceReport } from "./types.ts";

/**
 * Generate JSON report string
 */
export function generateJsonReport(report: PerformanceReport): string {
  return JSON.stringify(report, null, 2);
}

/**
 * Save report to file
 */
export async function saveJsonReport(
  report: PerformanceReport,
  filePath: string
): Promise<void> {
  const jsonContent = generateJsonReport(report);
  await Deno.writeTextFile(filePath, jsonContent);
}

/**
 * Print report to stdout
 */
export function printJsonReport(report: PerformanceReport): void {
  console.log(generateJsonReport(report));
}
