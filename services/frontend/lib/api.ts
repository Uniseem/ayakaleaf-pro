/**
 * The one way this client talks to the API.
 *
 * Every call goes through here, so the things that must be true of all of them
 * are true in one place: same origin, cookies included, one error shape, and a
 * body that is always JSON. A component that fetches on its own would be the
 * beginning of four different error handlings again.
 */

/** The error shape the API answers failures with. */
export type ApiErrorBody = {
  code: string
  message: string
  field?: string
}

/** ApiError is what every failed call throws. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly field?: string

  constructor(status: number, body: ApiErrorBody) {
    super(body.message)
    this.name = 'ApiError'
    this.status = status
    this.code = body.code
    this.field = body.field
  }

  /** Whether this is worth showing to the person, or is ours to fix. */
  get isOurFault(): boolean {
    return this.status >= 500
  }
}

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  signal?: AbortSignal
  /** Sent through on the server, where there is no browser to carry cookies. */
  headers?: Record<string, string>
}

/**
 * Calls the API.
 *
 * Relative URLs on the client, so the browser sends the session cookie and the
 * request is same-origin. On the server the caller passes the incoming
 * cookies through, because a server render has no cookie jar of its own.
 */
export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, signal, headers = {} } = options

  const base = typeof window === 'undefined'
    ? process.env.API_INTERNAL_URL || 'http://127.0.0.1:3400'
    : ''

  const response = await fetch(`${base}${path}`, {
    method,
    signal,
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      ...headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
  })

  if (response.status === 204) {
    return undefined as T
  }

  const text = await response.text()
  let parsed: unknown = undefined
  if (text) {
    try {
      parsed = JSON.parse(text)
    } catch {
      // A body that is not JSON from an API that only speaks JSON means
      // something else answered -- a proxy, or a crash page.
      throw new ApiError(response.status, {
        code: 'unreadable_response',
        message: 'The server sent something this page could not read.',
      })
    }
  }

  if (!response.ok) {
    const wrapper = parsed as { error?: ApiErrorBody } | undefined
    throw new ApiError(
      response.status,
      wrapper?.error ?? {
        code: 'unknown',
        message: 'Something went wrong.',
      }
    )
  }

  return parsed as T
}

/** Reads a message worth showing, whatever was thrown. */
export function messageFor(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message
  }
  if (error instanceof Error && error.name === 'AbortError') {
    return ''
  }
  return 'Something went wrong.'
}

/** The field an error was about, when it named one. */
export function fieldFor(error: unknown): string | undefined {
  return error instanceof ApiError ? error.field : undefined
}
