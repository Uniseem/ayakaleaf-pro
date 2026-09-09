import { redirect } from 'next/navigation'
import { SiteHeader } from '@/components/site-header'
import { currentUser } from '@/lib/auth'
import { listProjects, listTags } from '@/lib/projects'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { ProjectList } from '@/features/project-list/project-list'

export const metadata = { title: 'Projects' }

export default async function ProjectsPage() {
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect('/login')
  }

  const [projects, tags, name] = await Promise.all([
    listProjects(headers),
    // Tags are the one of the three this page can do without: an instance with
    // none, or one whose tag store is not answering, still has projects worth
    // showing.
    listTags(headers).catch(() => []),
    siteName(headers),
  ])

  return (
    <>
      <SiteHeader user={user} siteName={name} />
      <ProjectList initial={projects} initialTags={tags} userId={user.id} />
    </>
  )
}
