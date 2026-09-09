'use client'

/**
 * The keyboard shortcuts, from hotkeys-modal/components/hotkeys-modal.tsx.
 */

import { memo } from 'react'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { Trans, useTranslation } from '@/lib/i18n'

export default memo(function HotkeysModal({
  animation = true,
  handleHide,
  show,
  isMac = false,
  trackChangesVisible = false,
}: {
  animation?: boolean
  handleHide: () => void
  show: boolean
  isMac?: boolean
  trackChangesVisible?: boolean
}) {
  const { t } = useTranslation()
  const ctrl = isMac ? 'Cmd' : 'Ctrl'

  return (
    <OLModal size="lg" onHide={handleHide} show={show} animation={animation}>
      <OLModalHeader>
        <OLModalTitle>{t('hotkeys')}</OLModalTitle>
      </OLModalHeader>

      <OLModalBody className="hotkeys-modal ol-ui">
        <h3>{t('common')}</h3>

        <div className="row">
          <div className="col-4">
            <Hotkey combination={`${ctrl} + F`} description={t('hotkey_find_and_replace')} />
            <Hotkey combination={`${ctrl} + Enter`} description={t('hotkey_compile')} />
          </div>
          <div className="col-4">
            <Hotkey combination={`${ctrl} + Z`} description={t('hotkey_undo')} />
          </div>
          <div className="col-4">
            <Hotkey combination={`${ctrl} + Y`} description={t('hotkey_redo')} />
          </div>
        </div>

        <h3>{t('navigation')}</h3>

        <div className="row">
          <div className="col-4">
            <Hotkey combination={`${ctrl} + Home`} description={t('hotkey_beginning_of_document')} />
          </div>
          <div className="col-4">
            <Hotkey combination={`${ctrl} + End`} description={t('hotkey_end_of_document')} />
          </div>
          <div className="col-4">
            <Hotkey combination={`${ctrl} + Shift + L`} description={t('hotkey_go_to_line')} />
          </div>
        </div>

        <h3>{t('editing')}</h3>

        <div className="row">
          <div className="col-4">
            <Hotkey combination={`${ctrl} + /`} description={t('hotkey_toggle_comment')} />
            <Hotkey combination={`${ctrl} + D`} description={t('hotkey_delete_current_line')} />
            <Hotkey combination={`${ctrl} + A`} description={t('hotkey_select_all')} />
          </div>
          <div className="col-4">
            <Hotkey combination="Ctrl + U" description={t('hotkey_to_uppercase')} />
            <Hotkey combination="Ctrl + Shift + U" description={t('hotkey_to_lowercase')} />
            <Hotkey combination="Tab" description={t('hotkey_indent_selection')} />
          </div>
          <div className="col-4">
            <Hotkey combination={`${ctrl} + B`} description={t('hotkey_bold_text')} />
            <Hotkey combination={`${ctrl} + I`} description={t('hotkey_italic_text')} />
          </div>
        </div>

        <h3>{t('autocomplete')}</h3>

        <div className="row">
          <div className="col-4">
            <Hotkey combination="Ctrl + Space" description={t('hotkey_autocomplete_menu')} />
          </div>
          <div className="col-4">
            <Hotkey combination="Up / Down" description={t('hotkey_select_candidate')} />
          </div>
          <div className="col-4">
            <Hotkey combination="Enter / Tab" description={t('hotkey_insert_candidate')} />
          </div>
        </div>

        <h3>
          <Trans i18nKey="autocomplete_references" components={{ code: <code /> }} />
        </h3>

        <div className="row">
          <div className="col-4">
            <Hotkey combination="Ctrl + Space " description={t('hotkey_search_references')} />
          </div>
        </div>

        {trackChangesVisible ? (
          <>
            <h3>{t('review')}</h3>

            <div className="row">
              <div className="col-4">
                <Hotkey combination={`${ctrl} + J`} description={t('hotkey_toggle_review_panel')} />
              </div>
              <div className="col-4">
                <Hotkey combination={`${ctrl} + Shift + A`} description={t('hotkey_toggle_track_changes')} />
              </div>
              <div className="col-4">
                <Hotkey combination={`${ctrl} + Shift + C`} description={t('hotkey_add_a_comment')} />
              </div>
            </div>
          </>
        ) : null}

        <div className="hotkeys-modal-bottom-text">
          <Trans
            i18nKey="a_more_comprehensive_list_of_keyboard_shortcuts"
            values={{ appName: 'Overleaf' }}
            components={[
              // eslint-disable-next-line jsx-a11y/anchor-has-content, react/jsx-key
              <a href="https://www.overleaf.com/articles/overleaf-keyboard-shortcuts/qykqfvmxdnjf" target="_blank" rel="noreferrer" />,
            ]}
          />
        </div>
      </OLModalBody>

      <OLModalFooter>
        <Button variant="secondary" onClick={handleHide}>
          {t('close')}
        </Button>
      </OLModalFooter>
    </OLModal>
  )
})

function Hotkey({ combination, description }: { combination: string; description: string }) {
  return (
    <div className="hotkey" data-test-selector="hotkey">
      <span className="combination">{combination}</span>
      <span className="description">{description}</span>
    </div>
  )
}
