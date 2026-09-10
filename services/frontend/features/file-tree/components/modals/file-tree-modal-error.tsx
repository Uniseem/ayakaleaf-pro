'use client'

import { useTranslation } from '@/lib/i18n'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { useFileTreeActionable } from '../../contexts/file-tree-actionable'
import {
  BlockedFilenameError,
  DuplicateFilenameError,
  DuplicateFilenameMoveError,
  InvalidFilenameError,
} from '../../errors'
import { messageFor } from '@/lib/api'

/**
 * What went wrong with a rename or a move.
 *
 * Creating has its own dialog with its own input, so the error belongs beside
 * the field. A rename happens inline in the tree, where there is nowhere to
 * put a message, so it gets one of these.
 */
export function FileTreeModalError() {
  const { t } = useTranslation()
  const { isRenaming, isMoving, error, cancel } = useFileTreeActionable()

  if (!error || (!isRenaming && !isMoving)) {
    return null
  }

  function message() {
    if (error instanceof DuplicateFilenameError) {
      return t('file_already_exists')
    }
    if (error instanceof DuplicateFilenameMoveError) {
      return t('file_already_exists_in_this_location')
    }
    if (error instanceof InvalidFilenameError) {
      return t('files_cannot_include_invalid_characters')
    }
    if (error instanceof BlockedFilenameError) {
      return t('blocked_filename')
    }
    return messageFor(error)
  }

  return (
    <OLModal show onHide={cancel}>
      <OLModalHeader>
        <OLModalTitle>{t('error')}</OLModalTitle>
      </OLModalHeader>

      <OLModalBody>
        <div role="alert">{message()}</div>
      </OLModalBody>

      <OLModalFooter>
        <Button variant="primary" onClick={cancel}>
          {t('ok')}
        </Button>
      </OLModalFooter>
    </OLModal>
  )
}

export default FileTreeModalError
