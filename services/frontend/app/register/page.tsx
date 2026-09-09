import Link from 'next/link'
import { redirect } from 'next/navigation'
import { AuthCard } from '@/components/auth-card'
import { ProviderButtons } from '@/components/provider-buttons'
import { authStatus, currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { RegisterForm } from './register-form'

export const metadata = { title: 'Create an account' }

export default async function RegisterPage() {
  const headers = await forwardedHeaders()
  const [user, status, name] = await Promise.all([
    currentUser(headers).catch(() => null),
    authStatus(headers),
    siteName(headers),
  ])

  if (user) {
    redirect('/projects')
  }

  if (!status.open) {
    return (
      <AuthCard
        title="Sign up is closed"
        siteName={name}
        notice="Ask an administrator for an account."
        footer={
          <Link
            href="/login"
            className="text-[var(--link-web)] underline underline-offset-2 hover:text-[var(--link-web-hover)]"
          >
            Back to log in
          </Link>
        }
      >
        <ProviderButtons providers={status.providers} />
      </AuthCard>
    )
  }

  return (
    <AuthCard
      title={status.firstUser ? 'Set up this site' : 'Create an account'}
      siteName={name}
      notice={
        status.firstUser
          ? 'Nobody has an account here yet. The account you create now becomes the administrator, and you will be signed in and taken to the settings.'
          : status.allowedDomains.length > 0
            ? `Sign up is limited to these email domains: ${status.allowedDomains.join(', ')}`
            : undefined
      }
      footer={
        status.firstUser ? null : (
          <>
            Already have an account?{' '}
            <Link
              href="/login"
              className="text-[var(--link-web)] underline underline-offset-2 hover:text-[var(--link-web-hover)]"
            >
              Log in
            </Link>
          </>
        )
      }
    >
      <RegisterForm minPasswordLength={status.minPasswordLength} />
      <ProviderButtons providers={status.providers} />
    </AuthCard>
  )
}
