import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  turbopack: {
    root: process.cwd(),
  },
  async rewrites() {
    return {
      beforeFiles: [],
      afterFiles: [
        {
          source: "/api/:path*",
          destination: "http://localhost:8080/:path*",
        },
      ],
      fallback: [],
    };
  },
};

export default nextConfig;
