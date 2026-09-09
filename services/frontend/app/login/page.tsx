import Link from 'next/link'
import { redirect } from 'next/navigation'
import { AuthCard } from '@/components/auth-card'
import { ProviderButtons } from '@/components/provider-buttons'
import { authStatus, currentUser } from '@/lib/auth'
import { localPath } from '@/lib/paths'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { LoginForm } from './login-form'

export const metadata = { title: 'Sign in' }

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string; next?: string }>
}) {
  const query = await searchParams
  const headers = await forwardedHeaders()
  const [user, status, name] = await Promise.all([
    currentUser(headers).catch(() => null),
    authStatus(headers),
    siteName(headers),
  ])

  if (user) {
    redirect(localPath(query.next, '/projects'))
  }
  // A site nobody has claimed has nothing to sign in to. Sending the first
  // visitor to the sign-up form is what makes a fresh install claimable by
  // reaching it, rather than by finding a token in a log.
  if (status.firstUser) {
    redirect('/register')
  }

  return (
    <AuthCard
      title="Log in"
      siteName={name}
      footer={
        status.open ? (
          <>
            No account yet?{' '}
            <Link
              href="/register"
              className="text-[var(--link-web)] underline underline-offset-2 hover:text-[var(--link-web-hover)]"
            >
              Create one
            </Link>
          </>
        ) : null
      }
    >
      {/* Written by the API, never by a provider, so a link cannot put words
          on this page. */}
      {query.error ? (
        <div
          role="alert"
          className="mb-4 rounded-[4px] bg-[var(--bg-danger-03)] px-4 py-3 text-[14px] leading-5 text-[var(--content-danger)]"
        >
          {query.error}
        </div>
      ) : null}
      <LoginForm next={query.next} />
      <ProviderButtons providers={status.providers} />
    </AuthCard>
  )
}
