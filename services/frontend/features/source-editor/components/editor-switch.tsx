'use client'

import { memo, useCallback, useId, type ChangeEvent } from 'react'
import { Tooltip } from '@/components/ol/tooltip'
import { useTranslation } from '@/lib/i18n'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useEditorPropertiesContext } from '@/features/ide/contexts/editor-properties-context'
import { isVisualEditorAvailable } from '../utils/misc'

function EditorSwitch() {
  const { t } = useTranslation()
  const { showVisual: visual, setShowVisual: setVisual } = useEditorPropertiesContext()
  const openDocName = useEditor().current?.name
  const inputId = useId()

  const richTextAvailable = openDocName ? isVisualEditorAvailable(openDocName) : false

  const handleChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      const editorType = event.target.value

      switch (editorType) {
        case 'cm6':
          setVisual(false)
          break

        case 'rich-text':
          setVisual(true)
          break
      }
    },
    [setVisual]
  )

  return (
    <div className="editor-toggle-switch" aria-label={t('toolbar_code_visual_editor_switch')}>
      <form>
        <fieldset className="toggle-switch">
          <legend className="visually-hidden">Editor mode.</legend>

          <input
            type="radio"
            name="editor"
            value="cm6"
            id={inputId}
            className="toggle-switch-input"
            checked={!richTextAvailable || !visual}
            onChange={handleChange}
          />
          <label htmlFor={inputId} className="toggle-switch-label">
            <span>{t('code')}</span>
          </label>

          <RichTextToggle checked={richTextAvailable && visual} disabled={!richTextAvailable} handleChange={handleChange} />
        </fieldset>
      </form>
    </div>
  )
}

const RichTextToggle = ({
  checked,
  disabled,
  handleChange,
}: {
  checked: boolean
  disabled: boolean
  handleChange: (event: ChangeEvent<HTMLInputElement>) => void
}) => {
  const { t } = useTranslation()
  const inputId = useId()

  const toggle = (
    <span>
      <input
        type="radio"
        name="editor"
        value="rich-text"
        id={inputId}
        className="toggle-switch-input"
        checked={checked}
        onChange={handleChange}
        disabled={disabled}
      />
      <label htmlFor={inputId} className="toggle-switch-label">
        <span>{t('visual')}</span>
      </label>
    </span>
  )

  if (disabled) {
    return (
      <Tooltip
        description={t('visual_editor_does_not_support_this_file_type')}
        id="rich-text-toggle-tooltip"
        overlayProps={{ placement: 'bottom' }}
        tooltipProps={{ className: 'tooltip-wide' }}
      >
        {toggle}
      </Tooltip>
    )
  }

  return (
    <Tooltip
      id="rich-text-toggle-tooltip"
      description={t('toolbar_change_editor_mode')}
      overlayProps={{ placement: 'bottom' }}
      tooltipProps={{ className: 'tooltip-wide' }}
    >
      {toggle}
    </Tooltip>
  )
}

export default memo(EditorSwitch)
