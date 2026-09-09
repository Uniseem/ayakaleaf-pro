import { redirect } from 'next/navigation'
import { currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { SiteHeader } from '@/components/site-header'
import { AcceptInvite } from './accept'

export const metadata = { title: 'Project invitation' }

/**
 * Taking up an invitation.
 *
 * The link is followed while signed out as often as not, so signing in comes
 * first and the token is carried through it. Accepting is a POST from the
 * browser rather than something this page does while rendering: a GET that
 * changes what somebody has access to is a GET that a link preview can
 * trigger.
 */
export default async function InvitePage({
  params,
}: {
  params: Promise<{ token: string }>
}) {
  const { token } = await params
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect(`/login?next=/project/invite/${encodeURIComponent(token)}`)
  }

  const name = await siteName(headers)

  return (
    <>
      <SiteHeader user={user} siteName={name} />
      <main className="mx-auto w-full max-w-md px-4 py-16">
        <AcceptInvite token={token} />
      </main>
    </>
  )
}
