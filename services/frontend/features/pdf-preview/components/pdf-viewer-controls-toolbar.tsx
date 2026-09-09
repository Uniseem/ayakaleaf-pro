'use client'

import { memo, useCallback, useState } from 'react'
import { createPortal } from 'react-dom'
import PdfPageNumberControl from './pdf-page-number-control'
import PdfZoomButtons from './pdf-zoom-buttons'
import PdfZoomDropdown from './pdf-zoom-dropdown'
import { useResizeObserver } from '@/features/source-editor/hooks/use-resize-observer'
import PdfViewerControlsMenuButton from './pdf-viewer-controls-menu-button'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { PdfHybridThemeButton } from './pdf-hybrid-theme-button'

type PdfViewerControlsToolbarProps = {
  requestPresentationMode: () => void
  setZoom: (zoom: string) => void
  rawScale: number
  setPage: (page: number) => void
  page: number
  totalPages: number
  pdfContainer?: HTMLDivElement
}

/**
 * The page and zoom controls, put into the toolbar's right-hand slot from
 * inside the viewer, which is the only thing that knows the page count and
 * the scale. Below a certain width they fold into a menu.
 */
function PdfViewerControlsToolbar({
  requestPresentationMode,
  setZoom,
  rawScale,
  setPage,
  page,
  totalPages,
  pdfContainer,
}: PdfViewerControlsToolbarProps) {
  const { t } = useTranslation()
  const { showLogs } = useCompile()

  const toolbarControlsElement = typeof document !== 'undefined' ? document.querySelector('#toolbar-pdf-controls') : null

  const [availableWidth, setAvailableWidth] = useState<number>(1000)

  const handleResize = useCallback(
    (element: Element) => {
      setAvailableWidth((element as HTMLElement).offsetWidth)
    },
    [setAvailableWidth]
  )

  const { elementRef: pdfControlsRef } = useResizeObserver(handleResize)

  const { view: ideView, pdfLayout } = useLayout()
  const editorOnly = ideView !== 'pdf' && pdfLayout === 'flat'

  useCommandProvider(() => {
    if (editorOnly) {
      return
    }

    return [
      {
        id: 'view-pdf-presentation-mode',
        label: t('presentation_mode'),
        handler: requestPresentationMode,
      },
      {
        id: 'view-pdf-zoom-in',
        label: t('zoom_in'),
        handler: () => setZoom('zoom-in'),
      },
      {
        id: 'view-pdf-zoom-out',
        label: t('zoom_out'),
        handler: () => setZoom('zoom-out'),
      },
      {
        id: 'view-pdf-fit-width',
        label: t('fit_to_width'),
        handler: () => setZoom('page-width'),
      },
      {
        id: 'view-pdf-fit-height',
        label: t('fit_to_height'),
        handler: () => setZoom('page-height'),
      },
    ]
  }, [t, requestPresentationMode, setZoom, editorOnly])

  if (!toolbarControlsElement) {
    return null
  }

  if (showLogs) {
    return null
  }

  const InnerControlsComponent = availableWidth >= 320 ? PdfViewerControlsToolbarFull : PdfViewerControlsToolbarSmall

  return createPortal(
    <div className="pdfjs-viewer-controls" ref={pdfControlsRef}>
      <InnerControlsComponent
        requestPresentationMode={requestPresentationMode}
        setZoom={setZoom}
        rawScale={rawScale}
        setPage={setPage}
        page={page}
        totalPages={totalPages}
        pdfContainer={pdfContainer}
      />
    </div>,

    toolbarControlsElement
  )
}

type InnerControlsProps = {
  requestPresentationMode: () => void
  setZoom: (zoom: string) => void
  rawScale: number
  setPage: (page: number) => void
  page: number
  totalPages: number
  pdfContainer?: HTMLDivElement
}

function PdfViewerControlsToolbarFull({ requestPresentationMode, setZoom, rawScale, setPage, page, totalPages }: InnerControlsProps) {
  return (
    <>
      <PdfHybridThemeButton />
      <PdfPageNumberControl setPage={setPage} page={page} totalPages={totalPages} />
      <div className="pdfjs-zoom-controls">
        <PdfZoomButtons setZoom={setZoom} />
        <PdfZoomDropdown requestPresentationMode={requestPresentationMode} rawScale={rawScale} setZoom={setZoom} />
      </div>
    </>
  )
}

function PdfViewerControlsToolbarSmall({
  requestPresentationMode,
  setZoom,
  rawScale,
  setPage,
  page,
  totalPages,
  pdfContainer,
}: InnerControlsProps) {
  return (
    <div className="pdfjs-viewer-controls-small">
      <PdfHybridThemeButton />
      <PdfZoomDropdown requestPresentationMode={requestPresentationMode} rawScale={rawScale} setZoom={setZoom} />
      <PdfViewerControlsMenuButton setZoom={setZoom} setPage={setPage} page={page} totalPages={totalPages} pdfContainer={pdfContainer} />
    </div>
  )
}

export default memo(PdfViewerControlsToolbar)
