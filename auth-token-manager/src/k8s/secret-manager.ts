/**
 * Simplified Kubernetes Secret management for OAuth tokens
 */

import { kubernetes_client } from 'kubernetes_client';
import { CoreV1Api } from 'kubernetes_apis';
import { encode as base64Encode } from '@std/encoding/base64';
import type { TokenResponse, K8sSecretData, K8sError } from '../auth/types.ts';

export class K8sSecretManager {
  private k8sApi: CoreV1Api;
  
  constructor(
    private namespace: string,
    private secretName: string
  ) {
    const k8sClient = kubernetes_client.getDefaultClient();
    this.k8sApi = new CoreV1Api(k8sClient);
  }

  async updateTokenSecret(tokens: TokenResponse): Promise<void> {
    try {
      console.log(`🔐 Updating Kubernetes secret: ${this.secretName} in namespace: ${this.namespace}`);

      const secretData: K8sSecretData = {
        access_token: tokens.access_token,
        refresh_token: tokens.refresh_token,
        expires_at: tokens.expires_at.toISOString(),
        updated_at: new Date().toISOString()
      };

      // Encode secret data to base64
      const encodedData: Record<string, string> = {};
      for (const [key, value] of Object.entries(secretData)) {
        encodedData[key] = base64Encode(new TextEncoder().encode(value));
      }

      const secretBody = {
        metadata: {
          name: this.secretName,
          namespace: this.namespace,
          labels: {
            'app': 'auth-token-manager',
            'component': 'oauth-tokens',
            'managed-by': 'auth-token-manager'
          }
        },
        type: 'Opaque',
        data: encodedData
      };

      try {
        // Try to update existing secret
        await this.k8sApi.patchNamespacedSecret(
          this.secretName,
          this.namespace,
          secretBody,
          {
            headers: {
              'Content-Type': 'application/merge-patch+json'
            }
          }
        );
        console.log('✅ Secret updated successfully');
      } catch (error) {
        // If secret doesn't exist, create it
        if (error instanceof Error && error.message.includes('404')) {
          console.log('🆕 Secret not found, creating new one...');
          await this.k8sApi.createNamespacedSecret(this.namespace, secretBody);
          console.log('✅ Secret created successfully');
        } else {
          throw error;
        }
      }

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      console.error('❌ Failed to update Kubernetes secret:', errorMessage);
      
      throw new Error(`K8s secret update failed: ${errorMessage}`) as K8sError;
    }
  }

  /**
   * Retrieve OAuth token from Kubernetes Secret
   */
  async retrieveToken(secretName?: string): Promise<OAuth2Token | null> {
    const name = secretName || this.generateSecretName();

    const operation = async () => {
      try {
        const secret = await this.getSecret(name);
        if (!secret) {
          logger.warn('Token secret not found', {
            secret_name: name,
            namespace: this.config.namespace
          });
          return null;
        }

        const tokenData = this.extractTokenFromSecret(secret);
        
        logger.info('Token retrieved successfully from Kubernetes Secret', {
          secret_name: name,
          namespace: this.config.namespace,
          expires_at: tokenData.expires_at,
          is_active: tokenData.is_active
        });

        return tokenData;

      } catch (error) {
        const k8sError: KubernetesError = {
          type: 'kubernetes_error',
          code: this.mapErrorCode(error),
          message: `Failed to retrieve token from secret ${name}: ${error.message}`,
          details: {
            namespace: this.config.namespace,
            secret_name: name,
            api_version: 'v1'
          }
        };

        logger.error('Failed to retrieve token from Kubernetes Secret', k8sError);
        throw k8sError;
      }
    };

    return await retryWithBackoff(operation, {
      max_attempts: 3,
      initial_delay: 500,
      max_delay: 3000,
      backoff_factor: 2,
      jitter: true,
      retryable_status_codes: [429, 500, 502, 503, 504],
      retryable_errors: ['timeout', 'network', 'api']
    });
  }

