import { createRequire } from "module";

// Node ESM files (package.json has "type":"module") don't have the CommonJS
// `require` function.  We recreate it via `createRequire` so the conditional
// require below still works when the file is treated as an ES module.
const require = createRequire(import.meta.url);

let withBundleAnalyzer: (
  config: Record<string, unknown>,
) => Record<string, unknown> = (c) => c;

// Build-time env validation (fail fast if missing/malformed)
const mustPublicOrigin = (k: string) => {
  const v = process.env[k];
  if (!v) throw new Error(`[ENV] ${k} is required at build time`);
  let origin: string;
  try {
    origin = new URL(v).origin;
  } catch {
    throw new Error(`[ENV] ${k} must be a valid URL (got: ${v})`);
  }

  // Allow HTTP in test/development environments
  const isTestOrDev =
    process.env.NODE_ENV === "test" || process.env.NODE_ENV === "development";
  const isLocalhost =
    origin.includes("localhost") || origin.includes("127.0.0.1");

  const url = new URL(v);
  const hostIsLocal =
    url.hostname === "localhost" ||
    url.hostname === "127.0.0.1" ||
    url.hostname === "0.0.0.0";

  if (!isTestOrDev && !origin.startsWith("https://") && !hostIsLocal) {
    throw new Error(
      `[ENV] ${k} must be HTTPS origin in production (got: ${origin})`,
    );
  }

  // In test/dev, allow localhost HTTP origins
  if (
    (isTestOrDev && isLocalhost) ||
    origin.startsWith("https://") ||
    hostIsLocal
  ) {
    // Valid
  } else if (!origin.startsWith("https://") && !origin.startsWith("http://")) {
    throw new Error(
      `[ENV] ${k} must be a valid HTTP/HTTPS origin (got: ${origin})`,
    );
  }

  if (/\.cluster\.local(\b|:|\/)/i.test(origin)) {
    throw new Error(`[ENV] ${k} must be PUBLIC FQDN (got: ${origin})`);
  }
};

const resolveKratosProxyDestination = () => {
  const fallback = "https://curionoah.com/ory/:path*";
  const raw =
    process.env.KRATOS_PUBLIC_URL ||
    process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL ||
    "https://curionoah.com/ory";

  try {
    const url = new URL(raw);
    const sanitizedPath = url.pathname.replace(/\/+$/, "");
    const base = sanitizedPath === "/" ? "" : sanitizedPath;
    return `${url.origin}${base}/:path*`;
  } catch {
    console.warn(
      "[next.config] Falling back to default Kratos proxy destination due to invalid URL",
    );
    return fallback;
  }
};

// Only validate in non-test environments to allow test flexibility
if (process.env.NODE_ENV !== "test") {
  mustPublicOrigin("NEXT_PUBLIC_IDP_ORIGIN");
  mustPublicOrigin("NEXT_PUBLIC_KRATOS_PUBLIC_URL");
}

