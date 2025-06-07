/** @type {import('next').NextConfig} */
const nextConfig = {
  // Essential optimizations
  compress: true,
  poweredByHeader: false,
  
  experimental: {
    optimizePackageImports: ["@chakra-ui/react", "@emotion/react"],
    // Improve tree shaking
    esmExternals: true,
  },
  
  // Use stable turbopack
  turbopack: {},
  
  // Enhanced webpack optimization for performance
  webpack: (config: any, { isServer, dev }: { isServer: boolean; dev: boolean }) => {
    // Production optimizations only
    if (!isServer && !dev) {
      // Enhanced tree shaking
      config.optimization.usedExports = true;
      config.optimization.sideEffects = false;
      
      // Better chunk splitting for caching
      config.optimization.splitChunks = {
        chunks: 'all',
        cacheGroups: {
          // Separate vendor chunks
          vendor: {
            test: /[\\/]node_modules[\\/]/,
            name: 'vendors',
            chunks: 'all',
            priority: 10,
          },
          // Separate Chakra UI into its own chunk
          chakra: {
            test: /[\\/]node_modules[\\/]@chakra-ui[\\/]/,
            name: 'chakra',
            chunks: 'all',
            priority: 20,
          },
          // Common components
          common: {
            name: 'common',
            minChunks: 2,
            chunks: 'all',
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
    removeConsole: process.env.NODE_ENV === 'production',
  },

  // Performance budgets
  onDemandEntries: {
    // Period (in ms) where the server will keep pages in the buffer
    maxInactiveAge: 25 * 1000,
    // Number of pages that should be kept simultaneously without being disposed
    pagesBufferLength: 2,
  },
}

module.exports = nextConfig
