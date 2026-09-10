import { Card, CardBody, CardHeader } from '@/components/ol/card'
import { redirect } from 'next/navigation'
import { SiteFooter, SiteHeader } from '@/components/site-header'
import { linkedProviders } from '@/lib/account'
import { currentUser, displayName } from '@/lib/auth'
import { forwardedHeaders } from '@/lib/server'
import { siteName } from '@/lib/site'
import { githubStatus } from '@/lib/github'
import { listTokens } from '@/lib/tokens'
import { GitAccess } from './git-access'
import { GitHubAccount } from './github'
import { LinkedAccounts } from './linked-accounts'
import { EditorSettings } from '@/features/settings/editor-settings'
import { PasswordForm } from '@/features/settings/password-form'
import { Sessions } from '@/features/settings/sessions'
import { SettingsProvider } from '@/features/ide/contexts/settings-context'
import { getUserSettings } from '@/lib/user-settings'

export const metadata = { title: 'Account' }

export default async function AccountPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string }>
}) {
  const query = await searchParams
  const headers = await forwardedHeaders()
  const user = await currentUser(headers).catch(() => null)
  if (!user) {
    redirect('/login')
  }

  const [providers, name, tokens, github, stored] = await Promise.all([
    linkedProviders(headers).catch(() => []),
    siteName(headers),
    listTokens(headers).catch(() => []),
    githubStatus(headers).catch(() => ({ enabled: false, connected: false })),
    // An account that has never changed anything has none, which is not an
    // error: the client fills in its own defaults for whatever is absent.
    getUserSettings(headers).catch(() => ({})),
  ])

  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader user={user} siteName={name} />
      <main className="mx-auto flex w-full max-w-2xl flex-col gap-6 px-4 py-10">
        <h1 className="text-2xl font-semibold tracking-tight">Account</h1>

        {/* Written by the API, never by a provider, so a link cannot put words
            on this page. */}
        {query.error ? (
          <div
            role="alert"
            className="alert alert-danger"
          >
            {query.error}
          </div>
        ) : null}

        <Card>
          <CardHeader title="Who you are" />
          <CardBody className="flex flex-col gap-2">
            <div className="flex justify-between gap-4">
              <span className="text-[var(--content-secondary)]">Name</span>
              <span>{displayName(user)}</span>
            </div>
            <div className="flex justify-between gap-4">
              <span className="text-[var(--content-secondary)]">Email</span>
              <span>{user.email}</span>
            </div>
          </CardBody>
        </Card>

        <SettingsProvider initial={stored as Record<string, never>}>
          <EditorSettings />
        </SettingsProvider>
        <PasswordForm />
        <Sessions />
        <LinkedAccounts providers={providers} />
        <GitAccess tokens={tokens} />
        <GitHubAccount status={github} />
      </main>
      <SiteFooter siteName={name} />
    </div>
  )
}
