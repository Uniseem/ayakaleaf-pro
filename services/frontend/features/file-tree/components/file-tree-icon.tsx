'use client'

import { useTranslation } from '@/lib/i18n'
import cx from '@/lib/cx'
import MaterialIcon from '@/components/ol/material-icon'
import { iconTypeFromName } from '../util/icon-type-from-name'

/** The icon beside a file's name, plus the mark a linked file carries. */
export function FileTreeIcon({ isLinkedFile, name }: { name: string; isLinkedFile?: boolean }) {
  const { t } = useTranslation()

  const className = cx('file-tree-icon', { 'linked-file-icon': isLinkedFile })

  return (
    <>
      <MaterialIcon unfilled type={iconTypeFromName(name)} className={className} />
      {isLinkedFile && (
        <MaterialIcon
          type="open_in_new"
          modifier="rotate-180"
          className="linked-file-highlight"
          accessibilityLabel={t('linked_file')}
        />
      )}
    </>
  )
}

export default FileTreeIcon
