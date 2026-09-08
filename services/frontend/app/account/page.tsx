import { Card, CardBody, CardHeader } from '@heroui/react'
import { redirect } from 'next/navigation'
import { SiteHeader } from '@/components/site-header'
import { linkedProviders } from '@/lib/account'
import { currentUser, displayName } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { LinkedAccounts } from './linked-accounts'

export const metadata = { title: 'Account' }

export default async function AccountPage({
  searchParams,
}: {
  searchParams: { error?: string }
}) {
  const headers = forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect('/login')
  }

  const [providers, name] = await Promise.all([
    linkedProviders(headers).catch(() => []),
    siteName(headers),
  ])

  return (
    <>
      <SiteHeader user={user} siteName={name} />
      <main className="mx-auto flex w-full max-w-2xl flex-col gap-6 px-4 py-10">
        <h1 className="text-2xl font-semibold tracking-tight">Account</h1>

        {/* Written by the API, never by a provider, so a link cannot put words
            on this page. */}
        {searchParams.error ? (
          <div
            role="alert"
            className="rounded-medium border border-danger-200 bg-danger-50 px-4 py-3 text-small text-danger-700 dark:bg-danger-50/10"
          >
            {searchParams.error}
          </div>
        ) : null}

        <Card shadow="sm">
          <CardHeader className="px-6 pt-6">
            <h2 className="text-lg font-medium">Who you are</h2>
          </CardHeader>
          <CardBody className="gap-2 px-6 pb-6">
            <div className="flex justify-between gap-4">
              <span className="text-default-500">Name</span>
              <span>{displayName(user)}</span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-default-500">Email</span>
              <span>{user.email}</span>
            </div>
          </CardBody>
        </Card>

        <LinkedAccounts providers={providers} />
      </main>
    </>
  )
}
