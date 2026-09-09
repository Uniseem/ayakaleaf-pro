import Link from 'next/link'
import { AuthCard } from '@/components/auth-card'
import { authStatus } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { SetPasswordForm } from './form'

export const metadata = { title: 'Choose a new password' }

/**
 * The page an emailed reset link lands on.
 *
 * The token stays in the query string and is only sent when the form is
 * submitted. Nothing is checked here: a page that told you whether a token was
 * valid before you used it would be a way to test tokens.
 */
export default async function SetPasswordPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>
}) {
  const { token } = await searchParams
  const headers = await forwardedHeaders()
  const [status, name] = await Promise.all([
    authStatus(headers),
    siteName(headers),
  ])

  return (
    <AuthCard
      title="Choose a new password"
      siteName={name}
      notice={
        token
          ? undefined
          : 'That link is missing its token. Ask for another reset email.'
      }
      footer={
        <Link
          href="/login"
          className="text-[var(--link-web)] underline underline-offset-2 hover:text-[var(--link-web-hover)]"
        >
          Back to log in
        </Link>
      }
    >
      {token ? (
        <SetPasswordForm
          token={token}
          minPasswordLength={status.minPasswordLength}
        />
      ) : null}
    </AuthCard>
  )
}
