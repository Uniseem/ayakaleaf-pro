'use client'

import { useState, type FormEvent } from 'react'
import { messageFor } from '@/lib/api'
import { requestPasswordReset } from '@/lib/auth'
import { Button, Notification, TextField } from '@/components/ui'

/**
 * Asking for a reset link.
 *
 * The answer is the same whether or not the address has an account, so this
 * shows the same message either way rather than confirming that somebody is a
 * user here.
 */
export function RequestResetForm() {
  const [email, setEmail] = useState('')
  const [sent, setSent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const answer = await requestPasswordReset(email)
      setSent(answer.message)
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setBusy(false)
    }
  }

  if (sent) {
    return <Notification kind="success">{sent}</Notification>
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={submit} noValidate>
      {error ? <Notification kind="danger">{error}</Notification> : null}
      <p className="text-[14px] leading-5 text-[var(--content-secondary)]">
        Enter the address you signed up with and we will send you a link.
      </p>
      <TextField
        label="Email"
        type="email"
        value={email}
        onChange={event => setEmail(event.target.value)}
        placeholder="email@example.com"
        autoComplete="username"
        required
        autoFocus
      />
      <div>
        <Button type="submit" loading={busy} disabled={!email.trim()}>
          Send the link
        </Button>
      </div>
    </form>
  )
}