  /**
   * Delete OAuth token secret from Kubernetes
   */
  async deleteToken(secretName?: string): Promise<void> {
    const name = secretName || this.generateSecretName();

    const operation = async () => {
      try {
        await this.k8sApi.deleteNamespacedSecret(name, this.config.namespace);

        logger.info('Token secret deleted successfully', {
          secret_name: name,
          namespace: this.config.namespace
        });

      } catch (error) {
        if (error.status === 404) {
          logger.warn('Token secret not found for deletion', {
            secret_name: name,
            namespace: this.config.namespace
          });
          return; // Don't treat 404 as an error for deletion
        }

        const k8sError: KubernetesError = {
          type: 'kubernetes_error',
          code: this.mapErrorCode(error),
          message: `Failed to delete token secret ${name}: ${error.message}`,
          details: {
            namespace: this.config.namespace,
            secret_name: name,
            api_version: 'v1',
            status_code: error.status
          }
        };

        logger.error('Failed to delete token secret', k8sError);
        throw k8sError;
      }
    };

    await retryWithBackoff(operation, {
      max_attempts: 2,
      initial_delay: 1000,
      max_delay: 3000,
      backoff_factor: 2,
      jitter: true,
      retryable_status_codes: [429, 500, 502, 503, 504],
      retryable_errors: ['timeout', 'network', 'api']
    });
  }

  /**
   * List all OAuth token secrets in the namespace
   */
  async listTokenSecrets(): Promise<Array<{ name: string; created: Date; expires_at?: number }>> {
    const operation = async () => {
      try {
        const labelSelector = Object.entries(this.config.labels)
          .map(([key, value]) => `${key}=${value}`)
          .join(',');

        const secretList = await this.k8sApi.listNamespacedSecret(
          this.config.namespace,
          undefined, // pretty
          undefined, // allowWatchBookmarks
          undefined, // continue
          undefined, // fieldSelector
          labelSelector // labelSelector
        );

        const tokenSecrets = secretList.body.items
          .filter(secret => secret.metadata?.name?.includes('oauth-token'))
          .map(secret => {
            const createdTime = secret.metadata?.creationTimestamp
              ? new Date(secret.metadata.creationTimestamp)
              : new Date();

            let expiresAt: number | undefined;
            try {
              if (secret.data?.['token-data']) {
                const tokenData = JSON.parse(
                  atob(secret.data['token-data'])
                );
                expiresAt = tokenData.expires_at;
              }
            } catch (error) {
              // Ignore parsing errors for expires_at
            }

            return {
              name: secret.metadata?.name || '',
              created: createdTime,
              expires_at: expiresAt
            };
          });

        logger.info('Token secrets listed successfully', {
          namespace: this.config.namespace,
          secret_count: tokenSecrets.length
        });

        return tokenSecrets;

      } catch (error) {
        const k8sError: KubernetesError = {
          type: 'kubernetes_error',
          code: this.mapErrorCode(error),
          message: `Failed to list token secrets: ${error.message}`,
          details: {
            namespace: this.config.namespace,
            api_version: 'v1'
          }
        };

        logger.error('Failed to list token secrets', k8sError);
        throw k8sError;
      }
    };

    return await retryWithBackoff(operation, {
      max_attempts: 3,
      initial_delay: 500,
      max_delay: 3000,
      backoff_factor: 2,
      jitter: true,
      retryable_status_codes: [429, 500, 502, 503, 504],
      retryable_errors: ['timeout', 'network', 'api']
    });
  }

  /**
   * Check if token secret exists
   */
  async tokenExists(secretName?: string): Promise<boolean> {
    const name = secretName || this.generateSecretName();

    try {
      const secret = await this.getSecret(name);
      return secret !== null;
    } catch (error) {
      logger.debug('Token existence check failed', {
        secret_name: name,
        error: error.message
      });
      return false;
    }
  }

