/**
 * Liveness for this container.
 *
 * It answers without asking the API anything, on purpose: this says whether
 * the process that renders pages is running, and an API that is briefly down
 * is the API's own health check to fail, not this one's.
 */
export const dynamic = 'force-dynamic'

export function GET() {
  return new Response('ok\n', {
    headers: { 'content-type': 'text/plain', 'cache-control': 'no-store' },
  })
}
