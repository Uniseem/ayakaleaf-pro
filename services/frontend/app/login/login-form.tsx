'use client'

import Link from 'next/link'
import { Button, TextField } from '@/components/ui'
import { useState, type FormEvent } from 'react'
import { fieldFor } from '@/lib/api'
import { login } from '@/lib/auth'
import { localPath } from '@/lib/paths'
import { FormError } from '@/components/form-error'

/**
 * Signing in.
 *
 * One message for a wrong password and for an address nobody holds, because
 * the API answers with one: telling them apart is how an account list is
 * enumerated.
 */
export function LoginForm({ next }: { next?: string }) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setError(null)
    setBusy(true)
    try {
      const answer = await login({ email, password })
      // A full navigation rather than a push: signing in changed who the
      // server thinks we are, and every cached render is now about somebody
      // else.
      window.location.assign(localPath(next, answer.redirect))
    } catch (thrown) {
      setError(thrown)
      setBusy(false)
    }
  }

  const badField = fieldFor(error)

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
        autoComplete="current-password"
        required
        error={badField === 'password' ? ' ' : null}
      />
      {/* The button and the link share a row, which is where the original
          puts them: the form is short enough that a full-width button below
          a full-width field reads as one more field. */}
      <div className="flex items-center gap-4">
        <Button type="submit" loading={busy}>
          {busy ? 'Logging in' : 'Login'}
        </Button>
        <Link
          href="/password/reset"
          className="text-[14px] leading-5 text-[var(--link-web)] underline underline-offset-2 hover:text-[var(--link-web-hover)]"
        >
          Forgot your password?
        </Link>
      </div>
    </form>
  )
}
