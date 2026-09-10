'use client'

import { Button } from '@/components/ol/button'
import { Notification } from '@/components/ol/notification'
import { useRouter } from 'next/navigation'
import { useState } from 'react'
import { acceptInvite } from '@/lib/sharing'
import { messageFor } from '@/lib/api'

export function AcceptInvite({ token }: { token: string }) {
  const router = useRouter()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  return (
    <div className="card">
      <div className="card-body flex flex-col gap-4 p-6">
        <div>
          <h1 className="text-lg font-semibold">You have been invited</h1>
          <p className="mt-1 text-sm text-[var(--content-secondary)]">
            Accepting adds this project to your list. You can leave it again at
            any point.
          </p>
        </div>

        {error ? <Notification type="error" content={error} /> : null}

        <Button
          variant="primary"
          isLoading={busy}
          onClick={async () => {
            setBusy(true)
            setError(null)
            try {
              const { projectId } = await acceptInvite(token)
              router.replace(`/projects/${projectId}`)
            } catch (thrown) {
              setError(messageFor(thrown))
              setBusy(false)
            }
          }}
        >
          Accept
        </Button>
        <Button variant="secondary" onClick={() => router.replace('/projects')}>
          Not now
        </Button>
      </div>
    </div>
  )
}
