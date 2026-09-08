import { api } from './api'

/** What every page needs to know about the site itself. */
export type Site = {
  name: string
  url?: string
  git?: { enabled: boolean }
}

export async function site(headers?: Record<string, string>): Promise<Site> {
  try {
    return await api<Site>('/api/site', { headers })
  } catch {
    return { name: 'Ayakaleaf Pro' }
  }
}

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
