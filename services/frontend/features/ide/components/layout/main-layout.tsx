'use client'

/**
 * Where everything goes, from ide-react/components/layout/main-layout.tsx.
 *
 * The toolbar across the top; below it the rail, its panel, and a group of
 * two panels for the editor and the PDF. History replaces the two panels.
 * Panels are hidden rather than removed while a layout has no use for them,
 * so the group keeps their sizes for when they come back.
 */

import { useCallback, useRef, useState } from 'react'
import { Panel, PanelGroup, type ImperativePanelHandle } from 'react-resizable-panels'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useRailContext } from '@/features/ide/contexts/rail-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useAreTabsEnabled } from '@/features/ide/contexts/tabs-context'
import { useSettings, editorFontStyle } from '@/features/ide/contexts/settings-context'
import { HorizontalResizeHandle, HorizontalToggler } from '@/features/ide/components/resize/resize-handles'
import { RailLayout } from '@/features/ide/components/rail/rail'
import { Toolbar } from '@/features/ide/components/toolbar/toolbar'
import { TabsContainer } from '@/features/source-editor/tabs/tabs'
import { SourceEditor } from '@/features/source-editor/editor'
import { EditorToolbar } from '@/features/source-editor/editor-toolbar'
import { PdfPane } from '@/features/pdf-preview/pdf-pane'
import { FileView } from '@/features/file-view/file-view'
import { HistoryView } from '@/features/history/history-view'
import { useCollapsiblePanel } from '@/features/ide/hooks/use-collapsible-panel'

function usePdfPane() {
  const { view, pdfLayout, changeLayout } = useLayout()
  const pdfPanelRef = useRef<ImperativePanelHandle | null>(null)
  const pdfIsOpen = pdfLayout === 'sideBySide' || view === 'pdf'

  useCollapsiblePanel(pdfIsOpen, pdfPanelRef)

  const togglePdfPane = useCallback(() => {
    if (pdfIsOpen) {
      changeLayout('flat', 'editor')
    } else {
      changeLayout('sideBySide')
    }
  }, [changeLayout, pdfIsOpen])

  const setPdfIsOpen = useCallback(
    (value: boolean) => {
      if (value) {
        changeLayout('sideBySide')
      } else {
        changeLayout('flat', 'editor')
      }
    },
    [changeLayout]
  )

  const handlePdfPaneExpand = useCallback(() => {
    if (pdfLayout === 'flat' && view === 'editor') {
      changeLayout('sideBySide', 'editor')
    }
  }, [changeLayout, pdfLayout, view])

  const handlePdfPaneCollapse = useCallback(() => {
    if (pdfLayout === 'sideBySide') {
      changeLayout('flat', 'editor')
    }
  }, [changeLayout, pdfLayout])

  return { togglePdfPane, handlePdfPaneExpand, handlePdfPaneCollapse, setPdfIsOpen, pdfIsOpen, pdfPanelRef }
}

export function MainLayout() {
  const [resizing, setResizing] = useState(false)
  const { resizing: railResizing } = useRailContext()
  const { togglePdfPane, handlePdfPaneExpand, handlePdfPaneCollapse, setPdfIsOpen, pdfIsOpen, pdfPanelRef } = usePdfPane()
  const { view, pdfLayout } = useLayout()
  const { t } = useTranslation()

  const editorIsOpen = view === 'editor' || view === 'file' || pdfLayout === 'sideBySide'

  return (
    <div className="ide-redesign-main">
      <Toolbar />
      <div className="ide-redesign-body">
        <PanelGroup
          autoSaveId="ide-redesign-outer-layout"
          direction="horizontal"
          className={['ide-redesign-inner', resizing || railResizing ? 'ide-panel-group-resizing' : ''].filter(Boolean).join(' ')}
        >
          <RailLayout />
          <Panel id="ide-redesign-editor-and-pdf-panel" order={2}>
            {view === 'history' ? <HistoryView /> : null}
            <PanelGroup
              autoSaveId="ide-redesign-editor-and-pdf-panel-group"
              direction="horizontal"
              className={view === 'history' ? 'hidden' : undefined}
            >
              <Panel
                id="ide-redesign-editor-panel"
                order={1}
                className={!editorIsOpen || view === 'history' ? 'hidden' : undefined}
                minSize={5}
                defaultSize={50}
                tagName="section"
                aria-label={t('editor')}
              >
                <div className="ide-redesign-editor-container">
                  <EditorPanel />
                </div>
              </Panel>
              <HorizontalResizeHandle
                resizable={pdfLayout === 'sideBySide'}
                onDragging={setResizing}
                onDoubleClick={togglePdfPane}
                hitAreaMargins={{ coarse: 0, fine: 0 }}
                className={!editorIsOpen ? 'hidden' : undefined}
              >
                <HorizontalToggler
                  id="ide-redesign-pdf-panel"
                  togglerType="east"
                  isOpen={pdfIsOpen}
                  setIsOpen={setPdfIsOpen}
                  tooltipWhenOpen={t('tooltip_hide_pdf')}
                  tooltipWhenClosed={t('tooltip_show_pdf')}
                />
              </HorizontalResizeHandle>
              <Panel
                collapsible
                className={['ide-redesign-pdf-container', view === 'history' ? 'hidden' : ''].filter(Boolean).join(' ')}
                id="ide-redesign-pdf-panel"
                order={2}
                defaultSize={50}
                minSize={5}
                ref={pdfPanelRef}
                onExpand={handlePdfPaneExpand}
                onCollapse={handlePdfPaneCollapse}
                tagName="section"
                aria-label={t('pdf_preview')}
              >
                <PdfPane />
              </Panel>
            </PanelGroup>
          </Panel>
        </PanelGroup>
      </div>
    </div>
  )
}

/**
 * The editor half, from ide-react/components/editor/editor-panel.tsx: the
 * tabs, then whichever of a document, a binary file, or nothing is open.
 */
function EditorPanel() {
  const editor = useEditor()
  const settings = useSettings()
  const tabsEnabled = useAreTabsEnabled()
  const { t } = useTranslation()

  const nothingOpen = !editor.current && !editor.currentFile

  return (
    <>
      {tabsEnabled ? <TabsContainer /> : null}
      {nothingOpen ? (
        <div className="no-file-selection">
          <div className="no-file-selection-message">
            <h3>{t('no_selection_select_file')}</h3>
          </div>
        </div>
      ) : null}
      {editor.currentFile ? <FileView entry={editor.currentFile} onClose={editor.close} key={editor.currentFile.id} /> : null}
      {editor.current ? (
        <div className="ide-redesign-editor-content">
          <PanelGroup autoSaveId="ide-redesign-editor-symbol-palette" direction="vertical">
            <Panel id="ide-redesign-panel-source-editor" order={1} className="ide-redesign-editor-panel">
              <EditorToolbar />
              <div className="min-h-0 flex-1" style={editorFontStyle(settings)}>
                <SourceEditor />
              </div>
            </Panel>
          </PanelGroup>
        </div>
      ) : null}
    </>
  )
}
