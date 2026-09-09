'use client'

import MaterialIcon from '@/components/ol/material-icon'
import Tooltip from '@/components/ol/tooltip'
import { Button } from '@/components/ol/button'

type PDFToolbarButtonProps = {
  tooltipId: string
  icon: string
  label: string
  onClick: () => void
  shortcut?: string
  disabled?: boolean
}

/** One icon button in the viewer's controls, with its name in a tooltip. */
export default function PDFToolbarButton({ tooltipId, disabled, label, icon, onClick, shortcut }: PDFToolbarButtonProps) {
  return (
    <Tooltip
      id={tooltipId}
      description={
        <>
          <div>{label}</div>
          {shortcut && <div>{shortcut}</div>}
        </>
      }
      overlayProps={{ placement: 'bottom' }}
    >
      <Button
        variant="ghost"
        className="pdf-toolbar-btn pdfjs-toolbar-button"
        disabled={disabled}
        onClick={onClick}
        aria-label={label}
      >
        <MaterialIcon type={icon} />
      </Button>
    </Tooltip>
  )
}
