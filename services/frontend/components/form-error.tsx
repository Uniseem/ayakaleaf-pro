'use client'

import { messageFor } from '@/lib/api'

/**
 * What went wrong, in the words the API chose.
 *
 * The client does not rewrite these messages. The API already decided what a
 * person should be told, and a second opinion here is how the same failure
 * ends up phrased two ways.
 */
export function FormError({ error }: { error: unknown }) {
  if (!error) {
    return null
  }
  const message = messageFor(error)
  if (!message) {
    return null
  }
  return (
    <div
      role="alert"
      aria-live="polite"
      className="rounded-medium border border-danger-200 bg-danger-50 px-4 py-3 text-small text-danger-700 dark:border-danger-100 dark:bg-danger-50/10 dark:text-danger-400"
    >
      {message}
    </div>
  )
}
