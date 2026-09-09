import { redirect } from 'next/navigation'
import Link from 'next/link'
import { AuthCard } from '@/components/auth-card'
import { currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { RequestResetForm } from './form'

export const metadata = { title: 'Reset your password' }

export default async function ResetPage() {
  const headers = await forwardedHeaders()
  const [user, name] = await Promise.all([
    currentUser(headers).catch(() => null),
    siteName(headers),
  ])

  // Somebody already signed in does not need this; they have the settings
  // page, which asks for the old password rather than sending mail.
  if (user) {
    redirect('/account')
  }

  return (
    <AuthCard
      title="Reset your password"
      siteName={name}
      footer={
        <Link
          href="/login"
          className="text-[var(--link-web)] underline underline-offset-2 hover:text-[var(--link-web-hover)]"
        >
          Back to log in
        </Link>
      }
    >
      <RequestResetForm />
    </AuthCard>
  )
}
