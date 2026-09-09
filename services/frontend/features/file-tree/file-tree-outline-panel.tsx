'use client'

/**
 * The file tree over the outline, from
 * file-tree/components/file-tree-outline-panel.tsx and file-tree-toolbar.tsx.
 *
 * Two panels one above the other, each with a header that collapses it, and
 * the tree's actions -- new file, new folder, upload -- in the tree's header.
 */

import { useCallback, useRef } from 'react'
import { Panel, PanelGroup, type ImperativePanelHandle } from 'react-resizable-panels'
import { useTranslation } from '@/lib/i18n'
import { usePersistedState } from '@/lib/hooks'
import MaterialIcon from '@/components/ol/material-icon'
import { Tooltip } from '@/components/ol/tooltip'
import type { AvailableUnfilledIcon } from '@/lib/unfilled-symbols'
import { useProject } from '@/features/ide/contexts/project-context'
import { useRailContext } from '@/features/ide/contexts/rail-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { useCollapsiblePanel } from '@/features/ide/hooks/use-collapsible-panel'
import { VerticalResizeHandle } from '@/features/ide/components/resize/resize-handles'
import { FileTree } from './file-tree'
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
  const toggleFileTreeExpanded = useCallback(() => setFileTreeExpanded(value => !value), [setFileTreeExpanded])

  return (
    <PanelGroup className="file-tree-outline-panel-group" autoSaveId="ide-redesign-file-tree-outline" direction="vertical">
      <Panel
        className={['file-tree-panel', !fileTreeExpanded ? 'file-tree-panel-collapsed' : ''].filter(Boolean).join(' ')}
        defaultSize={50}
        id="ide-redesign-file-tree"
        order={1}
        collapsible
        ref={fileTreePanelRef}
        onExpand={expandFileTree}
        onCollapse={collapseFileTree}
        minSize={10}
      >
        <div className="file-tree">
          <FileTreeToolbar fileTreeExpanded={fileTreeExpanded} toggleFileTreeExpanded={toggleFileTreeExpanded} />
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
  )
}

function FileTreeToolbar({ fileTreeExpanded, toggleFileTreeExpanded }: { fileTreeExpanded: boolean; toggleFileTreeExpanded: () => void }) {
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

function FileTreeActionButtons({ fileTreeExpanded }: { fileTreeExpanded: boolean }) {
  const { t } = useTranslation()
  const { canWrite } = useProject()
  const { handlePaneCollapse } = useRailContext()

  const startCreatingDocOrFile = useCallback(() => window.dispatchEvent(new CustomEvent('ide:new-file')), [])
  const startCreatingFolder = useCallback(() => window.dispatchEvent(new CustomEvent('ide:new-folder')), [])
  const startUploadingDocOrFile = useCallback(() => window.dispatchEvent(new CustomEvent('ide:upload')), [])

  useCommandProvider(() => {
    if (!canWrite) return
    return [
      { label: t('new_file'), id: 'new_file', handler: startCreatingDocOrFile },
      { label: t('new_folder'), id: 'new_folder', handler: startCreatingFolder },
      { label: t('upload_file'), id: 'upload_file', handler: startUploadingDocOrFile },
    ]
  }, [canWrite, t, startCreatingDocOrFile, startCreatingFolder, startUploadingDocOrFile])

  return (
    <div className="file-tree-toolbar-action-buttons">
      {fileTreeExpanded && canWrite ? (
        <>
          <FileTreeActionButton id="new-file" description={t('new_file')} onClick={startCreatingDocOrFile} iconType="note_add" />
          <FileTreeActionButton id="new-folder" description={t('new_folder')} onClick={startCreatingFolder} iconType="create_new_folder" />
          <FileTreeActionButton id="upload" description={t('upload')} onClick={startUploadingDocOrFile} iconType="upload" />
        </>
      ) : null}
      <FileTreeActionButton id="close" description={t('close')} onClick={handlePaneCollapse} iconType="close" />
    </div>
  )
}

function FileTreeActionButton({
  id,
  description,
  onClick,
  iconType,
}: {
  id: string
  description: string
  onClick: () => void
  iconType: AvailableUnfilledIcon
}) {
  return (
    <Tooltip id={id} description={description} overlayProps={{ placement: 'bottom' }}>
      <button type="button" className="btn file-tree-toolbar-action-button" onClick={onClick}>
        <MaterialIcon unfilled type={iconType} accessibilityLabel={description} />
      </button>
    </Tooltip>
  )
}
