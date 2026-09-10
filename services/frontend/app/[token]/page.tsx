import { notFound, redirect } from 'next/navigation'
import { currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { SiteHeader } from '@/components/site-header'
import { JoinByLink } from '@/features/sharing/join-by-link'

export const metadata = { title: 'Shared project' }

/**
 * A sharing link that grants editing.
 *
 * At the root of the site, which is where the original puts it and therefore
 * where every link anybody has already sent points. Every real page is a
 * static segment and wins over this one; what reaches here is either a
 * sharing token or a mistyped URL, and the shape tells them apart -- a
 * read-and-write token is digits and then letters from an alphabet with no
 * vowels in it, so it cannot accidentally spell anything and nothing else
 * looks like one. Without that check every typo would render a page saying a
 * link had expired, which is a worse answer than "there is no such page".
 *
 * The lengths are a range rather than the exact ones the server issues today,
 * so a link made by a version that counted differently still opens.
 */
const READ_AND_WRITE = /^[0-9]{6,12}[bcdfghjkmnpqrstvwxyz]{6,16}$/

export default async function LinkPage({
  params,
}: {
  params: Promise<{ token: string }>
}) {
  const { token } = await params
  if (!READ_AND_WRITE.test(token)) {
    notFound()
  }

  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect(`/login?next=/${encodeURIComponent(token)}`)
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
