'use client'

import { Button, Card, CardBody, CardHeader, Chip } from '@heroui/react'
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
    <Card shadow="sm">
      <CardHeader className="flex flex-col items-start gap-1 px-6 pt-6">
        <h2 className="text-lg font-medium">Linked accounts</h2>
        <p className="text-small text-default-500">
          Sign in with any of these. They all reach this one account.
        </p>
      </CardHeader>
      <CardBody className="gap-4 px-6 pb-6">
        <FormError error={error} />
        {state.map(provider => (
          <div
            key={provider.id}
            className="flex items-center justify-between gap-4 border-b border-divider pb-4 last:border-0 last:pb-0"
          >
            <div className="flex items-center gap-3">
              <span className="font-medium">{provider.name}</span>
              {provider.linked ? (
                <Chip size="sm" color="success" variant="flat">
                  linked
                </Chip>
              ) : null}
            </div>
            {provider.linked ? (
              <Button
                size="sm"
                variant="bordered"
                color="danger"
                isLoading={busy === provider.id}
                onPress={() => unlink(provider.id)}
              >
                Unlink
              </Button>
            ) : (
              <Button as="a" href={provider.path} size="sm" variant="bordered">
                Link
              </Button>
            )}
          </div>
        ))}
      </CardBody>
    </Card>
  )
}
