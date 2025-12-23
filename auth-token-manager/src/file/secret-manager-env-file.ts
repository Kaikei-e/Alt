/**
 * File-based Secret management for OAuth tokens (.env format)
 */

import type { TokenResponse, SecretManager, SecretData } from '../auth/types.ts';

export class EnvFileSecretManager implements SecretManager {
  constructor(
    private filePath: string
  ) { }

  async updateTokenSecret(tokens: TokenResponse): Promise<void> {
    try {
      console.log(`🔐 Updating token file: ${this.filePath}`);

      // Read existing content if file exists
      let content = '';
      try {
        content = await Deno.readTextFile(this.filePath);
      } catch (error) {
        if (!(error instanceof Deno.errors.NotFound)) {
          throw error;
        }
        // File doesn't exist, start empty
      }

      // Parse existing lines to preserve non-token vars
      const lines = content.split('\n');
      const newLines: string[] = [];

      const tokenKeys = [
        'OAUTH2_ACCESS_TOKEN',
        'OAUTH2_REFRESH_TOKEN',
        'OAUTH2_TOKEN_TYPE',
        'OAUTH2_EXPIRES_AT',
        'OAUTH2_EXPIRES_IN'
      ];

      // Keep lines that are not token keys
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) continue;

        let isTokenKey = false;
        for (const key of tokenKeys) {
          if (trimmed.startsWith(`${key}=`)) {
            isTokenKey = true;
            break;
          }
        }

        if (!isTokenKey) {
          newLines.push(line);
        }
      }

      // Add new token values
      const expiresIn = Math.floor((tokens.expires_at.getTime() - Date.now()) / 1000);

      newLines.push(`OAUTH2_ACCESS_TOKEN=${tokens.access_token}`);
      newLines.push(`OAUTH2_REFRESH_TOKEN=${tokens.refresh_token}`);
      newLines.push(`OAUTH2_TOKEN_TYPE=${tokens.token_type || 'Bearer'}`);
      newLines.push(`OAUTH2_EXPIRES_AT=${tokens.expires_at.toISOString()}`);
      newLines.push(`OAUTH2_EXPIRES_IN=${expiresIn}`);

      // Write back to file
      await Deno.writeTextFile(this.filePath, newLines.join('\n') + '\n');

      console.log('✅ Token file updated successfully');
      console.log(`🕐 Token expires at: ${tokens.expires_at.toISOString()}`);

    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      console.error('❌ Failed to update token file:', errorMessage);
      throw new Error(`File update failed: ${errorMessage}`);
    }
  }

  async getTokenSecret(): Promise<SecretData | null> {
    try {
      console.log(`💫 Reading token file: ${this.filePath}`);

      const content = await Deno.readTextFile(this.filePath);
      const lines = content.split('\n');

      const data: Record<string, string> = {};

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith('#')) continue;

        const parts = trimmed.split('=', 2);
        if (parts.length === 2) {
          data[parts[0].trim()] = parts[1].trim();
        }
      }

      if (!data['OAUTH2_ACCESS_TOKEN'] || !data['OAUTH2_REFRESH_TOKEN']) {
        return null;
      }

      return {
        access_token: data['OAUTH2_ACCESS_TOKEN'],
        refresh_token: data['OAUTH2_REFRESH_TOKEN'],
        expires_at: data['OAUTH2_EXPIRES_AT'] || '',
        updated_at: new Date().toISOString(), // Approximate since we don't store update time in .env
        token_type: data['OAUTH2_TOKEN_TYPE'] || 'Bearer',
        scope: 'read write'
      };

    } catch (error) {
      if (error instanceof Deno.errors.NotFound) {
        return null;
      }
      console.error('❌ Failed to read token file:', error);
      throw error;
    }
  }

  async checkSecretExists(): Promise<boolean> {
    try {
      await Deno.stat(this.filePath);
      return true;
    } catch {
      return false;
    }
  }
}
