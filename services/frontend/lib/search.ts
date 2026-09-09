/** Searching the text of a project's documents. */

import { api } from './api'

export type SearchMatch = {
  /** Zero-based, matching how the editor counts lines internally. */
  line: number
  /** The whole line the match is on, for showing context. */
  text: string
  column: number
}

export type SearchHit = {
  id: string
  path: string
  matches: SearchMatch[]
}

export type SearchOptions = {
  query: string
  caseSensitive?: boolean
  wholeWord?: boolean
}

export function searchProject(
  projectId: string,
  options: SearchOptions,
  signal?: AbortSignal
): Promise<SearchHit[]> {
  const query = new URLSearchParams({ q: options.query })
  if (options.caseSensitive) {
    query.set('case', '1')
  }
  if (options.wholeWord) {
    query.set('word', '1')
  }
  return api<{ hits: SearchHit[] }>(
    `/api/projects/${projectId}/search?${query}`,
    { signal }
  ).then(answer => answer.hits)
}
