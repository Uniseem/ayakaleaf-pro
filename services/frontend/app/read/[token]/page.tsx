import { redirect } from 'next/navigation'
import { currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { SiteHeader } from '@/components/site-header'
import { JoinByLink } from '@/features/sharing/join-by-link'

export const metadata = { title: 'Shared project' }

/**
 * A read-only sharing link.
 *
 * Signing in comes first and the token is carried through it: these links are
 * followed while signed out as often as not, and losing the token at the
 * sign-in page would leave somebody staring at their own empty project list
 * wondering what happened to the thing they were sent.
 */
export default async function ReadLinkPage({
  params,
}: {
  params: Promise<{ token: string }>
}) {
  const { token } = await params
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect(`/login?next=/read/${encodeURIComponent(token)}`)
  }

  const name = await siteName(headers)

  return (
    <>
      <SiteHeader user={user} siteName={name} />
      <main className="mx-auto w-full max-w-md px-4 py-16">
        <JoinByLink token={token} />
      </main>
    </>
  )
}
