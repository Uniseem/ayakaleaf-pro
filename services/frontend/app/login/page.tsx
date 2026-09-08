import { Link } from '@heroui/react'
import { redirect } from 'next/navigation'
import { AuthCard } from '@/components/auth-card'
import { ProviderButtons } from '@/components/provider-buttons'
import { authStatus, currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { LoginForm } from './login-form'

export const metadata = { title: 'Sign in' }

export default async function LoginPage() {
  const headers = forwardedHeaders()
  const [user, status] = await Promise.all([
    currentUser(headers).catch(() => null),
    authStatus(headers),
  ])

  if (user) {
    redirect('/projects')
  }
  // A site nobody has claimed has nothing to sign in to. Sending the first
  // visitor to the sign-up form is what makes a fresh install claimable by
  // reaching it, rather than by finding a token in a log.
  if (status.firstUser) {
    redirect('/register')
  }

  return (
    <AuthCard
      title="Sign in"
      footer={
        status.open ? (
          <>
            No account yet? <Link href="/register" size="sm">Create one</Link>
          </>
        ) : null
      }
    >
      <LoginForm />
      <ProviderButtons providers={status.providers} />
    </AuthCard>
  )
}
