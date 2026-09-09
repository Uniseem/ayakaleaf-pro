'use client'

/**
 * The other places this account is signed in.
 *
 * Worth showing because it is the only way somebody notices a session they did
 * not start. Each row carries where it came from and when it began, which is
 * what makes one recognisable; ending them all is one button, and it keeps the
 * session doing the asking.
 */

import { useCallback, useEffect, useState } from 'react'
import { clearSessions, listSessions, type Session } from '@/lib/user-settings'
import { messageFor } from '@/lib/api'
import { Button, Notification } from '@/components/ui'

export function Sessions() {
  const [sessions, setSessions] = useState<Session[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setError(null)
    try {
      setSessions(await listSessions())
    } catch (thrown) {
      setError(messageFor(thrown))
      setSessions([])
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const others = (sessions ?? []).filter(each => !each.isCurrent)

  return (
    <section className="rounded-[8px] border border-[var(--border-divider)] p-6">
      <h2 className="text-[18px] font-bold leading-7">Sessions</h2>
      <p className="mt-1 text-[14px] leading-5 text-[var(--content-secondary)]">
        Where this account is signed in. If you do not recognise one, end them
        all and change your password.
      </p>

      {error ? (
        <div className="mt-4">
          <Notification kind="danger">{error}</Notification>
        </div>
      ) : null}

      {sessions === null ? (
        <p className="mt-4 text-[14px] text-[var(--content-secondary)]">Loading…</p>
      ) : sessions.length === 0 ? (
        <p className="mt-4 text-[14px] text-[var(--content-secondary)]">
          No other sessions.
        </p>
      ) : (
        <ul className="mt-4 divide-y divide-[var(--border-divider)] rounded-[4px] border border-[var(--border-divider)]">
          {sessions.map((each, index) => (
            <li
              key={`${each.ipAddress}-${each.sessionCreatedAt}-${index}`}
              className="flex items-center gap-3 px-3 py-2.5 text-[14px] leading-5"
            >
              <span className="font-mono">{each.ipAddress || 'unknown'}</span>
              <span className="text-[var(--content-secondary)]">
                {each.sessionCreatedAt
                  ? new Date(each.sessionCreatedAt).toLocaleString()
                  : ''}
              </span>
              {each.isCurrent ? (
                <span className="ml-auto rounded-full bg-[var(--bg-accent-03)] px-2 py-0.5 text-[12px] leading-4 text-[var(--link-web)]">
                  This one
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      )}

      {others.length > 0 ? (
        <Button
          kind="secondary"
          className="mt-4"
          loading={busy}
          onClick={async () => {
            setBusy(true)
            setError(null)
            try {
              await clearSessions()
              await load()
            } catch (thrown) {
              setError(messageFor(thrown))
            } finally {
              setBusy(false)
            }
          }}
        >
          End the other {others.length === 1 ? 'session' : `${others.length} sessions`}
        </Button>
      ) : null}
    </section>
  )
}