if (process.env.ANALYZE === "true") {
  try {
    const nextBundleAnalyzer = require("@next/bundle-analyzer");
    withBundleAnalyzer = nextBundleAnalyzer({ enabled: true });
  } catch {
    // Module might not be installed in production; log once and continue.
    console.warn(
      "[@next/bundle-analyzer] not installed; skipping bundle analysis.",
    );
  }
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  eslint: {
    // Run eslint via dedicated pnpm script; skip built-in lint during next build to avoid ESLint 9 patch failure
    ignoreDuringBuilds: true,
  },

  // X17.md Phase 17.2: HAR実証済み - local-devを回避する本番ビルドID生成
  generateBuildId: async () => {
    const buildId =
      process.env.GIT_SHA ||
      process.env.VERCEL_GIT_COMMIT_SHA ||
      process.env.BUILD_ID ||
      `production-${Date.now().toString(36)}`;
    console.log(`🏗️ Generated Build ID: ${buildId}`);
    return buildId;
  },

  // 環境変数の明示的な設定（ミドルウェアで確実にアクセスできるよう保証）
  env: {
    KRATOS_INTERNAL_URL:
      process.env.KRATOS_INTERNAL_URL ||
      "http://kratos-public.alt-auth.svc.cluster.local:4433",
    KRATOS_PUBLIC_URL: process.env.KRATOS_PUBLIC_URL || "https://curionoah.com",
  },

  // Essential optimizations
  compress: true,
  poweredByHeader: false,

  // Performance optimizations
  reactStrictMode: true,

  // CSP violations reporting endpoint and /ory proxy for Kratos
  async rewrites() {
    const kratosProxyDestination =
      process.env.NODE_ENV === "test"
        ? "http://localhost:4545/:path*"
        : resolveKratosProxyDestination();
    return [
      {
        source: "/api/csp-report",
        destination: "/api/security/csp-report",
      },
      // Proxy /ory requests to Kratos service (use mock server in test environment)
      {
        source: "/ory/:path*",
        destination: kratosProxyDestination,
      },
    ];
  },

  async headers() {
    const buildId =
      process.env.GIT_SHA ||
      process.env.VERCEL_GIT_COMMIT_SHA ||
      process.env.BUILD_ID ||
      `production-${Date.now().toString(36)}`;
    return [
      {
        source: "/((?!_next/static|_next/image|favicon.ico|robots.txt|icon.svg).*)",
        headers: [
          {
            key: "Cache-Control",
            value: "no-store, no-cache, must-revalidate",
          },
          {
            key: "X-Next-Build-Id",
            value: buildId,
          },
        ],
      },
      {
        source: "/_next/static/(.*)",
        headers: [
          {
            key: "Cache-Control",
            value: "public, max-age=31536000, immutable",
          },
        ],
      },
      {
        source: "/_next/image/(.*)",
        headers: [
          {
            key: "Cache-Control",
            value: "public, max-age=31536000",
          },
        ],
      },
      {
        source: "/icon.svg",
        headers: [
          {
            key: "Cache-Control",
            value: "public, max-age=31536000, immutable",
          },
        ],
      },
    ];
  },

  // Experimental optimizations
  experimental: {
    optimizePackageImports: [
      "@chakra-ui/react",
      "@emotion/react",
      "@chakra-ui/layout",
      "@chakra-ui/icon",
      "@chakra-ui/button",
      "@chakra-ui/spinner",
    ],
    esmExternals: true,
  },

  // Use stable turbopack
  turbopack: {
    resolveExtensions: [".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"],
  },

  // Enhanced webpack optimization for performance
  webpack: (
    config: any,
    { isServer, dev }: { isServer: boolean; dev: boolean },
  ) => {
    // Production optimizations only
    if (!isServer && !dev) {
      // Enhanced tree shaking
      config.optimization.usedExports = true;

      // Better chunk splitting for caching
      if (!config.optimization.splitChunks) {
        config.optimization.splitChunks = { cacheGroups: {} };
      }
      // Better chunk splitting for caching
      if (!config.optimization.splitChunks) {
        config.optimization.splitChunks = { cacheGroups: {} };
      }
      config.optimization.splitChunks.cacheGroups = {
        ...config.optimization.splitChunks.cacheGroups,
        chakra: {
          test: /[\\/]node_modules[\\/](@chakra-ui|framer-motion|@emotion)[\\/]/,
          name: "chakra",
          chunks: "all",
          priority: 30,
          enforce: true,
          reuseExistingChunk: true,
        },
      };

      // Minimize bundle size
      config.optimization.minimize = true;

      // Remove unused CSS
      config.optimization.usedExports = true;
    }

    // Performance optimizations for all builds
    config.resolve.alias = {
      ...config.resolve.alias,
      // Reduce bundle size by aliasing to smaller alternatives if needed
    };

    return config;
  },

  // Compiler optimizations
  compiler: {
    // Remove console logs in production
    removeConsole: process.env.NODE_ENV === "production",
  },

  // Performance budgets
  onDemandEntries: {
    // Period (in ms) where the server will keep pages in the buffer
    maxInactiveAge: 25 * 1000,
    // Number of pages that should be kept simultaneously without being disposed
    pagesBufferLength: 2,
  },
};

export default withBundleAnalyzer(nextConfig);
