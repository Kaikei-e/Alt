#!/usr/bin/env node

/**
 * Health check script for web servers before running tests
 */

const http = require('http');

async function checkServer(url, name, timeout = 30000) {
  return new Promise((resolve, reject) => {
    const startTime = Date.now();
    
    const checkHealth = () => {
      const req = http.get(url, (res) => {
        if (res.statusCode >= 200 && res.statusCode < 400) {
          console.log(`✅ ${name} is healthy (${res.statusCode})`);
          resolve(true);
        } else {
          console.log(`⚠️  ${name} returned ${res.statusCode}`);
          scheduleNextCheck();
        }
      });

      req.on('error', (error) => {
        if (Date.now() - startTime > timeout) {
          console.log(`❌ ${name} failed to start within ${timeout}ms`);
          reject(new Error(`${name} health check timeout`));
        } else {
          console.log(`⏳ ${name} not ready yet, retrying...`);
          scheduleNextCheck();
        }
      });

      req.setTimeout(5000, () => {
        req.destroy();
        scheduleNextCheck();
      });
    };

    const scheduleNextCheck = () => {
      setTimeout(checkHealth, 1000);
    };

    checkHealth();
  });
}

async function main() {
  console.log('🔍 Checking server health...');
  
  try {
    // Check mock auth service
    await checkServer('http://localhost:4545/sessions/whoami', 'Mock Auth Service', 15000);
    
    // Check Next.js app
    await checkServer('http://localhost:3010', 'Next.js App', 60000);
    
    console.log('✅ All servers are healthy!');
    process.exit(0);
  } catch (error) {
    console.error('❌ Health check failed:', error.message);
    process.exit(1);
  }
}

if (require.main === module) {
  main();
}