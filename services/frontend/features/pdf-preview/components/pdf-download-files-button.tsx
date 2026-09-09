'use client'

import { memo } from 'react'
import { Dropdown, DropdownMenu, DropdownToggle } from '@/components/ol/dropdown'
import PdfFileList from './pdf-file-list'
import { useTranslation } from '@/lib/i18n'
import { useCompile } from '@/features/ide/contexts/compile-context'

/** "Other logs and files": a menu of everything the compile produced. */
function PdfDownloadFilesButton() {
  const { compiling, fileList } = useCompile()

  const { t } = useTranslation()

  if (!fileList) {
    return null
  }

  return (
    <Dropdown drop="up">
      <DropdownToggle id="dropdown-files-logs-pane" variant="secondary" size="sm" disabled={compiling || !fileList}>
        {t('other_logs_and_files')}
      </DropdownToggle>
      <DropdownMenu id="dropdown-files-logs-pane-list">
        <PdfFileList fileList={fileList} />
      </DropdownMenu>
    </Dropdown>
  )
}

export default memo(PdfDownloadFilesButton)
