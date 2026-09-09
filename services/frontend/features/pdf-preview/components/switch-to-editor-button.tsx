'use client'

import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { Button } from '@/components/ol/button'
import { useLayout } from '@/features/ide/contexts/layout-context'

/** The way back to the editor when the PDF has the whole width. */
function SwitchToEditorButton() {
  const { pdfLayout, restoreView, detachRole } = useLayout()

  const { t } = useTranslation()

  if (detachRole) {
    return null
  }

  if (pdfLayout === 'sideBySide') {
    return null
  }

  function handleClick() {
    restoreView()
    window.setTimeout(() => {
      window.dispatchEvent(new Event('editor:focus'))
    })
  }

  return (
    <Button variant="secondary" size="sm" onClick={handleClick}>
      <MaterialIcon type="code" />
      {t('switch_to_editor')}
    </Button>
  )
}

export default SwitchToEditorButton
