'use client'

/**
 * The editor page, from ide-react/components/layout/ide-page.tsx and the
 * providers around it.
 *
 * Providers on the outside, the layout on the inside. Nothing in here
 * fetches: the page fetched the project on the server and handed it in, so
 * the first paint is the project rather than a spinner.
 */

import { useMemo } from 'react'
import type { ProjectView } from '@/lib/editor'
import type { PublicUser } from '@/lib/auth'
import { setOwnUserId } from '@/lib/colors'
import { ProjectProvider } from '@/features/ide/contexts/project-context'
import { LayoutProvider } from '@/features/ide/contexts/layout-context'
import { CompileProvider } from '@/features/ide/contexts/compile-context'
import { ConnectionProvider } from '@/features/ide/contexts/connection-context'
import { EditorProvider } from '@/features/ide/contexts/editor-context'
import { ReviewProvider } from '@/features/ide/contexts/review-context'
import { SettingsProvider } from '@/features/ide/contexts/settings-context'
import { CommandRegistryProvider } from '@/features/ide/contexts/command-registry-context'
import { RailProvider } from '@/features/ide/contexts/rail-context'
import { TabsProvider } from '@/features/ide/contexts/tabs-context'
import { SiteProvider, type SiteValue } from '@/features/ide/contexts/site-context'
import { OutlineProvider } from '@/features/ide/contexts/outline-context'
import { MetadataProvider } from '@/features/ide/contexts/metadata-context'
import { EditorPropertiesProvider } from '@/features/ide/contexts/editor-properties-context'
import { MainLayout } from '@/features/ide/components/layout/main-layout'
import { SettingsModal } from '@/features/settings/settings-modal'

export type IdePageProps = {
  user: PublicUser
  view: ProjectView
  site: Omit<SiteValue, 'user'>
}

export function IdePage({ user, view, site }: IdePageProps) {
  setOwnUserId(user.id)
  const siteValue = useMemo<SiteValue>(() => ({ ...site, user }), [site, user])

  return (
    <SiteProvider value={siteValue}>
      <SettingsProvider>
        <ProjectProvider initial={view}>
          <ConnectionProvider>
            <LayoutProvider>
              <CompileProvider>
                <EditorProvider>
                  <ReviewProvider>
                    <CommandRegistryProvider>
                      <EditorPropertiesProvider>
                        <MetadataProvider>
                          <OutlineProvider>
                      <RailProvider>
                        <TabsProvider>
                          <div id="ide-root" className="ide-shell">
                            <SettingsModal />
                            <MainLayout />
                          </div>
                        </TabsProvider>
                      </RailProvider>
                          </OutlineProvider>
                        </MetadataProvider>
                      </EditorPropertiesProvider>
                    </CommandRegistryProvider>
                  </ReviewProvider>
                </EditorProvider>
              </CompileProvider>
            </LayoutProvider>
          </ConnectionProvider>
        </ProjectProvider>
      </SettingsProvider>
    </SiteProvider>
  )
}
