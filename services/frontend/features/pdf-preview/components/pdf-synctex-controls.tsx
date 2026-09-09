'use client'

import { memo, useCallback, useMemo } from 'react'
import cx from '@/lib/cx'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useTranslation } from '@/lib/i18n'
import Tooltip, { type Placement } from '@/components/ol/tooltip'
import { Button } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'
import useSynctex from '../hooks/use-synctex'
import { Spinner } from '@/components/ol/spinner'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'

/**
 * The two arrows between the editor and the PDF: code to PDF, and PDF to
 * code. From pdf-preview/components/pdf-synctex-controls.
 */

const GoToCodeButton = memo(function GoToCodeButton({
  syncToCode,
  syncToCodeInFlight,
  isDetachLayout,
}: {
  syncToCode: ({ visualOffset }: { visualOffset: number }) => void
  syncToCodeInFlight: boolean
  isDetachLayout?: boolean
}) {
  const { t } = useTranslation()
  useCommandProvider(
    () => [
      {
        id: 'synctex-sync-to-code',
        handler: () => {
          syncToCode({ visualOffset: 72 })
        },
        disabled: syncToCodeInFlight,
        label: t('go_to_pdf_location_in_code_action'),
      },
    ],
    [t, syncToCode, syncToCodeInFlight]
  )

  const buttonClasses = cx('synctex-control', {
    'detach-synctex-control': !!isDetachLayout,
  })

  let buttonIcon = null
  if (syncToCodeInFlight) {
    buttonIcon = <Spinner size="sm" />
  } else if (!isDetachLayout) {
    buttonIcon = <MaterialIcon type="arrow_left_alt" className="synctex-control-icon" />
  }

  const syncToCodeWithButton = useCallback(() => {
    syncToCode({ visualOffset: 72 })
  }, [syncToCode])

  const overlayProps = useMemo(
    () => ({
      placement: (isDetachLayout ? 'bottom' : 'right') as Placement,
    }),
    [isDetachLayout]
  )

  return (
    <Tooltip id="sync-to-code" description={t('go_to_pdf_location_in_code')} overlayProps={overlayProps}>
      <span>
        <Button
          variant="secondary"
          size="sm"
          onClick={syncToCodeWithButton}
          disabled={syncToCodeInFlight}
          className={buttonClasses}
          aria-label={t('go_to_pdf_location_in_code')}
        >
          {buttonIcon}
          {isDetachLayout ? <span>&nbsp;{t('show_in_code')}</span> : ''}
        </Button>
      </span>
    </Tooltip>
  )
})

const GoToPdfButton = memo(function GoToPdfButton({
  syncToPdf,
  syncToPdfInFlight,
  isDetachLayout,
  canSyncToPdf,
}: {
  syncToPdf: () => void
  syncToPdfInFlight: boolean
  canSyncToPdf: boolean
  isDetachLayout?: boolean
}) {
  const { t } = useTranslation()
  const tooltipPlacement: Placement = isDetachLayout ? 'bottom' : 'right'
  const buttonClasses = cx('synctex-control', {
    'detach-synctex-control': !!isDetachLayout,
  })

  const handleSyncToPdf = useCallback(() => {
    syncToPdf()
  }, [syncToPdf])

  useCommandProvider(
    () => [
      {
        id: 'synctex-sync-to-pdf',
        handler: () => {
          syncToPdf()
        },
        label: t('go_to_code_location_in_pdf'),
        disabled: syncToPdfInFlight || !canSyncToPdf,
      },
    ],
    [t, syncToPdf, syncToPdfInFlight, canSyncToPdf]
  )

  let buttonIcon = null
  if (syncToPdfInFlight) {
    buttonIcon = <Spinner size="sm" />
  } else if (!isDetachLayout) {
    buttonIcon = <MaterialIcon type="arrow_right_alt" className="synctex-control-icon" />
  }

  return (
    <Tooltip id="sync-to-pdf" description={t('go_to_code_location_in_pdf')} overlayProps={{ placement: tooltipPlacement }}>
      <span>
        <Button
          variant="secondary"
          size="sm"
          onClick={handleSyncToPdf}
          disabled={syncToPdfInFlight || !canSyncToPdf}
          className={buttonClasses}
          aria-label={t('go_to_code_location_in_pdf')}
        >
          {buttonIcon}
          {isDetachLayout ? <span>&nbsp;{t('show_in_pdf')}</span> : ''}
        </Button>
      </span>
    </Tooltip>
  )
})

function PdfSynctexControls() {
  const { detachRole } = useLayout()
  const { pdfUrl, pdfViewer, position } = useCompile()
  const { syncToCode, syncToPdf, syncToCodeInFlight, syncToPdfInFlight, canSyncToPdf } = useSynctex()

  if (!position) {
    return null
  }

  if (!pdfUrl || pdfViewer === 'native') {
    return null
  }

  if (detachRole === 'detacher') {
    return <GoToPdfButton syncToPdf={syncToPdf} syncToPdfInFlight={syncToPdfInFlight} isDetachLayout canSyncToPdf={canSyncToPdf} />
  } else if (detachRole === 'detached') {
    return <GoToCodeButton syncToCode={syncToCode} syncToCodeInFlight={syncToCodeInFlight} isDetachLayout />
  } else {
    return (
      <>
        <GoToPdfButton syncToPdf={syncToPdf} syncToPdfInFlight={syncToPdfInFlight} canSyncToPdf={canSyncToPdf} />

        <GoToCodeButton syncToCode={syncToCode} syncToCodeInFlight={syncToCodeInFlight} />
      </>
    )
  }
}

export default memo(PdfSynctexControls)

/** The controls, in the layout where the editor and PDF share the window. */
export function DefaultSynctexControl() {
  const { detachRole } = useLayout()
  if (!detachRole) {
    return <PdfSynctexControls />
  }
  return null
}
