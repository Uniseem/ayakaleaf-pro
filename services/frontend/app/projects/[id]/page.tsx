import { notFound, redirect } from 'next/navigation'
import { currentUser } from '@/lib/auth'
import { getProject } from '@/lib/editor'
import { ApiError } from '@/lib/api'
import { forwardedHeaders } from '@/lib/server'
import { site as fetchSite } from '@/lib/site'
import { IdePageClient } from '@/features/ide/components/ide-page-client'

export const metadata = { title: 'Editor' }

/**
 * The editor.
 *
 * The project is fetched here rather than in the browser so that the first
 * paint already has the file tree and the project's name: opening a project
 * should not begin with an empty frame that then fills in.
 */
export default async function ProjectPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect(`/login?next=/projects/${id}`)
  }

  let view
  try {
    view = await getProject(id, headers)
  } catch (error) {
    // A project somebody may not open and one that does not exist are the
    // same answer here, deliberately: the API does not tell them apart, and
    // neither should this.
    if (error instanceof ApiError && error.status === 404) {
      notFound()
    }
    throw error
  }

  const site = await fetchSite(headers)

  return (
    <IdePageClient
      user={user}
      view={view}
      site={{
        appName: site.name,
        siteUrl: site.url ?? '',
        // A self-hosted site has no support desk and no wiki of its own.
        showSupport: false,
        wikiEnabled: false,
        symbolPaletteAvailable: true,
        capabilities: ['chat', 'link-sharing'],
        gitBridgeEnabled: Boolean(site.git?.enabled),
        hasLinkUrlFeature: true,
      }}
    />
  )
}
