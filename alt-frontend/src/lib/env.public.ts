// Centralized public env constants for client/runtime usage
// Use only static property access so Next can inline at build time.

function assertHttpsPublicOrigin(name: string, value: string | undefined): string {
  if (!value) throw new Error(`${name} missing`);
  let origin: string;
  try {
    origin = new URL(value).origin;
  } catch {
    throw new Error(`${name} must be a valid URL (got: ${value})`);
  }
  if (!origin.startsWith('https://')) {
    throw new Error(`${name} must be HTTPS origin (got: ${origin})`);
  }
  const clusterLocalPattern = new RegExp('\\.' + 'cluster' + '\\.' + 'local' + '(\\b|:|\/)', 'i');
  if (clusterLocalPattern.test(origin)) {
    throw new Error(`${name} must be PUBLIC FQDN (got: ${origin})`);
  }
  return origin;
}

export const IDP_ORIGIN = assertHttpsPublicOrigin(
  'NEXT_PUBLIC_IDP_ORIGIN',
  process.env.NEXT_PUBLIC_IDP_ORIGIN,
);

export const KRATOS_PUBLIC_URL = assertHttpsPublicOrigin(
  'NEXT_PUBLIC_KRATOS_PUBLIC_URL',
  process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL,
);

// Optional public endpoints for CSP/connect-src etc.
export const PUBLIC_API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
export const PUBLIC_WS_URL = process.env.NEXT_PUBLIC_WS_URL || 'ws://localhost:8080';
export const PUBLIC_API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL || 'http://localhost:9000';
