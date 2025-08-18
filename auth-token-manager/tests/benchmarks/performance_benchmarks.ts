/**
 * Performance Benchmarks for Enhanced Logger System - 2025 Edition
 * Comprehensive performance testing for sanitization, logging, and security features
 */

import { DataSanitizer, StructuredLogger } from "../../src/utils/logger.ts";
import { securityMonitor } from "../../src/monitoring/security_monitor.ts";

/**
 * Benchmark configuration
 */
interface BenchmarkConfig {
  iterations: number;
  warmupIterations: number;
  dataSize: 'small' | 'medium' | 'large';
  includeIntegrity: boolean;
}

/**
 * Benchmark results
 */
interface BenchmarkResult {
  testName: string;
  iterations: number;
  totalTimeMs: number;
  averageTimeMs: number;
  operationsPerSecond: number;
  memoryUsageMB?: number;
  cacheHitRate?: number;
}

/**
 * Performance benchmark suite
 */
export class PerformanceBenchmarks {
  private logger = new StructuredLogger('benchmark-suite');
  
  /**
   * Generate test data of specified size
   */
  private generateTestData(size: 'small' | 'medium' | 'large'): any[] {
    const baseData = {
      user_id: 'user123',
      session_id: 'sess_456789',
      access_token: 'ya29.1234567890abcdefghijklmnopqrstuvwxyz',
      refresh_token: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.signature',
      api_key: 'AIza1234567890abcdefghijklmnopqrstuvwxyz',
      github_token: 'ghp_1234567890abcdefghijklmnopqrstuvwxyz',
      email: 'user@example.com',
      phone: '+1-555-123-4567',
      credit_card: '4532-1234-5678-9012',
      ssn: '123-45-6789',
      message: 'User performed authentication with token validation',
      timestamp: new Date().toISOString(),
      ip_address: '192.168.1.100',
      user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
    };

    const sizeMappings = {
      small: 100,
      medium: 1000,
      large: 10000
    };

    return Array.from({ length: sizeMappings[size] }, (_, i) => ({
      ...baseData,
      id: i,
      unique_field: `unique_${i}_${Math.random().toString(36).substring(7)}`
    }));
  }

  /**
   * Run performance benchmark
   */
  private async runBenchmark(
    testName: string,
    testFunction: () => Promise<void> | void,
    config: BenchmarkConfig
  ): Promise<BenchmarkResult> {
    console.log(`Starting benchmark: ${testName}`);
    
    // Warmup
    for (let i = 0; i < config.warmupIterations; i++) {
      await testFunction();
    }
    
    const startTime = performance.now();
    
    // Run actual benchmark
    for (let i = 0; i < config.iterations; i++) {
      await testFunction();
    }
    
    const endTime = performance.now();
    
    const totalTimeMs = endTime - startTime;
    const averageTimeMs = totalTimeMs / config.iterations;
    const operationsPerSecond = 1000 / averageTimeMs;
    
    const result: BenchmarkResult = {
      testName,
      iterations: config.iterations,
      totalTimeMs: Math.round(totalTimeMs * 100) / 100,
      averageTimeMs: Math.round(averageTimeMs * 1000) / 1000,
      operationsPerSecond: Math.round(operationsPerSecond * 100) / 100
    };
    
    console.log(`Completed benchmark: ${testName}`);
    console.log(`- Average time: ${result.averageTimeMs}ms`);
    console.log(`- Operations/sec: ${result.operationsPerSecond}`);
    
    return result;
  }

  /**
   * Benchmark: Synchronous data sanitization
   */
  async benchmarkSyncSanitization(): Promise<BenchmarkResult> {
    const testData = this.generateTestData('medium');
    
    return this.runBenchmark(
      'Synchronous Data Sanitization',
      () => {
        testData.forEach(data => DataSanitizer.sanitize(data));
      },
      {
        iterations: 100,
        warmupIterations: 10,
        dataSize: 'medium',
        includeIntegrity: false
      }
    );
  }

  /**
   * Benchmark: Asynchronous data sanitization
   */
  async benchmarkAsyncSanitization(): Promise<BenchmarkResult> {
    const testData = this.generateTestData('medium');
    
    return this.runBenchmark(
      'Asynchronous Data Sanitization',
      async () => {
        await DataSanitizer.sanitizeAsync(testData);
      },
      {
        iterations: 100,
        warmupIterations: 10,
        dataSize: 'medium',
        includeIntegrity: false
      }
    );
  }

  /**
   * Benchmark: Basic structured logging
   */
  async benchmarkBasicLogging(): Promise<BenchmarkResult> {
    const logger = new StructuredLogger('benchmark');
    const testData = { message: 'Test log message', data: { id: 123, value: 'test' } };
    
    return this.runBenchmark(
      'Basic Structured Logging',
      () => {
        logger.info(testData.message, testData.data);
      },
      {
        iterations: 1000,
        warmupIterations: 100,
        dataSize: 'small',
        includeIntegrity: false
      }
    );
  }

  /**
   * Benchmark: Security event monitoring
   */
  async benchmarkSecurityMonitoring(): Promise<BenchmarkResult> {
    return this.runBenchmark(
      'Security Event Monitoring',
      () => {
        securityMonitor.recordSecurityEvent(
          'suspicious_pattern',
          'medium',
          {
            source_ip: '192.168.1.100',
            user_id: 'user123',
            action: 'token_validation',
            success: true
          }
        );
      },
      {
        iterations: 500,
        warmupIterations: 50,
        dataSize: 'small',
        includeIntegrity: false
      }
    );
  }

  /**
   * Run benchmark suite
   */
  async runBenchmarkSuite(): Promise<BenchmarkResult[]> {
    console.log('='.repeat(60));
    console.log('PERFORMANCE BENCHMARK SUITE - 2025 EDITION');
    console.log('='.repeat(60));
    
    const benchmarks = [
      () => this.benchmarkSyncSanitization(),
      () => this.benchmarkAsyncSanitization(),
      () => this.benchmarkBasicLogging(),
      () => this.benchmarkSecurityMonitoring()
    ];
    
    const results: BenchmarkResult[] = [];
    
    for (const benchmark of benchmarks) {
      try {
        const result = await benchmark();
        results.push(result);
        console.log('');
      } catch (error) {
        console.error(`Benchmark failed:`, error);
      }
    }
    
    return results;
  }
}

// Export default instance for easy use
export const performanceBenchmarks = new PerformanceBenchmarks();

// Main benchmark runner (for direct execution)
if (import.meta.main) {
  console.log('Running performance benchmarks...');
  
  try {
    await performanceBenchmarks.runBenchmarkSuite();
  } catch (error) {
    console.error('Benchmark suite failed:', error);
    Deno.exit(1);
  }
}