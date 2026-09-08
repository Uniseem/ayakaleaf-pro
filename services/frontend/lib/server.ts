import { cookies } from 'next/headers'

/**
 * The headers a server render must pass through to the API.
 *
 * A server component has no cookie jar: the browser's cookies arrived on the
 * incoming request and have to be forwarded by hand, or the API sees an
 * anonymous visitor and every page renders signed out.
 */
export async function forwardedHeaders(): Promise<Record<string, string>> {
  const jar = await cookies()
  const cookie = jar
    .getAll()
    .map(({ name, value }) => `${name}=${value}`)
    .join('; ')
  return cookie ? { cookie } : {}
}
