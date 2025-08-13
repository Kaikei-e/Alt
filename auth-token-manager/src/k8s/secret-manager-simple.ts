/**
 * Simplified Kubernetes Secret management for OAuth tokens
 */

// Simplified approach: use kubectl commands instead of kubernetes client libs
// import { kubernetes_client } from 'kubernetes_client';
// import { CoreV1Api } from 'kubernetes_apis';
import { encodeBase64 } from '@std/encoding/base64';
import type { TokenResponse, K8sSecretData, K8sError } from '../auth/types.ts';

export class K8sSecretManager {
  constructor(
    private namespace: string,
    private secretName: string
  ) {}
  
  private async runKubectl(args: string[]): Promise<string> {
    const cmd = new Deno.Command('kubectl', {
      args,
      stdout: 'piped',
      stderr: 'piped'
    });
    
    const result = await cmd.output();
    
    if (!result.success) {
      const errorText = new TextDecoder().decode(result.stderr);
      throw new Error(`kubectl failed: ${errorText}`);
    }
    
    return new TextDecoder().decode(result.stdout);
  }

  async updateTokenSecret(tokens: TokenResponse): Promise<void> {
    try {
      console.log(`🔐 Updating Kubernetes secret: ${this.secretName} in namespace: ${this.namespace}`);

      // Create token data in pre-processor-sidecar expected format
      const tokenData = {
        access_token: tokens.access_token,
        refresh_token: tokens.refresh_token,
        token_type: tokens.token_type || "Bearer",
        expires_in: Math.floor((tokens.expires_at.getTime() - Date.now()) / 1000),
        expires_at: tokens.expires_at.toISOString(),
        scope: tokens.scope || "read write"
      };

      console.log(`📋 Token data prepared:`, {
        expires_in: tokenData.expires_in,
        expires_at: tokenData.expires_at,
        scope: tokenData.scope
      });

      // Encode token data as JSON for pre-processor-sidecar compatibility
      const tokenDataJson = JSON.stringify(tokenData);
      const encodedTokenData = encodeBase64(tokenDataJson);

      // Create secret YAML with pre-processor-sidecar expected format
      const secretYaml = `
apiVersion: v1
kind: Secret
metadata:
  name: ${this.secretName}
  namespace: ${this.namespace}
  labels:
    app: auth-token-manager
    component: oauth-tokens
    managed-by: auth-token-manager
    target-service: pre-processor-sidecar
type: Opaque
data:
  token_data: ${encodedTokenData}
`;

      // Write secret to temporary file
      const tempFile = `/tmp/secret-${Date.now()}.yaml`;
      await Deno.writeTextFile(tempFile, secretYaml);

      try {
        // Try to apply the secret
        await this.runKubectl(['apply', '-f', tempFile]);
        console.log('✅ Secret updated successfully in pre-processor-sidecar format');
        console.log(`🕐 Token expires at: ${tokenData.expires_at}`);
        console.log(`⏱️  Token valid for: ${tokenData.expires_in} seconds`);
      } finally {
        // Clean up temp file
        try {
          await Deno.remove(tempFile);
        } catch {
          // Ignore cleanup errors
        }
      }

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      console.error('❌ Failed to update Kubernetes secret:', errorMessage);
      
      throw new Error(`K8s secret update failed: ${errorMessage}`) as K8sError;
    }
  }


  async getTokenSecret(): Promise<K8sSecretData | null> {
    try {
      console.log(`💫 Reading Kubernetes secret: ${this.secretName} in namespace: ${this.namespace}`);

      const output = await this.runKubectl([
        'get', 'secret', this.secretName,
        '-n', this.namespace,
        '-o', 'jsonpath={.data.token_data}'
      ]);
      
      if (!output || output.trim() === '') {
        console.log('ℹ️ Secret not found or token_data missing');
        return null;
      }

      // Decode the token_data field which contains JSON
      const tokenDataDecoded = new TextDecoder().decode(
        Uint8Array.from(atob(output.trim()), c => c.charCodeAt(0))
      );
      
      const tokenData = JSON.parse(tokenDataDecoded);
      
      console.log('📋 Found token data:', {
        expires_at: tokenData.expires_at,
        token_type: tokenData.token_type,
        scope: tokenData.scope
      });
      
      // Convert to our expected format
      const result: K8sSecretData = {
        access_token: tokenData.access_token,
        refresh_token: tokenData.refresh_token,
        expires_at: tokenData.expires_at,
        updated_at: new Date().toISOString()
      };

      if (!result.access_token || !result.refresh_token) {
        console.log('⚠️ Secret missing required token fields');
        return null;
      }

      console.log('✅ Secret read successfully');
      return result;

    } catch (error) {
      if (error instanceof Error && error.message.includes('NotFound')) {
        console.log('ℹ️ Secret not found');
        return null;
      }

      const errorMessage = error instanceof Error ? error.message : String(error);
      console.error('❌ Failed to read Kubernetes secret:', errorMessage);
      
      throw new Error(`K8s secret read failed: ${errorMessage}`) as K8sError;
    }
  }

  async checkSecretExists(): Promise<boolean> {
    try {
      await this.runKubectl(['get', 'secret', this.secretName, '-n', this.namespace]);
      return true;
    } catch (error) {
      return false;
    }
  }
}