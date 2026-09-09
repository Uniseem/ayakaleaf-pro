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

import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels'
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
import { Rail } from './rail'
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

  return (
    <div className="ide-shell flex flex-col bg-background text-foreground">
      <Toolbar userName={userName} />

      <PanelGroup direction="horizontal" className="min-h-0 flex-1">
        {/* The rail's own strip is fixed width and lives outside the panel,
            so dragging the handle resizes the panel and not the buttons. */}
        <div className="flex min-h-0">
          <Rail />
        </div>

        {layout.railIsOpen ? (
          <PanelResizeHandle className="w-px bg-divider transition-colors hover:bg-primary data-[resize-handle-active]:bg-primary" />
        ) : null}

        <Panel id="main" order={2} minSize={30}>
          {layout.view === 'history' ? <HistoryView /> : null}

          <PanelGroup
            direction="horizontal"
            autoSaveId="ide.main"
            className={layout.view === 'history' ? 'hidden' : ''}
          >
            {layout.editorIsOpen ? (
              <Panel id="editor" order={1} minSize={20} defaultSize={50}>
                <EditorPane />
              </Panel>
            ) : null}

            {layout.editorIsOpen && layout.pdfIsOpen ? (
              <PanelResizeHandle className="w-px bg-divider transition-colors hover:bg-primary data-[resize-handle-active]:bg-primary" />
            ) : null}

            {layout.pdfIsOpen ? (
              <Panel id="pdf" order={2} minSize={20} defaultSize={50}>
                <PdfPane />
              </Panel>
            ) : null}
          </PanelGroup>
        </Panel>
      </PanelGroup>
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
