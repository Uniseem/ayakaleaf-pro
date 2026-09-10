'use client'

import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import type { AvailableUnfilledIcon } from '@/lib/unfilled-symbols'

/** One icon button in the file tree's header. */
export function FileTreeActionButton({
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
      <button className="btn file-tree-toolbar-action-button" onClick={onClick}>
        <MaterialIcon unfilled type={iconType} accessibilityLabel={description} />
      </button>
    </Tooltip>
  )
}

export default FileTreeActionButton
