'use client'

import { useCallback } from 'react'
import { useTranslation, Trans } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import PdfLogEntry from './pdf-log-entry'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useStopOnFirstError } from '../hooks/use-stop-on-first-error'

/** Says the compile stopped at the first error, and offers to stop doing that. */
export default function StopOnFirstErrorPrompt() {
  const { t } = useTranslation()
  const { startCompile, setAnimateCompileDropdownArrow } = useCompile()
  const { disableStopOnFirstError } = useStopOnFirstError()

  const handleDisableButtonClick = useCallback(() => {
    disableStopOnFirstError()
    startCompile({ stopOnFirstError: false })
    setAnimateCompileDropdownArrow(true)
  }, [disableStopOnFirstError, startCompile, setAnimateCompileDropdownArrow])

  return (
    <PdfLogEntry
      headerTitle={t('stop_on_first_error_enabled_title')}
      formattedContent={
        <>
          <Trans i18nKey="stop_on_first_error_enabled_description" components={[<strong key="strong" />]} />{' '}
          <Button variant="primary" size="sm" onClick={handleDisableButtonClick}>
            {t('disable_stop_on_first_error')}
          </Button>
        </>
      }
      level="info"
    />
  )
}
