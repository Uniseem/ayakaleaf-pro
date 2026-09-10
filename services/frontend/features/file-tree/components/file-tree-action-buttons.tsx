'use client'

import { useTranslation } from '@/lib/i18n'
import { useProject } from '@/features/ide/contexts/project-context'
import { useRailContext } from '@/features/ide/contexts/rail-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { useFileTreeActionable } from '../contexts/file-tree-actionable'
import FileTreeActionButton from './file-tree-action-button'

/** New file, new folder, upload, delete: the file tree's own header buttons. */
export function FileTreeActionButtons({ fileTreeExpanded }: { fileTreeExpanded: boolean }) {
  const { t } = useTranslation()
  const { canWrite } = useProject()
  const { handlePaneCollapse } = useRailContext()

  const {
    canCreate,
    canBulkDelete,
    startDeleting,
    startCreatingFolder,
    startCreatingDocOrFile,
    startUploadingDocOrFile,
  } = useFileTreeActionable()

  useCommandProvider(() => {
    if (!canCreate || !canWrite) {
      return
    }
    return [
      { label: t('new_file'), id: 'new_file', handler: startCreatingDocOrFile },
      { label: t('new_folder'), id: 'new_folder', handler: startCreatingFolder },
      { label: t('upload_file'), id: 'upload_file', handler: startUploadingDocOrFile },
    ]
  }, [canCreate, canWrite, t, startCreatingDocOrFile, startCreatingFolder, startUploadingDocOrFile])

  if (!canWrite) {
    return null
  }

  return (
    <div className="file-tree-toolbar-action-buttons">
      {fileTreeExpanded && (
        <>
          {canCreate && (
            <FileTreeActionButton
              id="new-file"
              description={t('new_file')}
              onClick={startCreatingDocOrFile}
              iconType="note_add"
            />
          )}
          {canCreate && (
            <FileTreeActionButton
              id="new-folder"
              description={t('new_folder')}
              onClick={startCreatingFolder}
              iconType="create_new_folder"
            />
          )}
          {canCreate && (
            <FileTreeActionButton
              id="upload"
              description={t('upload')}
              onClick={startUploadingDocOrFile}
              iconType="upload"
            />
          )}
          {canBulkDelete && (
            <FileTreeActionButton
              id="delete"
              description={t('delete')}
              onClick={startDeleting}
              iconType="delete"
            />
          )}
        </>
      )}
      <FileTreeActionButton
        id="close"
        description={t('close')}
        onClick={handlePaneCollapse}
        iconType="close"
      />
    </div>
  )
}

export default FileTreeActionButtons
