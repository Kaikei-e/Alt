import "@testing-library/jest-dom";

// Global test setup for Vitest

process.env.NEXT_PUBLIC_IDP_ORIGIN =
  process.env.NEXT_PUBLIC_IDP_ORIGIN || "https://id.test.local";
process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL =
  process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || "https://id.test.local";
process.env.NEXT_PUBLIC_APP_ORIGIN =
  process.env.NEXT_PUBLIC_APP_ORIGIN || "https://app.test.local";
process.env.NEXT_PUBLIC_AUTH_SERVICE_URL =
  process.env.NEXT_PUBLIC_AUTH_SERVICE_URL || "https://auth.test.local";
