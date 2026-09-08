import { Link } from '@heroui/react'
import { redirect } from 'next/navigation'
import { AuthCard } from '@/components/auth-card'
import { ProviderButtons } from '@/components/provider-buttons'
import { authStatus, currentUser } from '@/lib/auth'
import { localPath } from '@/lib/paths'
import { forwardedHeaders } from '@/lib/server'
import { LoginForm } from './login-form'

export const metadata = { title: 'Sign in' }

export default async function LoginPage({
  searchParams,
}: {
  searchParams: { error?: string; next?: string }
}) {
  const headers = forwardedHeaders()
  const [user, status] = await Promise.all([
    currentUser(headers).catch(() => null),
    authStatus(headers),
  ])

  if (user) {
    redirect(localPath(searchParams.next, '/projects'))
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
      {/* Written by the API, never by a provider, so a link cannot put words
          on this page. */}
      {searchParams.error ? (
        <div
          role="alert"
          className="rounded-medium border border-danger-200 bg-danger-50 px-4 py-3 text-small text-danger-700 dark:bg-danger-50/10"
        >
          {searchParams.error}
        </div>
      ) : null}
      <LoginForm next={searchParams.next} />
      <ProviderButtons providers={status.providers} />
    </AuthCard>
  )
}
