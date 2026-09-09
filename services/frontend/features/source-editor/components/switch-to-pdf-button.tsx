'use client'

import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { Button } from '@/components/ol/button'
import { useLayout } from '@/features/ide/contexts/layout-context'

function SwitchToPDFButton() {
  const { pdfLayout, setView, detachRole } = useLayout()

  const { t } = useTranslation()

  if (detachRole) {
    return null
  }

  if (pdfLayout === 'sideBySide') {
    return null
  }

  function handleClick() {
    setView('pdf')
  }

  return (
    <Button variant="secondary" size="sm" onClick={handleClick}>
      <MaterialIcon type="picture_as_pdf" />
      {t('switch_to_pdf')}
    </Button>
  )
}

export default SwitchToPDFButton
