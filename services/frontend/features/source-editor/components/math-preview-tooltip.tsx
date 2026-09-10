'use client'

import { FC, useCallback, useState } from 'react'
import ReactDOM from 'react-dom'
import { getTooltip } from '@codemirror/view'
import { Trans, useTranslation } from '@/lib/i18n'
import { Dropdown, DropdownMenu, DropdownToggle, DropdownItem } from '@/components/ol/dropdown'
import { Button as OLButton } from '@/components/ol/button'
import {
  OLModal,
  OLModalBody,
  OLModalFooter,
  OLModalHeader,
  OLModalTitle,
} from '@/components/ol/modal'
import MaterialIcon from '@/components/ol/material-icon'
import { useEventListener } from '@/lib/hooks'
import { useSettings } from '@/features/ide/contexts/settings-context'
import {
  useCodeMirrorStateContext,
  useCodeMirrorViewContext,
} from './codemirror-context'
import { mathPreviewStateField } from '../extensions/math-preview'

/**
 * The menu on the equation preview.
 *
 * Rendered into the tooltip the extension built rather than beside it: the
 * tooltip is positioned by CodeMirror against the equation, and a second
 * element would have to be positioned all over again.
 */
const MathPreviewTooltipContainer: FC = () => {
  const state = useCodeMirrorStateContext()
  const view = useCodeMirrorViewContext()

  const mathPreviewState = state.field(mathPreviewStateField, false)

  if (!mathPreviewState) {
    return null
  }

  const { tooltip } = mathPreviewState

  if (!tooltip) {
    return null
  }

  const tooltipView = getTooltip(view, tooltip)

  if (!tooltipView) {
    return null
  }

  const inner = tooltipView.dom.querySelector('#ol-cm-math-tooltip')

  if (!inner) {
    return null
  }

  return ReactDOM.createPortal(<MathPreviewTooltipMenu />, inner)
}

const MathPreviewTooltipMenu: FC = () => {
  const { t } = useTranslation()

  const [showDisableModal, setShowDisableModal] = useState(false)
  const { set } = useSettings()
  const openDisableModal = useCallback(() => setShowDisableModal(true), [])
  const closeDisableModal = useCallback(() => setShowDisableModal(false), [])

  const onHide = useCallback(() => {
    window.dispatchEvent(new Event('editor:hideMathTooltip'))
  }, [])

  const keyDownListener = useCallback(
    (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onHide()
      }
    },
    [onHide]
  )

  useEventListener('keydown', keyDownListener)

  return (
    <>
      <Dropdown align="end">
        <DropdownToggle
          id="math-preview-options"
          className="math-tooltip-options-toggle"
          variant="secondary"
          size="sm"
        >
          <MaterialIcon type="more_vert" accessibilityLabel={t('more_options')} />
        </DropdownToggle>
        <DropdownMenu flip={false}>
          <DropdownItem
            onClick={onHide}
            description={t('temporarily_hides_the_preview')}
            trailingIcon={
              <span className="math-tooltip-options-keyboard-shortcut">Esc</span>
            }
          >
            {t('hide')}
          </DropdownItem>
          <DropdownItem
            onClick={openDisableModal}
            description={t('permanently_disables_the_preview')}
          >
            {t('disable')}
          </DropdownItem>
        </DropdownMenu>
      </Dropdown>

      {showDisableModal && (
        <OLModal show onHide={closeDisableModal}>
          <OLModalHeader>
            <OLModalTitle>{t('disable_equation_preview')}</OLModalTitle>
          </OLModalHeader>

          <OLModalBody>
            {t('disable_equation_preview_confirm')}
            <br />
            <Trans
              i18nKey="disable_equation_preview_enable_in_settings"
              components={{ b: <strong /> }}
            />
          </OLModalBody>

          <OLModalFooter>
            <OLButton variant="secondary" onClick={closeDisableModal}>
              {t('cancel')}
            </OLButton>
            <OLButton variant="danger" onClick={() => set('mathPreview', false)}>
              {t('disable')}
            </OLButton>
          </OLModalFooter>
        </OLModal>
      )}
    </>
  )
}

export default MathPreviewTooltipContainer
