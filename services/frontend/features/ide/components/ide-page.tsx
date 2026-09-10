'use client'

/**
 * The editor page, from ide-react/components/layout/ide-page.tsx and the
 * providers around it.
 *
 * Providers on the outside, the layout on the inside. Nothing in here
 * fetches: the page fetched the project on the server and handed it in, so
 * the first paint is the project rather than a spinner.
 */

import { useEffect, useMemo } from 'react'
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
import CommandPalette from '@/features/command-palette/components/command-palette'
import { GlobalToasts } from '@/features/ide/components/global-toasts'
import { ChatProvider } from '@/features/chat/contexts/chat-context'
import { ThreadsProvider } from '@/features/review-panel/contexts/threads-context'
import { ReviewPanelViewProvider } from '@/features/review-panel/contexts/review-panel-view-context'

export type IdePageProps = {
  user: PublicUser
  view: ProjectView
  site: Omit<SiteValue, 'user'>
}

export function IdePage({ user, view, site }: IdePageProps) {
  setOwnUserId(user.id)

  // The page is a fixed-size shell: pinning the document keeps anything
  // that lands outside it (CodeMirror's tooltip container, for one) from
  // making the window scroll, as the original did with the same class.
  useEffect(() => {
    document.documentElement.classList.add('fixed-size-document')
    return () => {
      document.documentElement.classList.remove('fixed-size-document')
    }
  }, [])
  const siteValue = useMemo<SiteValue>(() => ({ ...site, user }), [site, user])

  return (
    <SiteProvider value={siteValue}>
      <SettingsProvider>
        <ProjectProvider initial={view}>
          <ConnectionProvider>
            <LayoutProvider>
              <EditorProvider>
                <CompileProvider>
                  <ReviewProvider>
                    <CommandRegistryProvider>
                      <EditorPropertiesProvider>
                        <MetadataProvider>
                          <OutlineProvider>
                            <ChatProvider>
                            <ThreadsProvider>
                            <ReviewPanelViewProvider>
                            <RailProvider>
                              <TabsProvider>
                                <div id="ide-root" className="ide-shell">
                                  <SettingsModal />
                                  <CommandPalette />
                                  <MainLayout />
                                  <GlobalToasts />
                                </div>
                              </TabsProvider>
                            </RailProvider>
                            </ReviewPanelViewProvider>
                            </ThreadsProvider>
                            </ChatProvider>
                          </OutlineProvider>
                        </MetadataProvider>
                      </EditorPropertiesProvider>
                    </CommandRegistryProvider>
                  </ReviewProvider>
                </CompileProvider>
              </EditorProvider>
            </LayoutProvider>
          </ConnectionProvider>
        </ProjectProvider>
      </SettingsProvider>
    </SiteProvider>
  )
}
