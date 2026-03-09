import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // output: "export", // Uncomment before `npm run build`. Commented out during dev so dynamic [code] routes render without generateStaticParams().
  trailingSlash: true,
  images: {
    unoptimized: true, // Required for static export
  },
};

export default nextConfig;
