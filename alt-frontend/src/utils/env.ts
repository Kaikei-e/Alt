/**
 * Utilities for working with environment variables in browser-safe contexts.
 */

/**
 * Returns the value of a public environment variable or throws if it is absent.
 */
export function requiredPublicEnv(name: string): string {
  const value = process.env[name];

  if (!value || value.trim() === "") {
    throw new Error(`Missing required environment variable: ${name}`);
  }

  return value;
}