  /**
   * Validate Kubernetes RBAC permissions
   */
  async validateRBACPermissions(): Promise<boolean> {
    try {
      // Test read permissions
      await this.k8sApi.listNamespacedSecret(
        this.config.namespace,
        undefined, // pretty
        undefined, // allowWatchBookmarks
        undefined, // continue
        undefined, // fieldSelector
        undefined, // labelSelector
        1 // limit
      );

      // Test write permissions by creating a temporary secret
      const testSecretName = `rbac-test-${Date.now()}`;
      const testSecret: V1Secret = {
        apiVersion: 'v1',
        kind: 'Secret',
        metadata: {
          name: testSecretName,
          namespace: this.config.namespace,
          labels: {
            'app.kubernetes.io/name': 'auth-token-manager',
            'app.kubernetes.io/component': 'rbac-test'
          }
        },
        type: 'Opaque',
        data: {
          test: btoa('rbac-validation')
        }
      };

      await this.k8sApi.createNamespacedSecret(this.config.namespace, testSecret);
      
      // Clean up test secret
      await this.k8sApi.deleteNamespacedSecret(testSecretName, this.config.namespace);

      logger.info('RBAC permissions validated successfully', {
        namespace: this.config.namespace,
        service_account: this.config.service_account
      });

      return true;

    } catch (error) {
      logger.error('RBAC permission validation failed', {
        namespace: this.config.namespace,
        service_account: this.config.service_account,
        error: error.message
      });

      return false;
    }
  }

  /**
   * Create new token secret
   */
  private async createTokenSecret(name: string, token: OAuth2Token): Promise<void> {
    const secretData = this.prepareSecretData(token);
    
    const secret: V1Secret = {
      apiVersion: 'v1',
      kind: 'Secret',
      metadata: {
        name,
        namespace: this.config.namespace,
        labels: {
          ...this.config.labels,
          'auth-token-manager.alt.dev/token-type': 'oauth2',
          'auth-token-manager.alt.dev/provider': 'inoreader',
          'auth-token-manager.alt.dev/source': token.source
        },
        annotations: {
          ...this.config.annotations,
          'auth-token-manager.alt.dev/created-at': new Date().toISOString(),
          'auth-token-manager.alt.dev/expires-at': new Date(token.expires_at).toISOString(),
          'auth-token-manager.alt.dev/refresh-count': token.refresh_count.toString()
        }
      },
      type: 'Opaque',
      data: secretData
    };

    await this.k8sApi.createNamespacedSecret(this.config.namespace, secret);

    logger.debug('Token secret created', {
      secret_name: name,
      namespace: this.config.namespace,
      labels: Object.keys(secret.metadata?.labels || {}),
      annotations: Object.keys(secret.metadata?.annotations || {})
    });
  }

  /**
   * Update existing token secret
   */
  private async updateTokenSecret(name: string, token: OAuth2Token): Promise<void> {
    const secretData = this.prepareSecretData(token);

    const secret: V1Secret = {
      apiVersion: 'v1',
      kind: 'Secret',
      metadata: {
        name,
        namespace: this.config.namespace,
        labels: {
          ...this.config.labels,
          'auth-token-manager.alt.dev/token-type': 'oauth2',
          'auth-token-manager.alt.dev/provider': 'inoreader',
          'auth-token-manager.alt.dev/source': token.source
        },
        annotations: {
          ...this.config.annotations,
          'auth-token-manager.alt.dev/updated-at': new Date().toISOString(),
          'auth-token-manager.alt.dev/expires-at': new Date(token.expires_at).toISOString(),
          'auth-token-manager.alt.dev/refresh-count': token.refresh_count.toString()
        }
      },
      type: 'Opaque',
      data: secretData
    };

    await this.k8sApi.replaceNamespacedSecret(name, this.config.namespace, secret);

    logger.debug('Token secret updated', {
      secret_name: name,
      namespace: this.config.namespace,
      refresh_count: token.refresh_count
    });
  }

