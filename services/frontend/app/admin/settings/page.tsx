import { redirect } from 'next/navigation'
import { SiteHeader } from '@/components/site-header'
import { currentUser } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { describeSettings } from '@/lib/settings'
import { siteName } from '@/lib/site'
import { SettingsForm } from './settings-form'

export const metadata = { title: 'Settings' }

export default async function AdminSettingsPage() {
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect('/login')
  }
  // The API refuses this to anybody who is not an administrator; checking here
  // as well is so that somebody who is not gets a sensible page rather than an
  // error where a page should be.
  if (!user.isAdmin) {
    redirect('/projects')
  }

  const [description, name] = await Promise.all([
    describeSettings(headers),
    siteName(headers),
  ])

  return (
    <>
      <SiteHeader user={user} siteName={name} />
      <main className="mx-auto w-full max-w-3xl px-4 py-10">
        <SettingsForm initial={description} />
      </main>
    </>
  )
}
