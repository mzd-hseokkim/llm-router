import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: "/api/admin/:path*",
        destination: `${process.env.GATEWAY_URL || "http://localhost:8080"}/admin/:path*`,
      },
    ];
  },
};

export default nextConfig;
