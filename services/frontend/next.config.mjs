/**
 * The client is a separate process from the API, but it must look like one
 * site to a browser: the session cookie is host-scoped, and the API refuses a
 * state-changing request that came from another origin. So both sit behind one
 * address and are told apart by prefix.
 *
 *   /api/*   the Go API
 *   /*       a page, rendered here
 *
 * There is no fallback to the old service. This client is the whole client; a
 * path it does not answer is a path that does not exist yet.
 *
 * @type {import('next').NextConfig}
 */
const nextConfig = {
  // A self-contained server tree, so the image copies one directory rather
  // than a node_modules the size of the monorepo.
  output: 'standalone',
  // Where that tree is rooted. Without this it is wherever the nearest
  // package.json above happens to be, which is this directory when the image
  // is built and the whole repository when it is not -- and the server ends up
  // at a different path in each.
  outputFileTracingRoot: import.meta.dirname,
  reactStrictMode: true,
  poweredByHeader: false,

  async rewrites() {
    const api = process.env.API_INTERNAL_URL || 'http://127.0.0.1:3400'
    return [
      { source: '/api/:path*', destination: `${api}/api/:path*` },
    ]
  },

  async headers() {
    return [
      {
        source: '/:path*',
        headers: [
          { key: 'X-Content-Type-Options', value: 'nosniff' },
          { key: 'Referrer-Policy', value: 'same-origin' },
          { key: 'X-Frame-Options', value: 'SAMEORIGIN' },
        ],
      },
    ]
  },
}

export default nextConfig
