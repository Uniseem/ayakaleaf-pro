'use client'

/**
 * The editor's settings, in the dialog the rail's cog opens.
 *
 * The original's settings modal is a large thing with its own sections; this
 * is the same dialog frame around the settings controls that exist so far,
 * and is where the sections will go.
 */

import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { EditorSettings } from './editor-settings'

export function SettingsModal() {
  const { t } = useTranslation()
  const { leftMenuShown, setLeftMenuShown } = useLayout()

  return (
    <OLModal show={leftMenuShown} onHide={() => setLeftMenuShown(false)} size="lg" id="settings-modal">
      <OLModalHeader>
        <OLModalTitle>{t('settings')}</OLModalTitle>
      </OLModalHeader>
      <OLModalBody>
        <EditorSettings />
      </OLModalBody>
      <OLModalFooter>
        <Button variant="secondary" onClick={() => setLeftMenuShown(false)}>
          {t('close')}
        </Button>
      </OLModalFooter>
    </OLModal>
  )
}
