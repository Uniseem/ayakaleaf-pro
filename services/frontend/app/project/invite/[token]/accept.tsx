'use client'

import { Button, Card, CardBody } from '@heroui/react'
import { useRouter } from 'next/navigation'
import { useState } from 'react'
import { acceptInvite } from '@/lib/sharing'
import { messageFor } from '@/lib/api'

export function AcceptInvite({ token }: { token: string }) {
  const router = useRouter()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  return (
    <Card>
      <CardBody className="gap-4 p-6">
        <div>
          <h1 className="text-lg font-semibold">You have been invited</h1>
          <p className="mt-1 text-sm text-default-500">
            Accepting adds this project to your list. You can leave it again at
            any point.
          </p>
        </div>

        {error ? (
          <p className="rounded bg-danger-50 px-3 py-2 text-sm text-danger">
            {error}
          </p>
        ) : null}

        <Button
          color="primary"
          isLoading={busy}
          onPress={async () => {
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
        <Button variant="light" onPress={() => router.replace('/projects')}>
          Not now
        </Button>
      </CardBody>
    </Card>
  )
}
