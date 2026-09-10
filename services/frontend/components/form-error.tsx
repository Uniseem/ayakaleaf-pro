'use client'

import { messageFor } from '@/lib/api'
import { Notification } from '@/components/ol/notification'

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
  return <Notification type="error" content={message} />
}
