/**
 * DEPRECATED: This file is kept for backwards compatibility.
 *
 * For new code:
 * - Browser code: import { createClientTransport } from "$lib/connect" (or transport-client.ts)
 * - Server code: import { createServerTransport } from "$lib/connect/transport-server"
 *
 * DO NOT import this file in browser code - it will cause build errors
 * due to $env/dynamic/private imports.
 */

// Re-export from new locations
export { createClientTransport } from "./transport-client";
export { createServerTransport } from "./transport-server";
