#!/usr/bin/env node

import { readFileSync, writeFileSync, existsSync } from 'fs';
import { join } from 'path';
import { execSync } from 'child_process';

/**
 * セキュリティレポートを生成するスクリプト
 */
async function generateSecurityReport() {
  const report = {
    timestamp: new Date().toISOString(),
    version: '1.0.0',
    summary: {
      total_tests: 0,
      passed_tests: 0,
      failed_tests: 0,
      vulnerabilities_found: 0
    },
    test_results: {
      xss_prevention: null,
      content_sanitization: null,
      performance_impact: null,
      automated_scan: null
    },
    vulnerabilities: [],
    recommendations: []
  };

  console.log('🔐 Generating security report...');

  try {
    // Vitestテスト結果を取得
    console.log('📋 Running security tests...');
    
    // セキュリティテストの実行
    try {
      execSync('npm run test:security -- --reporter=json --outputFile=security-test-results.json', {
        stdio: 'inherit'
      });
      
      if (existsSync('security-test-results.json')) {
        const vitestResults = JSON.parse(readFileSync('security-test-results.json', 'utf8'));
        report.test_results.content_sanitization = {
          status: vitestResults.success ? 'passed' : 'failed',
          total: vitestResults.numTotalTests,
          passed: vitestResults.numPassedTests,
          failed: vitestResults.numFailedTests
        };
        
        report.summary.total_tests += vitestResults.numTotalTests;
        report.summary.passed_tests += vitestResults.numPassedTests;
        report.summary.failed_tests += vitestResults.numFailedTests;
      }
    } catch (error) {
      console.error('Security tests failed:', error.message);
      report.test_results.content_sanitization = {
        status: 'failed',
        error: error.message
      };
    }

    // パフォーマンステストの実行
    try {
      execSync('npm run test:performance:security -- --reporter=json --outputFile=performance-test-results.json', {
        stdio: 'inherit'
      });
      
      if (existsSync('performance-test-results.json')) {
        const performanceResults = JSON.parse(readFileSync('performance-test-results.json', 'utf8'));
        report.test_results.performance_impact = {
          status: performanceResults.success ? 'passed' : 'failed',
          total: performanceResults.numTotalTests,
          passed: performanceResults.numPassedTests,
          failed: performanceResults.numFailedTests
        };
        
        report.summary.total_tests += performanceResults.numTotalTests;
        report.summary.passed_tests += performanceResults.numPassedTests;
        report.summary.failed_tests += performanceResults.numFailedTests;
      }
    } catch (error) {
      console.error('Performance tests failed:', error.message);
      report.test_results.performance_impact = {
        status: 'failed',
        error: error.message
      };
    }

    // 自動セキュリティスキャンの実行
    try {
      execSync('npm run test:security:scan -- --reporter=json --outputFile=security-scan-results.json', {
        stdio: 'inherit'
      });
      
      if (existsSync('security-scan-results.json')) {
        const scanResults = JSON.parse(readFileSync('security-scan-results.json', 'utf8'));
        report.test_results.automated_scan = {
          status: scanResults.success ? 'passed' : 'failed',
          total: scanResults.numTotalTests,
          passed: scanResults.numPassedTests,
          failed: scanResults.numFailedTests
        };
        
        report.summary.total_tests += scanResults.numTotalTests;
        report.summary.passed_tests += scanResults.numPassedTests;
        report.summary.failed_tests += scanResults.numFailedTests;
      }
    } catch (error) {
      console.error('Automated scan failed:', error.message);
      report.test_results.automated_scan = {
        status: 'failed',
        error: error.message
      };
    }

    // Playwrightテスト結果を取得
    try {
      execSync('npm run test:security:e2e -- --reporter=json --output-file=xss-test-results.json', {
        stdio: 'inherit'
      });
      
      if (existsSync('xss-test-results.json')) {
        const xssResults = JSON.parse(readFileSync('xss-test-results.json', 'utf8'));
        report.test_results.xss_prevention = {
          status: xssResults.stats.failures === 0 ? 'passed' : 'failed',
          total: xssResults.stats.tests,
          passed: xssResults.stats.tests - xssResults.stats.failures,
          failed: xssResults.stats.failures
        };
        
        report.summary.total_tests += xssResults.stats.tests;
        report.summary.passed_tests += xssResults.stats.tests - xssResults.stats.failures;
        report.summary.failed_tests += xssResults.stats.failures;
      }
    } catch (error) {
      console.error('XSS prevention tests failed:', error.message);
      report.test_results.xss_prevention = {
        status: 'failed',
        error: error.message
      };
    }

    // 脆弱性の分析と推奨事項の生成
    analyzeVulnerabilities(report);
    generateRecommendations(report);

    // レポートの生成
    const reportPath = join(process.cwd(), 'security-report.json');
    writeFileSync(reportPath, JSON.stringify(report, null, 2));
    
    console.log('📊 Security report generated:', reportPath);
    console.log('🔍 Summary:');
    console.log(`  Total tests: ${report.summary.total_tests}`);
    console.log(`  Passed: ${report.summary.passed_tests}`);
    console.log(`  Failed: ${report.summary.failed_tests}`);
    console.log(`  Vulnerabilities: ${report.summary.vulnerabilities_found}`);
    
    // レポートのテキスト版も生成
    generateTextReport(report);
    
    // 失敗したテストがある場合は終了コード1で終了
    if (report.summary.failed_tests > 0) {
      process.exit(1);
    }
    
  } catch (error) {
    console.error('❌ Failed to generate security report:', error.message);
    process.exit(1);
  }
}

