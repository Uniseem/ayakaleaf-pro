'use client'

import { useRouter } from 'next/navigation'
import { useState, type FormEvent } from 'react'
import { fieldFor, messageFor } from '@/lib/api'
import { setPasswordFromToken } from '@/lib/auth'
import { Button, Notification, TextField } from '@/components/ui'

export function SetPasswordForm({
  token,
  minPasswordLength,
}: {
  token: string
  minPasswordLength: number
}) {
  const router = useRouter()
  const [password, setPassword] = useState('')
  const [again, setAgain] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [field, setField] = useState<string | undefined>(undefined)
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)

  const tooShort = password !== '' && password.length < minPasswordLength
  const mismatch = again !== '' && password !== again

  async function submit(event: FormEvent) {
    event.preventDefault()
    setBusy(true)
    setError(null)
    setField(undefined)
    try {
      await setPasswordFromToken({ token, password })
      setDone(true)
      // Every session was ended by the reset, including any this browser had,
      // so the way on is to sign in with the new password.
      setTimeout(() => router.replace('/login'), 1500)
    } catch (thrown) {
      setError(messageFor(thrown))
      setField(fieldFor(thrown))
      setBusy(false)
    }
  }

  if (done) {
    return (
      <Notification kind="success">
        Your password has been changed, and every other session has been signed
        out. Taking you to the log in page.
      </Notification>
    )
  }

  return (
    <form className="flex flex-col gap-4" onSubmit={submit} noValidate>
      {error && !field ? <Notification kind="danger">{error}</Notification> : null}
      <TextField
        label="New password"
        type="password"
        value={password}
        onChange={event => setPassword(event.target.value)}
        autoComplete="new-password"
        required
        autoFocus
        hint={`At least ${minPasswordLength} characters.`}
        error={
          tooShort
            ? `At least ${minPasswordLength} characters.`
            : field === 'password'
              ? error
              : null
        }
      />
      <TextField
        label="New password again"
        type="password"
        value={again}
        onChange={event => setAgain(event.target.value)}
        autoComplete="new-password"
        required
        error={mismatch ? 'These two do not match.' : null}
      />
      <div>
        <Button
          type="submit"
          loading={busy}
          disabled={!password || tooShort || mismatch}
        >
          Change password
        </Button>
      </div>
    </form>
  )
}
