'use client'

/**
 * The file tree over the outline, from
 * file-tree/components/file-tree-outline-panel.tsx and file-tree-toolbar.tsx.
 *
 * Two panels one above the other, each with a header that collapses it, and
 * the tree's actions -- new file, new folder, upload -- in the tree's header.
 *
 * The tree's providers wrap both, because the header's buttons and the rows
 * below are asking the same context the same questions: what is selected, and
 * what is being created.
 */

import { useCallback, useRef } from 'react'
import { Panel, PanelGroup, type ImperativePanelHandle } from 'react-resizable-panels'
import { useTranslation } from '@/lib/i18n'
import { usePersistedState } from '@/lib/hooks'
import MaterialIcon from '@/components/ol/material-icon'
import { useCollapsiblePanel } from '@/features/ide/hooks/use-collapsible-panel'
import { VerticalResizeHandle } from '@/features/ide/components/resize/resize-handles'
import { FileTree, FileTreeProviders } from './file-tree'
import FileTreeActionButtons from './components/file-tree-action-buttons'
import { OutlineContainer, useOutlineState } from '@/features/outline/outline-pane'

export function FileTreeOutlinePanel() {
  const { canShowOutline, outlineExpanded, expandOutline, collapseOutline } = useOutlineState()
  const outlinePanelRef = useRef<ImperativePanelHandle | null>(null)
  useCollapsiblePanel(canShowOutline && outlineExpanded, outlinePanelRef)

  const [fileTreeExpanded, setFileTreeExpanded] = usePersistedState('file-tree-expanded', true)
  const fileTreePanelRef = useRef<ImperativePanelHandle | null>(null)
  useCollapsiblePanel(fileTreeExpanded, fileTreePanelRef)

  const expandFileTree = useCallback(() => setFileTreeExpanded(true), [setFileTreeExpanded])
  const collapseFileTree = useCallback(() => setFileTreeExpanded(false), [setFileTreeExpanded])
  const toggleFileTreeExpanded = useCallback(
    () => setFileTreeExpanded(value => !value),
    [setFileTreeExpanded]
  )

  return (
    <FileTreeProviders>
      <PanelGroup
        className="file-tree-outline-panel-group"
        autoSaveId="ide-redesign-file-tree-outline"
        direction="vertical"
      >
        <Panel
          className={['file-tree-panel', !fileTreeExpanded ? 'file-tree-panel-collapsed' : '']
            .filter(Boolean)
            .join(' ')}
          defaultSize={50}
          id="ide-redesign-file-tree"
          order={1}
          collapsible
          ref={fileTreePanelRef}
          onExpand={expandFileTree}
          onCollapse={collapseFileTree}
          minSize={10}
        >
          <div className="file-tree" data-testid="file-tree">
            <FileTreeToolbar
              fileTreeExpanded={fileTreeExpanded}
              toggleFileTreeExpanded={toggleFileTreeExpanded}
            />
            <FileTree />
          </div>
        </Panel>
        <VerticalResizeHandle hitAreaMargins={{ coarse: 0, fine: 0 }} disabled={!canShowOutline} />
        <Panel
          className="file-outline-panel"
          defaultSize={50}
          id="ide-redesign-file-outline"
          order={2}
          collapsible
          ref={outlinePanelRef}
          onExpand={expandOutline}
          onCollapse={collapseOutline}
          minSize={10}
        >
          <OutlineContainer />
        </Panel>
      </PanelGroup>
    </FileTreeProviders>
  )
}

function FileTreeToolbar({
  fileTreeExpanded,
  toggleFileTreeExpanded,
}: {
  fileTreeExpanded: boolean
  toggleFileTreeExpanded: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className="file-tree-toolbar">
      <button
        type="button"
        className="file-tree-expand-collapse-button"
        onClick={toggleFileTreeExpanded}
        aria-label={fileTreeExpanded ? t('hide_file_tree') : t('show_file_tree')}
      >
        <MaterialIcon type={fileTreeExpanded ? 'keyboard_arrow_down' : 'keyboard_arrow_right'} />
        <h4>{t('file_tree')}</h4>
      </button>
      <FileTreeActionButtons fileTreeExpanded={fileTreeExpanded} />
    </div>
  )
}
