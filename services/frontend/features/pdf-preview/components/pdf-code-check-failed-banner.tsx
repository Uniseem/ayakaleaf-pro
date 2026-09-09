'use client'

import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'
import { useCompile } from '@/features/ide/contexts/compile-context'

/** Why the PDF is not being rebuilt: the linter found an error. */
export default function PdfCodeCheckFailedBanner() {
  const { t } = useTranslation()

  const { codeCheckFailed } = useCompile()

  if (!codeCheckFailed) {
    return null
  }

  return (
    <div className="pdf-code-check-failed-banner-container">
      <div className="pdf-code-check-failed-banner">
        <MaterialIcon unfilled type="picture_as_pdf" />
        {t('code_check_failed_explanation')}
      </div>
    </div>
  )
}
