'use client'

import { Card, CardBody, CardHeader } from '@/components/ol/card'
import { Button } from '@/components/ol/button'
import { Badge } from '@/components/ol/badge'
import { useState } from 'react'
import { FormError } from '@/components/form-error'
import { unlinkProvider, type LinkedProvider } from '@/lib/account'

/**
 * The identity providers this account can sign in with.
 *
 * Detaching the only way somebody can get in is refused by the API, and the
 * message it gives back is shown as it wrote it: the client does not know
 * whether a password is set, and guessing here would mean two answers to the
 * same question.
 */
export function LinkedAccounts({ providers }: { providers: LinkedProvider[] }) {
  const [state, setState] = useState(providers)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState<string | null>(null)

  if (state.length === 0) {
    return null
  }

  async function unlink(id: string) {
    setError(null)
    setBusy(id)
    try {
      await unlinkProvider(id)
      setState(previous =>
        previous.map(provider =>
          provider.id === id ? { ...provider, linked: false } : provider
        )
      )
    } catch (thrown) {
      setError(thrown)
    } finally {
      setBusy(null)
    }
  }

  return (
    <Card>
      <CardHeader
        title="Linked accounts"
        subtitle="Sign in with any of these. They all reach this one account."
      />
      <CardBody className="flex flex-col gap-4">
        <FormError error={error} />
        {state.map(provider => (
          <div
            key={provider.id}
            className="flex items-center justify-between gap-4 border-b border-[var(--border-divider)] pb-4 last:border-0 last:pb-0"
          >
            <div className="flex items-center gap-3">
              <span className="font-medium">{provider.name}</span>
              {provider.linked ? <Badge bg="success">linked</Badge> : null}
            </div>
            {provider.linked ? (
              <Button
                size="sm"
                variant="danger-ghost"
                isLoading={busy === provider.id}
                onClick={() => unlink(provider.id)}
              >
                Unlink
              </Button>
            ) : (
              <Button href={provider.path} size="sm" variant="secondary">
                Link
              </Button>
            )}
          </div>
        ))}
      </CardBody>
    </Card>
  )
}
