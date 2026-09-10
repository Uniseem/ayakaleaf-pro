'use client'

import { useTranslation } from '@/lib/i18n'
import { DropdownDivider, DropdownItem } from '@/components/ol/dropdown'
import { useFileTreeActionable } from '../../contexts/file-tree-actionable'

/** What the context menu offers, from file-tree-item/file-tree-item-menu-items. */
export function FileTreeItemMenuItems() {
  const { t } = useTranslation()

  const {
    canRename,
    canDelete,
    canBulkDelete,
    canCreate,
    startRenaming,
    startDeleting,
    startCreatingFolder,
    startCreatingDocOrFile,
    startUploadingDocOrFile,
    downloadPath,
    selectedFileName,
    canSetRootDocId,
    setRootDocId,
  } = useFileTreeActionable()

  return (
    <>
      {canRename ? (
        <li role="none">
          <DropdownItem as="button" onClick={startRenaming}>
            {t('rename')}
          </DropdownItem>
        </li>
      ) : null}
      {downloadPath ? (
        <li role="none">
          <DropdownItem href={downloadPath} download={selectedFileName ?? undefined}>
            {t('download')}
          </DropdownItem>
        </li>
      ) : null}
      {canSetRootDocId ? (
        <>
          <DropdownDivider />
          <li role="none">
            <DropdownItem as="button" onClick={setRootDocId}>
              {t('set_as_main_document')}
            </DropdownItem>
          </li>
        </>
      ) : null}
      {canDelete || canBulkDelete ? (
        <>
          <DropdownDivider />
          <li role="none">
            <DropdownItem as="button" onClick={startDeleting}>
              {t('delete')}
            </DropdownItem>
          </li>
        </>
      ) : null}
      {canCreate ? (
        <>
          <DropdownDivider />
          <li role="none">
            <DropdownItem as="button" onClick={startCreatingDocOrFile}>
              {t('new_file')}
            </DropdownItem>
          </li>
          <li role="none">
            <DropdownItem as="button" onClick={startCreatingFolder}>
              {t('new_folder')}
            </DropdownItem>
          </li>
          <li role="none">
            <DropdownItem as="button" onClick={startUploadingDocOrFile}>
              {t('upload')}
            </DropdownItem>
          </li>
        </>
      ) : null}
    </>
  )
}

export default FileTreeItemMenuItems