  /**
   * Get secret from Kubernetes API
   */
  private async getSecret(name: string): Promise<V1Secret | null> {
    try {
      const response = await this.k8sApi.readNamespacedSecret(name, this.config.namespace);
      return response.body;
    } catch (error) {
      if (error.status === 404) {
        return null;
      }
      throw error;
    }
  }

  /**
   * Prepare secret data with optional encryption
   */
  private prepareSecretData(token: OAuth2Token): Record<string, string> {
    const tokenJson = JSON.stringify(token, null, 0);
    
    let tokenData: string;
    if (this.config.encryption.enabled) {
      // In a real implementation, you would encrypt the token data here
      // For now, we'll just base64 encode it as Kubernetes requires
      tokenData = btoa(tokenJson);
    } else {
      tokenData = btoa(tokenJson);
    }

    return {
      'token-data': tokenData,
      'access-token': btoa(token.access_token),
      'token-type': btoa(token.token_type),
      'expires-at': btoa(token.expires_at.toString()),
      'created-at': btoa(token.created_at.toString()),
      'source': btoa(token.source)
    };
  }

  /**
   * Extract token data from secret
   */
  private extractTokenFromSecret(secret: V1Secret): OAuth2Token {
    if (!secret.data) {
      throw new Error('Secret data is empty');
    }

    try {
      const tokenDataEncoded = secret.data['token-data'];
      if (!tokenDataEncoded) {
        throw new Error('Token data not found in secret');
      }

      const tokenJson = atob(tokenDataEncoded);
      const tokenData = JSON.parse(tokenJson);

      // Validate token structure
      if (!tokenData.access_token || !tokenData.token_type || !tokenData.expires_at) {
        throw new Error('Invalid token structure in secret');
      }

      return tokenData as OAuth2Token;

    } catch (error) {
      throw new Error(`Failed to extract token from secret: ${error.message}`);
    }
  }

  /**
   * Generate standardized secret name
   */
  private generateSecretName(): string {
    const baseName = this.config.secret_name || 'inoreader-oauth-token';
    return `${baseName}-${Date.now().toString(36)}`;
  }

  /**
   * Map Kubernetes API errors to standardized error codes
   */
  private mapErrorCode(error: any): KubernetesError['code'] {
    if (error.status === 401 || error.status === 403) {
      return 'UNAUTHORIZED';
    }
    if (error.status === 404) {
      return 'SECRET_NOT_FOUND';
    }
    if (error.message?.includes('namespace')) {
      return 'NAMESPACE_NOT_FOUND';
    }
    return 'API_ERROR';
  }

  /**
   * Get current configuration (without sensitive data)
   */
  getConfig() {
    return {
      namespace: this.config.namespace,
      secret_name: this.config.secret_name,
      service_account: this.config.service_account,
      labels: this.config.labels,
      annotations: this.config.annotations,
      encryption_enabled: this.config.encryption.enabled
    };
  }

  /**
   * Health check for Kubernetes connectivity and permissions
   */
  async healthCheck(): Promise<{
    healthy: boolean;
    message: string;
    details: Record<string, unknown>;
  }> {
    try {
      // Test namespace access
      await this.k8sApi.readNamespace(this.config.namespace);

      // Test RBAC permissions
      const rbacValid = await this.validateRBACPermissions();

      if (!rbacValid) {
        return {
          healthy: false,
          message: 'RBAC permissions insufficient',
          details: {
            namespace: this.config.namespace,
            service_account: this.config.service_account
          }
        };
      }

      return {
        healthy: true,
        message: 'Kubernetes Secret manager healthy',
        details: {
          namespace: this.config.namespace,
          service_account: this.config.service_account,
          rbac_validated: true
        }
      };

    } catch (error) {
      return {
        healthy: false,
        message: `Kubernetes connectivity failed: ${error.message}`,
        details: {
          namespace: this.config.namespace,
          error_code: error.status || 'unknown'
        }
      };
    }
  }
}