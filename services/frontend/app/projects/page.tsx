import { redirect } from 'next/navigation'
import { SiteHeader } from '@/components/site-header'
import { currentUser } from '@/lib/auth'
import { listProjects } from '@/lib/projects'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { ProjectList } from './project-list'

export const metadata = { title: 'Projects' }

export default async function ProjectsPage() {
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect('/login')
  }

  const [projects, name] = await Promise.all([
    listProjects(headers),
    siteName(headers),
  ])

  return (
    <>
      <SiteHeader user={user} siteName={name} />
      <main className="mx-auto w-full max-w-6xl px-4 py-10">
        <ProjectList projects={projects} />
      </main>
    </>
  )
}
