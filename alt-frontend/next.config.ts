import { createRequire } from "module";
import { securityHeaders } from "./src/config/security";

// Node ESM files (package.json has "type":"module") don’t have the CommonJS
// `require` function.  We recreate it via `createRequire` so the conditional
// require below still works when the file is treated as an ES module.
// eslint-disable-next-line @typescript-eslint/no-var-requires
const require = createRequire(import.meta.url);

let withBundleAnalyzer: (
  config: Record<string, unknown>,
) => Record<string, unknown> = (c) => c;

if (process.env.ANALYZE === "true") {
  try {
    // eslint-disable-next-line @typescript-eslint/no-var-requires, import/no-extraneous-dependencies
    const nextBundleAnalyzer = require("@next/bundle-analyzer");
    withBundleAnalyzer = nextBundleAnalyzer({ enabled: true });
  } catch (err) {
    // Module might not be installed in production; log once and continue.
    // eslint-disable-next-line no-console
    console.warn(
      "[@next/bundle-analyzer] not installed; skipping bundle analysis.",
    );
  }
}

/** @type {import('next').NextConfig} */
const nextConfig = {
  // REPORT.md A: Build ID固定化でPod間整合性確保
  generateBuildId: async () => process.env.GIT_SHA || process.env.VERCEL_GIT_COMMIT_SHA || 'local-dev',
  
  // Enable standalone output for containerized deployment
  output: "standalone",
  
  // Essential optimizations
  compress: true,
  poweredByHeader: false,

  // CSP violations reporting endpoint
  async rewrites() {
    return [
      {
        source: "/api/csp-report",
        destination: "/api/security/csp-report",
      },
    ];
  },

  // REPORT.md A: Build IDヘッダ出力（ビルド時評価）
  async headers() {
    const buildId = process.env.GIT_SHA || process.env.VERCEL_GIT_COMMIT_SHA || 'local-dev'
    return [
      {
        source: "/(.*)",
        headers: [
          {
            key: "X-Next-Build-Id",
            value: buildId,
          },
        ],
      },
    ];
  },

  // Experimental optimizations
  experimental: {
    optimizePackageImports: ["@chakra-ui/react", "@emotion/react"],
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
      config.optimization.splitChunks = {
        chunks: "all",
        cacheGroups: {
          // Separate vendor chunks
          vendor: {
            test: /[\\/]node_modules[\\/]/,
            name: "vendors",
            chunks: "all",
            priority: 10,
          },
          // Separate Chakra UI into its own chunk
          chakra: {
            test: /[\\/]node_modules[\\/]@chakra-ui[\\/]/,
            name: "chakra",
            chunks: "all",
            priority: 20,
          },
          // Common components
          common: {
            name: "common",
            minChunks: 2,
            chunks: "all",
            priority: 5,
            reuseExistingChunk: true,
          },
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
