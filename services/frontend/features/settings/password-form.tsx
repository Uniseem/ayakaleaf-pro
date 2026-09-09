'use client'

/**
 * Changing a password.
 *
 * The current one is required. Holding the session is not enough: a session
 * left open on a shared machine would otherwise be enough to lock its owner
 * out of their own account.
 */

import { Button, Card, CardBody, CardHeader, Input } from '@heroui/react'
import { useState } from 'react'
import { changePassword } from '@/lib/user-settings'
import { fieldFor, messageFor } from '@/lib/api'

export function PasswordForm() {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [again, setAgain] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [field, setField] = useState<string | undefined>(undefined)
  const [done, setDone] = useState(false)

  const mismatch = again !== '' && next !== again

  return (
    <Card>
      <CardHeader className="flex-col items-start gap-0.5">
        <h2 className="text-base font-semibold">Password</h2>
        <p className="text-xs text-default-500">
          Changing it does not sign you out anywhere else.
        </p>
      </CardHeader>
      <CardBody>
        <form
          className="flex max-w-sm flex-col gap-3"
          onSubmit={async event => {
            event.preventDefault()
            if (busy || mismatch || !current || !next) {
              return
            }
            setBusy(true)
            setError(null)
            setField(undefined)
            setDone(false)
            try {
              await changePassword({ currentPassword: current, newPassword: next })
              setCurrent('')
              setNext('')
              setAgain('')
              setDone(true)
            } catch (thrown) {
              setError(messageFor(thrown))
              setField(fieldFor(thrown))
            } finally {
              setBusy(false)
            }
          }}
        >
          <Input
            type="password"
            label="Current password"
            autoComplete="current-password"
            value={current}
            onValueChange={setCurrent}
            isInvalid={field === 'currentPassword'}
            errorMessage={field === 'currentPassword' ? error : null}
          />
          <Input
            type="password"
            label="New password"
            autoComplete="new-password"
            value={next}
            onValueChange={setNext}
            isInvalid={field === 'newPassword'}
            errorMessage={field === 'newPassword' ? error : null}
          />
          <Input
            type="password"
            label="New password again"
            autoComplete="new-password"
            value={again}
            onValueChange={setAgain}
            isInvalid={mismatch}
            errorMessage={mismatch ? 'These two do not match.' : null}
          />

          {error && !field ? (
            <p className="text-sm text-danger">{error}</p>
          ) : null}
          {done ? (
            <p className="text-sm text-success">Your password has been changed.</p>
          ) : null}

          <Button
            type="submit"
            color="primary"
            className="self-start"
            isLoading={busy}
            isDisabled={!current || !next || mismatch}
          >
            Change password
          </Button>
        </form>
      </CardBody>
    </Card>
  )
}
