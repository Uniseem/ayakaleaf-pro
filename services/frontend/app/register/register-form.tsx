'use client'

import { Button, TextField } from '@/components/ui'
import { useState, type FormEvent } from 'react'
import { fieldFor } from '@/lib/api'
import { register } from '@/lib/auth'
import { FormError } from '@/components/form-error'

/**
 * Signing up.
 *
 * The minimum password length comes from the API rather than being repeated
 * here: it is a setting an administrator changes, and a copy in the form is a
 * copy that goes stale.
 */
export function RegisterForm({ minPasswordLength }: { minPasswordLength: number }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const answer = await register({ email, password })
      window.location.assign(answer.redirect)
    } catch (thrown) {
      setError(thrown)
      setBusy(false)
    }
  }

  const badField = fieldFor(error)
  const tooShort = password.length > 0 && password.length < minPasswordLength

  return (
    <form className="flex flex-col gap-4" onSubmit={submit} noValidate>
      <FormError error={error} />
      <TextField
        label="Email"
        type="email"
        value={email}
        onChange={event => setEmail(event.target.value)}
        autoComplete="username"
        placeholder="email@example.com"
        required
        autoFocus
        error={badField === 'email' ? ' ' : null}
      />
      <TextField
        label="Password"
        type="password"
        value={password}
        onChange={event => setPassword(event.target.value)}
        autoComplete="new-password"
        required
        hint={`At least ${minPasswordLength} characters.`}
        error={tooShort ? `At least ${minPasswordLength} characters.` : null}
      />
      <div>
        <Button type="submit" loading={busy} disabled={tooShort}>
          {busy ? 'Creating your account' : 'Create account'}
        </Button>
      </div>
    </form>
  )
}
