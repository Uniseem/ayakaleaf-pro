'use client'

import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'

/** The arrow that shows whether a folder is open. */
export function FileTreeFolderIcons({ expanded }: { expanded: boolean }) {
  const { t } = useTranslation()

  return (
    <div className="folder-expand-collapse-button" aria-label={expanded ? t('collapse') : t('expand')}>
      <MaterialIcon type={expanded ? 'expand_more' : 'chevron_right'} className="file-tree-expand-icon" />
    </div>
  )
}

export default FileTreeFolderIcons
