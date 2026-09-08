import { Link } from '@heroui/react'
import { redirect } from 'next/navigation'
import { AuthCard } from '@/components/auth-card'
import { ProviderButtons } from '@/components/provider-buttons'
import { authStatus, currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { RegisterForm } from './register-form'

export const metadata = { title: 'Create an account' }

export default async function RegisterPage() {
  const headers = await forwardedHeaders()
  const [user, status] = await Promise.all([
    currentUser(headers).catch(() => null),
    authStatus(headers),
  ])

  if (user) {
    redirect('/projects')
  }

  if (!status.open) {
    return (
      <AuthCard
        title="Sign up is closed"
        subtitle="Ask an administrator for an account."
        footer={<Link href="/login" size="sm">Back to sign in</Link>}
      >
        <ProviderButtons providers={status.providers} />
      </AuthCard>
    )
  }

  return (
    <AuthCard
      title={status.firstUser ? 'Set up this site' : 'Create an account'}
      subtitle={
        status.firstUser
          ? 'Nobody has an account here yet. The account you create now becomes the administrator.'
          : status.allowedDomains.length > 0
            ? `Sign up is limited to these email domains: ${status.allowedDomains.join(', ')}`
            : undefined
      }
      footer={
        status.firstUser ? null : (
          <>
            Already have an account? <Link href="/login" size="sm">Sign in</Link>
          </>
        )
      }
    >
      <RegisterForm minPasswordLength={status.minPasswordLength} />
      <ProviderButtons providers={status.providers} />
    </AuthCard>
  )
}
