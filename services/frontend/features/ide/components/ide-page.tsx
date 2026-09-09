'use client'

/**
 * The editor page.
 *
 * Providers on the outside, panels on the inside, and the panels are resizable
 * because the right split between text and PDF is a matter of what somebody is
 * doing at the time. The sizes are remembered by the panel library under the
 * ids given here.
 *
 * Nothing in here fetches: the page fetched the project on the server and
 * handed it in, so the first paint is the project rather than a spinner.
 */

import { useEffect, useRef } from 'react'
import {
  Panel,
  PanelGroup,
  PanelResizeHandle,
  type ImperativePanelHandle,
} from 'react-resizable-panels'
import type { ProjectView } from '@/lib/editor'
import { displayName, type PublicUser } from '@/lib/auth'
import { ProjectProvider } from '@/features/ide/contexts/project-context'
import { LayoutProvider, useLayout } from '@/features/ide/contexts/layout-context'
import { CompileProvider } from '@/features/ide/contexts/compile-context'
import { ConnectionProvider } from '@/features/ide/contexts/connection-context'
import { EditorProvider, useEditor } from '@/features/ide/contexts/editor-context'
import { SettingsProvider, useSettings, editorFontStyle } from '@/features/ide/contexts/settings-context'
import { SourceEditor } from '@/features/source-editor/editor'
import { PdfPane } from '@/features/pdf-preview/pdf-pane'
import { FileView } from '@/features/file-view/file-view'
import { HistoryView } from '@/features/history/history-view'
import { RailTabs, RailPanel } from './rail'
import { Toolbar } from './toolbar'

export function IdePage({
  user,
  view,
}: {
  user: PublicUser
  view: ProjectView
}) {
  return (
    <SettingsProvider>
      <ProjectProvider initial={view}>
        <ConnectionProvider>
          <LayoutProvider>
            <CompileProvider>
              <EditorProvider>
                <IdeShell userName={displayName(user)} />
              </EditorProvider>
            </CompileProvider>
          </LayoutProvider>
        </ConnectionProvider>
      </ProjectProvider>
    </SettingsProvider>
  )
}

function IdeShell({ userName }: { userName: string }) {
  const layout = useLayout()

  // The panels are collapsed, never removed. react-resizable-panels registers
  // its panels with the group and sizes them against each other, so taking one
  // out of the tree leaves the group looking for something that is no longer
  // there -- and the saved layout, which names them, restores into nothing.
  const railPanel = useRef<ImperativePanelHandle>(null)
  const editorPanel = useRef<ImperativePanelHandle>(null)
  const pdfPanel = useRef<ImperativePanelHandle>(null)

  useEffect(() => {
    const panel = railPanel.current
    if (!panel) {
      return
    }
    if (layout.railIsOpen) {
      panel.expand()
    } else {
      panel.collapse()
    }
  }, [layout.railIsOpen])

  useEffect(() => {
    const panel = editorPanel.current
    if (!panel) {
      return
    }
    if (layout.editorIsOpen) {
      panel.expand()
    } else {
      panel.collapse()
    }
  }, [layout.editorIsOpen])

  useEffect(() => {
    const panel = pdfPanel.current
    if (!panel) {
      return
    }
    if (layout.pdfIsOpen) {
      panel.expand()
    } else {
      panel.collapse()
    }
  }, [layout.pdfIsOpen])

  const handle =
    'w-px bg-divider transition-colors hover:bg-primary data-[resize-handle-active]:bg-primary'

  return (
    <div className="ide-shell flex flex-col bg-background text-foreground">
      <Toolbar userName={userName} />

      <div className="flex min-h-0 flex-1">
        {/* The strip of buttons is a fixed width and is not part of any panel
            group, so dragging a handle resizes a panel and not the buttons. */}
        <RailTabs />

        <PanelGroup
          direction="horizontal"
          autoSaveId="ide.outer"
          className="min-h-0 flex-1"
        >
          <Panel
            id="rail"
            order={1}
            ref={railPanel}
            collapsible
            collapsedSize={0}
            defaultSize={18}
            minSize={12}
            maxSize={40}
          >
            <RailPanel />
          </Panel>

          <PanelResizeHandle className={handle} disabled={!layout.railIsOpen} />

          <Panel id="main" order={2} minSize={30}>
            {layout.view === 'history' ? (
              <HistoryView />
            ) : (
              <PanelGroup
                direction="horizontal"
                autoSaveId="ide.main"
                className="h-full"
              >
                <Panel
                  id="editor"
                  order={1}
                  ref={editorPanel}
                  collapsible
                  collapsedSize={0}
                  minSize={20}
                  defaultSize={50}
                >
                  <EditorPane />
                </Panel>

                <PanelResizeHandle
                  className={handle}
                  disabled={!layout.editorIsOpen || !layout.pdfIsOpen}
                />

                <Panel
                  id="pdf"
                  order={2}
                  ref={pdfPanel}
                  collapsible
                  collapsedSize={0}
                  minSize={20}
                  defaultSize={50}
                >
                  <PdfPane />
                </Panel>
              </PanelGroup>
            )}
          </Panel>
        </PanelGroup>
      </div>
    </div>
  )
}

/**
 * Whichever of the two things can be in the editor half.
 *
 * A binary file is not a document: it has no text to edit, so it gets a viewer
 * rather than an editor that would show its bytes.
 */
function EditorPane() {
  const editor = useEditor()
  const settings = useSettings()

  if (editor.currentFile) {
    return <FileView entry={editor.currentFile} onClose={editor.close} />
  }

  return (
    <div className="h-full min-h-0" style={editorFontStyle(settings)}>
      <SourceEditor />
    </div>
  )
}
