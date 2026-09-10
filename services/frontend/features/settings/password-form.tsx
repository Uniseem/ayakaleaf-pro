'use client'

/**
 * Changing a password.
 *
 * The current one is required. Holding the session is not enough: a session
 * left open on a shared machine would otherwise be enough to lock its owner
 * out of their own account.
 */

import { useState } from 'react'
import { Card, CardBody, CardHeader } from '@/components/ol/card'
import { Button } from '@/components/ol/button'
import { OLFormControl, OLFormFeedback, OLFormGroup, OLFormLabel } from '@/components/ol/form-control'
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
      <CardHeader title="Password" subtitle="Changing it does not sign you out anywhere else." />
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
          <OLFormGroup controlId="current-password">
            <OLFormLabel>Current password</OLFormLabel>
            <OLFormControl
              type="password"
              autoComplete="current-password"
              value={current}
              onChange={event => setCurrent(event.target.value)}
              isInvalid={field === 'currentPassword'}
            />
            {field === 'currentPassword' && error ? <OLFormFeedback type="invalid">{error}</OLFormFeedback> : null}
          </OLFormGroup>

          <OLFormGroup controlId="new-password">
            <OLFormLabel>New password</OLFormLabel>
            <OLFormControl
              type="password"
              autoComplete="new-password"
              value={next}
              onChange={event => setNext(event.target.value)}
              isInvalid={field === 'newPassword'}
            />
            {field === 'newPassword' && error ? <OLFormFeedback type="invalid">{error}</OLFormFeedback> : null}
          </OLFormGroup>

          <OLFormGroup controlId="new-password-again">
            <OLFormLabel>New password again</OLFormLabel>
            <OLFormControl
              type="password"
              autoComplete="new-password"
              value={again}
              onChange={event => setAgain(event.target.value)}
              isInvalid={mismatch}
            />
            {mismatch ? <OLFormFeedback type="invalid">These two do not match.</OLFormFeedback> : null}
          </OLFormGroup>

          {error && !field ? <p className="text-sm text-[var(--content-danger)]">{error}</p> : null}
          {done ? (
            <p className="text-sm text-[var(--content-positive)]">Your password has been changed.</p>
          ) : null}

          <Button
            type="submit"
            variant="primary"
            className="self-start"
            isLoading={busy}
            disabled={!current || !next || mismatch}
          >
            Change password
          </Button>
        </form>
      </CardBody>
    </Card>
  )
}
