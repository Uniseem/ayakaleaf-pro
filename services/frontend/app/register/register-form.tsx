'use client'

import { Button, Input } from '@heroui/react'
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
        autoComplete="new-password"
        isRequired
        isInvalid={badField === 'password' || tooShort}
        description={`At least ${minPasswordLength} characters.`}
        errorMessage={tooShort ? `At least ${minPasswordLength} characters.` : undefined}
        variant="bordered"
      />
      <Button
        type="submit"
        color="primary"
        isLoading={busy}
        isDisabled={tooShort}
        fullWidth
      >
        {busy ? 'Creating your account' : 'Create account'}
      </Button>
    </form>
  )
}
