'use client'

/**
 * The page at the end of a sharing link.
 *
 * Somebody has been sent a URL and followed it. What they want is the
 * project, so this asks for it and goes there -- no button, because there is
 * nothing to decide: they already decided by clicking the link, and a page
 * that makes them click a second time to get what they asked for is a page
 * that is in the way.
 *
 * It is still a POST. Following the link changes who can see the project, and
 * a GET that does that is one a mail scanner or a link preview can fire on
 * somebody's behalf before they have read the message.
 */

import { useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { Notification } from '@/components/ol/notification'
import { Button } from '@/components/ol/button'
import { messageFor } from '@/lib/api'
import { redeemLink } from '@/lib/sharing'

export function JoinByLink({ token }: { token: string }) {
  const router = useRouter()
  const [error, setError] = useState<string | null>(null)
  // React runs effects twice in development, and this one is a write.
  const asked = useRef(false)

  useEffect(() => {
    if (asked.current) {
      return
    }
    asked.current = true
    void (async () => {
      try {
        const { projectId } = await redeemLink(token)
        router.replace(`/projects/${projectId}`)
      } catch (thrown) {
        setError(messageFor(thrown))
      }
    })()
  }, [token, router])

  if (error) {
    return (
      <div className="card">
        <div className="card-body flex flex-col gap-4 p-6">
          <div>
            <h1 className="text-lg font-semibold">This link did not work</h1>
            <p className="mt-1 text-sm text-[var(--content-secondary)]">
              A sharing link stops working when the owner turns sharing off or
              makes a new one. Ask them for the current link.
            </p>
          </div>
          <Notification type="error" content={error} />
          <Button variant="secondary" onClick={() => router.replace('/projects')}>
            Go to your projects
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="card">
      <div className="card-body flex items-center gap-3 p-6">
        <span
          aria-hidden
          className="h-4 w-4 animate-spin rounded-full border-2 border-[var(--border-divider)] border-t-[var(--bg-accent-01)]"
        />
        <p className="text-sm text-[var(--content-secondary)]">
          Opening the project…
        </p>
      </div>
    </div>
  )
}
