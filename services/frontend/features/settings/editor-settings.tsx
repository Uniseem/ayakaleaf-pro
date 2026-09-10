'use client'

/**
 * How this person likes the editor, from editor-left-menu's settings.
 *
 * Every control here is live: it changes the editor as it is moved rather than
 * waiting for a save button. That is what makes choosing a font size possible
 * -- the only way to know which one is right is to see it.
 */

import { useTranslation } from '@/lib/i18n'
import { Select } from '@/components/ol/select'
import { Button } from '@/components/ol/button'
import { defaultSettings, useSettings, type UserSettings } from '@/features/ide/contexts/settings-context'

type Option = { value: string; label: string }

type Choice = {
  key: keyof UserSettings
  label: string
  description?: string
  options: Option[]
}

/** The things that are simply on or off. */
type Switch = {
  key: keyof UserSettings
  label: string
  description?: string
}

export function EditorSettings() {
  const { t } = useTranslation()
  const settings = useSettings()

  const choices: Choice[] = [
    {
      key: 'overallTheme',
      label: t('overall_theme'),
      options: [
        { value: 'light', label: t('editor_theme_light') },
        { value: 'dark', label: t('editor_theme_dark') },
      ],
    },
    {
      key: 'fontFamily',
      label: t('editor_font_family'),
      options: [
        { value: 'monaco', label: 'Monaco / Menlo / Consolas' },
        { value: 'lucida', label: 'Lucida / Source Code Pro' },
        { value: 'opendyslexicmono', label: 'OpenDyslexic Mono' },
      ],
    },
    {
      key: 'lineHeight',
      label: t('editor_line_height'),
      options: [
        { value: 'compact', label: t('compact') },
        { value: 'normal', label: t('normal') },
        { value: 'wide', label: t('wide') },
      ],
    },
    {
      key: 'keybindings',
      label: t('keybindings'),
      options: [
        { value: 'default', label: t('off') },
        { value: 'vim', label: 'Vim' },
        { value: 'emacs', label: 'Emacs' },
      ],
    },
    {
      key: 'pdfViewer',
      label: t('pdf_viewer'),
      options: [
        { value: 'pdfjs', label: t('overleaf') },
        { value: 'native', label: t('browser') },
      ],
    },
  ]

  const switches: Switch[] = [
    { key: 'autoComplete', label: t('auto_complete') },
    { key: 'autoPairDelimiters', label: t('auto_close_brackets') },
    { key: 'syntaxValidation', label: t('syntax_checks') },
    { key: 'mathPreview', label: t('math') },
    { key: 'showOutline', label: t('show_outline') },
    { key: 'breadcrumbs', label: t('show_breadcrumbs') },
    {
      key: 'nonBlinkingCursor',
      label: t('non_blinking_cursor'),
      description: t('reduces_visual_distraction_by_keeping_the_cursor_solid'),
    },
    { key: 'darkModePdf', label: t('invert_pdf_preview_colors') },
  ]

  return (
    <div className="settings-entries">
      <div className="settings-group">
        {choices.map(choice => {
          const selected = choice.options.find(option => option.value === String(settings[choice.key]))
          return (
            <div className="settings-entry" key={String(choice.key)}>
              <Select<Option>
                label={choice.label}
                items={choice.options}
                itemToKey={option => option.value}
                itemToString={option => option?.label ?? ''}
                selected={selected ?? null}
                size="sm"
                onSelectedItemChanged={option => {
                  if (option) {
                    settings.set(choice.key, option.value as UserSettings[typeof choice.key])
                  }
                }}
              />
              {choice.description ? <p className="settings-entry-description">{choice.description}</p> : null}
            </div>
          )
        })}

        <div className="settings-entry">
          <label className="form-label" htmlFor="setting-font-size">
            {t('editor_font_size')}
          </label>
          <div className="settings-entry-range">
            <input
              id="setting-font-size"
              type="range"
              className="form-range"
              min={8}
              max={30}
              step={1}
              value={settings.fontSize}
              onChange={event => settings.set('fontSize', Number(event.target.value))}
            />
            <span className="settings-entry-range-value">{settings.fontSize}px</span>
          </div>
          <p
            className="settings-font-preview"
            style={{
              fontSize: `${settings.fontSize}px`,
              fontFamily:
                settings.fontFamily === 'monaco'
                  ? "Monaco, Menlo, 'Ubuntu Mono', Consolas, monospace"
                  : settings.fontFamily === 'lucida'
                    ? "'Lucida Console', 'Source Code Pro', monospace"
                    : "'OpenDyslexic Mono', monospace",
              lineHeight:
                settings.lineHeight === 'compact' ? 1.33 : settings.lineHeight === 'wide' ? 2 : 1.6,
            }}
          >
            {'\\section{Introduction}'}
            <br />
            {'The quick brown fox jumps over the lazy dog.'}
          </p>
        </div>
      </div>

      <ul className="settings-switches list-unstyled">
        {switches.map(each => (
          <li key={String(each.key)} className="settings-switch">
            <label className="settings-switch-label" htmlFor={`setting-${String(each.key)}`}>
              <span>
                {each.label}
                {each.description ? (
                  <span className="settings-entry-description">{each.description}</span>
                ) : null}
              </span>
              <input
                id={`setting-${String(each.key)}`}
                type="checkbox"
                className="form-check-input"
                checked={Boolean(settings[each.key])}
                onChange={event => settings.set(each.key, event.target.checked as never)}
              />
            </label>
          </li>
        ))}
      </ul>

      <Button variant="link" size="sm" className="settings-reset" onClick={() => settings.reset()}>
        {t('restore')}
      </Button>
    </div>
  )
}

export default EditorSettings