/**
 * 脆弱性の分析
 */
function analyzeVulnerabilities(report) {
  const vulnerabilities = [];
  
  // 各テスト結果から脆弱性を抽出
  Object.entries(report.test_results).forEach(([testName, result]) => {
    if (result && result.status === 'failed') {
      vulnerabilities.push({
        type: testName,
        severity: getSeverityLevel(testName),
        description: getVulnerabilityDescription(testName),
        impact: getImpactLevel(testName)
      });
    }
  });
  
  report.vulnerabilities = vulnerabilities;
  report.summary.vulnerabilities_found = vulnerabilities.length;
}

/**
 * 推奨事項の生成
 */
function generateRecommendations(report) {
  const recommendations = [];
  
  // 脆弱性に基づく推奨事項
  report.vulnerabilities.forEach(vulnerability => {
    switch (vulnerability.type) {
      case 'xss_prevention':
        recommendations.push({
          priority: 'high',
          action: 'Fix XSS prevention mechanisms',
          description: 'Review and fix content sanitization and CSP headers'
        });
        break;
      case 'content_sanitization':
        recommendations.push({
          priority: 'high',
          action: 'Improve content sanitization',
          description: 'Enhance content filtering and validation'
        });
        break;
      case 'performance_impact':
        recommendations.push({
          priority: 'medium',
          action: 'Optimize security performance',
          description: 'Improve security function performance'
        });
        break;
      case 'automated_scan':
        recommendations.push({
          priority: 'medium',
          action: 'Fix security scan issues',
          description: 'Address issues found in automated security scan'
        });
        break;
    }
  });
  
  // 一般的な推奨事項
  recommendations.push({
    priority: 'low',
    action: 'Regular security monitoring',
    description: 'Continue regular security testing and monitoring'
  });
  
  report.recommendations = recommendations;
}

/**
 * テキストレポートの生成
 */
function generateTextReport(report) {
  const textReport = `
# Security Report

**Generated**: ${report.timestamp}
**Version**: ${report.version}

## Summary

- Total Tests: ${report.summary.total_tests}
- Passed: ${report.summary.passed_tests}
- Failed: ${report.summary.failed_tests}
- Vulnerabilities Found: ${report.summary.vulnerabilities_found}

## Test Results

### XSS Prevention Tests
Status: ${report.test_results.xss_prevention?.status || 'N/A'}
${report.test_results.xss_prevention?.total ? `Total: ${report.test_results.xss_prevention.total}, Passed: ${report.test_results.xss_prevention.passed}, Failed: ${report.test_results.xss_prevention.failed}` : ''}

### Content Sanitization Tests
Status: ${report.test_results.content_sanitization?.status || 'N/A'}
${report.test_results.content_sanitization?.total ? `Total: ${report.test_results.content_sanitization.total}, Passed: ${report.test_results.content_sanitization.passed}, Failed: ${report.test_results.content_sanitization.failed}` : ''}

### Performance Impact Tests
Status: ${report.test_results.performance_impact?.status || 'N/A'}
${report.test_results.performance_impact?.total ? `Total: ${report.test_results.performance_impact.total}, Passed: ${report.test_results.performance_impact.passed}, Failed: ${report.test_results.performance_impact.failed}` : ''}

### Automated Security Scan
Status: ${report.test_results.automated_scan?.status || 'N/A'}
${report.test_results.automated_scan?.total ? `Total: ${report.test_results.automated_scan.total}, Passed: ${report.test_results.automated_scan.passed}, Failed: ${report.test_results.automated_scan.failed}` : ''}

## Vulnerabilities

${report.vulnerabilities.length > 0 ? 
  report.vulnerabilities.map(v => `- **${v.type}** (${v.severity}): ${v.description}`).join('\n') : 
  'No vulnerabilities found.'
}

## Recommendations

${report.recommendations.map(r => `- **${r.priority.toUpperCase()}**: ${r.action} - ${r.description}`).join('\n')}
`;
  
  writeFileSync('security-report.md', textReport);
  console.log('📋 Text report generated: security-report.md');
}

/**
 * 重要度レベルの取得
 */
function getSeverityLevel(testName) {
  const severityMap = {
    'xss_prevention': 'critical',
    'content_sanitization': 'high',
    'performance_impact': 'medium',
    'automated_scan': 'medium'
  };
  
  return severityMap[testName] || 'low';
}

/**
 * 脆弱性の説明取得
 */
function getVulnerabilityDescription(testName) {
  const descriptions = {
    'xss_prevention': 'XSS attacks are not properly prevented',
    'content_sanitization': 'Content is not properly sanitized',
    'performance_impact': 'Security functions have performance issues',
    'automated_scan': 'Automated security scan found issues'
  };
  
  return descriptions[testName] || 'Unknown vulnerability';
}

/**
 * 影響レベルの取得
 */
function getImpactLevel(testName) {
  const impactMap = {
    'xss_prevention': 'high',
    'content_sanitization': 'high',
    'performance_impact': 'low',
    'automated_scan': 'medium'
  };
  
  return impactMap[testName] || 'low';
}

// スクリプトの実行
generateSecurityReport().catch(console.error);