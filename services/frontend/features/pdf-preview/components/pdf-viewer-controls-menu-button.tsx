'use client'

import { useRef } from 'react'
import PdfPageNumberControl from './pdf-page-number-control'
import PdfZoomButtons from './pdf-zoom-buttons'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import useDropdown from '@/features/source-editor/hooks/use-dropdown'
import Tooltip from '@/components/ol/tooltip'
import { Button } from '@/components/ol/button'
import { Overlay, Popover } from '@/components/ol/overlay'

type PdfViewerControlsMenuButtonProps = {
  setZoom: (zoom: string) => void
  setPage: (page: number) => void
  page: number
  totalPages: number
  pdfContainer?: HTMLDivElement
}

/** When the toolbar is too narrow, the page and zoom buttons behind a menu. */
export default function PdfViewerControlsMenuButton({
  setZoom,
  setPage,
  page,
  totalPages,
  pdfContainer,
}: PdfViewerControlsMenuButtonProps) {
  const { t } = useTranslation()

  const { open: popoverOpen, onToggle: togglePopover, ref: popoverRef } = useDropdown()

  const targetRef = useRef<HTMLButtonElement | null>(null)

  return (
    <>
      <Tooltip id="pdf-controls-menu-tooltip" description={t('view_options')} overlayProps={{ placement: 'bottom' }}>
        <Button
          variant="ghost"
          className="pdf-toolbar-btn pdfjs-toolbar-popover-button"
          onClick={() => togglePopover(!popoverOpen)}
          ref={targetRef}
        >
          <MaterialIcon type="more_horiz" />
        </Button>
      </Tooltip>

      <Overlay
        show={popoverOpen}
        target={targetRef.current}
        placement="bottom"
        container={pdfContainer}
        containerPadding={0}
        rootClose
        onHide={() => togglePopover(false)}
      >
        <Popover className="pdfjs-toolbar-popover" id="pdf-toolbar-popover-menu" ref={popoverRef}>
          <PdfPageNumberControl setPage={setPage} page={page} totalPages={totalPages} />
          <div className="pdfjs-zoom-controls">
            <PdfZoomButtons setZoom={setZoom} />
          </div>
        </Popover>
      </Overlay>
    </>
  )
}
