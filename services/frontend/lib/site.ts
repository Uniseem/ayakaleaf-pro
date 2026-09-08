import { api } from './api'

/**
 * The name this site calls itself.
 *
 * Read from the API rather than baked in, because an administrator sets it and
 * a build-time constant would mean rebuilding the client to rename the site.
 * Anything that goes wrong falls back to the product's own name, which is
 * better than a page with a hole in it.
 */
export async function siteName(headers?: Record<string, string>): Promise<string> {
  try {
    const answer = await api<{ name: string }>('/api/site', { headers })
    return answer.name || 'Ayakaleaf Pro'
  } catch {
    return 'Ayakaleaf Pro'
  }
}
