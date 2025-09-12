#!/usr/bin/env node

/**
 * Pre-test setup validation script
 */

const { exec } = require("child_process");
const fs = require("fs");
const path = require("path");

function checkPortAvailable(port) {
  return new Promise((resolve) => {
    exec(`lsof -i :${port}`, (error, stdout) => {
      if (error || !stdout.trim()) {
        resolve(true); // Port is available
      } else {
        resolve(false); // Port is in use
      }
    });
  });
}

function validateEnvironment() {
  const requiredEnvVars = [
    "NEXT_PUBLIC_APP_ORIGIN",
    "NEXT_PUBLIC_KRATOS_PUBLIC_URL",
    "NEXT_PUBLIC_IDP_ORIGIN",
  ];

  const missing = requiredEnvVars.filter((envVar) => !process.env[envVar]);

  if (missing.length > 0) {
    console.log("⚠️  Missing environment variables:", missing);
    console.log("💡 Make sure .env.test is loaded");
  } else {
    console.log("✅ Environment variables are set");
  }

  return missing.length === 0;
}

function checkDependencies() {
  const packageJsonPath = path.join(__dirname, "..", "package.json");

  if (!fs.existsSync(packageJsonPath)) {
    console.log("❌ package.json not found");
    return false;
  }

  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, "utf8"));
  const hasPlaywright =
    packageJson.devDependencies?.["@playwright/test"] ||
    packageJson.dependencies?.["@playwright/test"];

  if (!hasPlaywright) {
    console.log("❌ Playwright is not installed");
    return false;
  }

  console.log("✅ Dependencies check passed");
  return true;
}

async function checkPorts() {
  const ports = [3010, 4545];
  const conflicts = [];

  for (const port of ports) {
    const available = await checkPortAvailable(port);
    if (!available) {
      conflicts.push(port);
    }
  }

  if (conflicts.length > 0) {
    console.log("⚠️  Ports in use:", conflicts);
    console.log("💡 Consider stopping other services or changing ports");
  } else {
    console.log("✅ Required ports are available");
  }

  return conflicts.length === 0;
}

function checkBrowsers() {
  return new Promise((resolve) => {
    exec("npx playwright install --dry-run", (error, stdout, stderr) => {
      if (error || stderr.includes("not found")) {
        console.log("⚠️  Some browsers may need installation");
        console.log("💡 Run: npx playwright install");
        resolve(false);
      } else {
        console.log("✅ Browsers are installed");
        resolve(true);
      }
    });
  });
}

function ensureDirectories() {
  const dirs = ["playwright/.auth", "test-results", "playwright-report"];

  dirs.forEach((dir) => {
    const dirPath = path.join(__dirname, "..", dir);
    if (!fs.existsSync(dirPath)) {
      fs.mkdirSync(dirPath, { recursive: true });
      console.log(`📁 Created directory: ${dir}`);
    }
  });

  console.log("✅ Directories are ready");
}

async function main() {
  console.log("🚀 Running pre-test setup validation...\n");

  let allGood = true;

  // Check dependencies
  if (!checkDependencies()) allGood = false;

  // Validate environment
  if (!validateEnvironment()) allGood = false;

  // Check ports
  if (!(await checkPorts())) {
    console.log("ℹ️  Port conflicts detected but tests may still work");
  }

  // Check browsers
  if (!(await checkBrowsers())) {
    console.log("ℹ️  Browser installation issues detected");
  }

  // Ensure directories exist
  ensureDirectories();

  console.log("\n" + "=".repeat(50));

  if (allGood) {
    console.log("✅ Pre-test validation completed successfully!");
    console.log("🎯 Ready to run Playwright tests");
    process.exit(0);
  } else {
    console.log("⚠️  Some issues detected but tests might still work");
    console.log("🔧 Consider fixing the issues above for best results");
    process.exit(0); // Don't fail completely, just warn
  }
}

if (require.main === module) {
  main().catch((error) => {
    console.error("❌ Setup validation failed:", error);
    process.exit(1);
  });
}

module.exports = { validateEnvironment, checkPorts, checkDependencies };
