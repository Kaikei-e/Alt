/**
 * Main entry point for auth-token-manager
 * Automated OAuth token refresh system using Playwright + Deno 2.0
 */

import { config } from './src/utils/config.ts';
import { InoreaderOAuthAutomator } from './src/auth/oauth.ts';
import { K8sSecretManager } from './src/k8s/secret-manager-simple.ts';

console.log('🚀 Starting auth-token-manager v2.0.0');

async function main() {
  try {
    // Load configuration
    const configOptions = await config.loadConfig();
    
    console.log('✅ Configuration loaded successfully');
    console.log(`Environment: ${config.isProductionMode() ? 'production' : 'development'}`);
    console.log(`Kubernetes namespace: ${configOptions.kubernetes_namespace}`);
    console.log(`Secret name: ${configOptions.secret_name}`);

    // Validate configuration
    if (!config.validateConfig()) {
      console.error('❌ Configuration validation failed');
      Deno.exit(1);
    }

    console.log('✅ Configuration validation successful');

    // Get command from arguments
    const command = Deno.args[0] || 'health';
    
    switch (command) {
      case 'refresh':
        await runTokenRefresh();
        break;
      case 'health':
        await runHealthCheck();
        break;
      case 'validate':
        await runValidation();
        break;
      case 'help':
        showHelp();
        break;
      default:
        console.error(`Unknown command: ${command}`);
        showHelp();
        Deno.exit(1);
    }

  } catch (error) {
    console.error('Critical error during startup:', error);
    Deno.exit(1);
  }
}

async function runTokenRefresh() {
  console.log('🔄 Starting token refresh...');
  
  try {
    const configOptions = await config.loadConfig();
    const credentials = config.getInoreaderCredentials();
    
    // Initialize OAuth automator
    const oauthAutomator = new InoreaderOAuthAutomator(credentials, configOptions.browser);
    
    console.log('🔧 Initializing browser automation...');
    await oauthAutomator.initializeBrowser();
    
    console.log('🔐 Performing OAuth flow...');
    const result = await oauthAutomator.performOAuth();
    
    if (!result.success || !result.tokens) {
      throw new Error(`OAuth failed: ${result.error}`);
    }
    
    console.log('💾 Storing tokens to Kubernetes secret...');
    const secretManager = new K8sSecretManager(
      configOptions.kubernetes_namespace,
      configOptions.secret_name
    );
    
    await secretManager.updateTokenSecret(result.tokens);
    
    console.log('✅ Token refresh completed successfully');
    console.log(`🕒 New token expires at: ${result.tokens.expires_at}`);
    
    // Cleanup
    await oauthAutomator.cleanup();
    
  } catch (error) {
    console.error('❌ Token refresh failed:', error);
    throw error;
  }
}

async function runHealthCheck() {
  console.log('🔍 Running health check...');

  try {
    const checks = {
      config_valid: config.validateConfig(),
      environment_ready: true,
      kubernetes_ready: false, // TODO: Check K8s connectivity
      oauth_automation_ready: false, // TODO: Check browser/playwright
    };

    const healthyChecks = Object.values(checks).filter(Boolean).length;
    const totalChecks = Object.keys(checks).length;
    
    let status: 'healthy' | 'degraded' | 'unhealthy';
    if (healthyChecks === totalChecks) {
      status = 'healthy';
    } else if (healthyChecks > 0) {
      status = 'degraded';
    } else {
      status = 'unhealthy';
    }

    console.log('Health Check Results:');
    console.log(`Status: ${status.toUpperCase()}`);
    console.log(`Checks: ${healthyChecks}/${totalChecks} passing`);
    
    for (const [check, result] of Object.entries(checks)) {
      console.log(`  ${result ? '✅' : '❌'} ${check}`);
    }

    if (status === 'unhealthy') {
      Deno.exit(1);
    }

  } catch (error) {
    console.error('Health check failed:', error);
    throw error;
  }
}

async function runValidation() {
  console.log('🔍 Running configuration validation...');

  try {
    const configOptions = await config.loadConfig();
    const credentials = config.getInoreaderCredentials();
    const k8sConfig = config.getKubernetesConfig();

    console.log('Configuration Validation Results:');
    console.log('✅ Configuration loaded successfully');
    console.log(`✅ Environment: ${config.isProductionMode() ? 'production' : 'development'}`);
    console.log(`✅ Kubernetes namespace: ${k8sConfig.namespace}`);
    console.log(`✅ Secret name: ${k8sConfig.secretName}`);
    console.log(`✅ Browser headless: ${configOptions.browser.headless}`);
    console.log(`✅ Retry max attempts: ${configOptions.retry.max_attempts}`);
    console.log(`✅ Log level: ${configOptions.logger.level}`);
    console.log('✅ Inoreader credentials present');

    console.log('✅ Configuration validation completed successfully');

  } catch (error) {
    console.error('Configuration validation failed:', error);
    console.log('❌ Configuration validation failed');
    throw error;
  }
}

function showHelp() {
  console.log(`
🤖 Auth Token Manager v2.0.0
Automated OAuth token refresh system using Playwright + Deno 2.0

USAGE:
  deno run --allow-all main.ts [COMMAND]

COMMANDS:
  refresh    Refresh OAuth tokens (default)
  health     Run health check
  validate   Validate configuration
  help       Show this help message

ENVIRONMENT VARIABLES:
  Required:
    INOREADER_USERNAME      Inoreader username
    INOREADER_PASSWORD      Inoreader password  
    INOREADER_CLIENT_ID     OAuth client ID
    INOREADER_CLIENT_SECRET OAuth client secret

  Optional:
    KUBERNETES_NAMESPACE         Kubernetes namespace (default: alt-processing)
    SECRET_NAME                  Secret name (default: inoreader-tokens)
    BROWSER_HEADLESS            Run browser in headless mode (default: true)
    LOG_LEVEL                   Log level (default: INFO)
    RETRY_MAX_ATTEMPTS          Max retry attempts (default: 3)

EXAMPLES:
  # Refresh tokens
  deno run --allow-all main.ts refresh

  # Check system health
  deno run --allow-all main.ts health

  # Validate configuration
  deno run --allow-all main.ts validate

For more information, see: https://github.com/Kaikei-e/Alt
`);
}

// Handle graceful shutdown
function setupSignalHandlers() {
  const signals: Deno.Signal[] = ['SIGINT', 'SIGTERM'];
  
  for (const signal of signals) {
    Deno.addSignalListener(signal, () => {
      console.log(`Received ${signal}, shutting down gracefully...`);
      Deno.exit(0);
    });
  }
}

// Error boundary
globalThis.addEventListener('error', (event) => {
  console.error('Unhandled error:', event.error);
  Deno.exit(1);
});

globalThis.addEventListener('unhandledrejection', (event) => {
  console.error('Unhandled promise rejection:', event.reason);
  Deno.exit(1);
});

// Setup signal handlers
setupSignalHandlers();

// Run main function
if (import.meta.main) {
  await main();
}