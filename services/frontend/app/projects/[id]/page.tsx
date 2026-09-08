import { notFound, redirect } from 'next/navigation'
import { currentUser } from '@/lib/auth'
import { getProject } from '@/lib/editor'
import { ApiError } from '@/lib/api'
import { forwardedHeaders } from '@/lib/server'
import { site } from '@/lib/site'
import { Editor } from './editor'

export const metadata = { title: 'Editor' }

/**
 * The editor.
 *
 * The project is fetched here rather than in the browser so that the first
 * paint already has the file tree and the project's name: opening a project
 * should not begin with an empty frame that then fills in.
 */
export default async function ProjectPage({ params }: { params: { id: string } }) {
  const headers = forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect(`/login?next=/projects/${params.id}`)
  }

  const where = await site(headers)

  let view
  try {
    view = await getProject(params.id, headers)
  } catch (error) {
    // A project somebody may not open and one that does not exist are the
    // same answer here, deliberately: the API does not tell them apart, and
    // neither should this.
    if (error instanceof ApiError && error.status === 404) {
      notFound()
    }
    throw error
  }

  return <Editor user={user} view={view} git={where.git?.enabled ?? false} />
}
