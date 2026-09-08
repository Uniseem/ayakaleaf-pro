'use client'

import { Button, Input } from '@heroui/react'
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
      <Input
        label="Email"
        type="email"
        value={email}
        onValueChange={setEmail}
        autoComplete="username"
        isRequired
        autoFocus
        isInvalid={badField === 'email'}
        variant="bordered"
      />
      <Input
        label="Password"
        type="password"
        value={password}
        onValueChange={setPassword}
        autoComplete="current-password"
        isRequired
        isInvalid={badField === 'password'}
        variant="bordered"
      />
      <Button type="submit" color="primary" isLoading={busy} fullWidth>
        {busy ? 'Signing in' : 'Sign in'}
      </Button>
    </form>
  )
}
