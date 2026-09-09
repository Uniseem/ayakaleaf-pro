'use client'

/**
 * How this person likes the editor.
 *
 * Every control here is live: it changes the editor as it is moved rather than
 * waiting for a save button. That is what makes choosing a font size possible
 * -- the only way to know which one is right is to see it.
 */

import {
  Card,
  CardBody,
  CardHeader,
  Select,
  SelectItem,
  Slider,
  Switch,
} from '@heroui/react'
import {
  defaultSettings,
  useSettings,
  type UserSettings,
} from '@/features/ide/contexts/settings-context'

/** The named things that are a choice from a list. */
const CHOICES: Array<{
  key: keyof UserSettings
  label: string
  description?: string
  options: Array<{ value: string; label: string }>
}> = [
  {
    key: 'overallTheme',
    label: 'Theme',
    options: [
      { value: 'light', label: 'Light' },
      { value: 'dark', label: 'Dark' },
    ],
  },
  {
    key: 'fontFamily',
    label: 'Font',
    options: [
      { value: 'monaco', label: 'Monaco / Menlo / Consolas' },
      { value: 'lucida', label: 'Lucida / Source Code Pro' },
      { value: 'opendyslexicmono', label: 'OpenDyslexic Mono' },
    ],
  },
  {
    key: 'lineHeight',
    label: 'Line spacing',
    options: [
      { value: 'compact', label: 'Compact' },
      { value: 'normal', label: 'Normal' },
      { value: 'wide', label: 'Wide' },
    ],
  },
  {
    key: 'keybindings',
    label: 'Keybindings',
    description: 'Vim and Emacs change what most keys do.',
    options: [
      { value: 'default', label: 'Default' },
      { value: 'vim', label: 'Vim' },
      { value: 'emacs', label: 'Emacs' },
    ],
  },
  {
    key: 'pdfViewer',
    label: 'PDF viewer',
    options: [
      { value: 'pdfjs', label: 'Built in' },
      { value: 'native', label: "The browser's" },
    ],
  },
]

/** The things that are simply on or off. */
const SWITCHES: Array<{
  key: keyof UserSettings
  label: string
  description?: string
}> = [
  {
    key: 'autoComplete',
    label: 'Autocomplete',
    description: 'Suggest commands, environments and this document’s own labels.',
  },
  {
    key: 'autoPairDelimiters',
    label: 'Close brackets',
    description: 'Typing { adds the closing one.',
  },
  {
    key: 'syntaxValidation',
    label: 'Check syntax',
    description: 'Mark an unclosed environment before you compile.',
  },
  {
    key: 'mathPreview',
    label: 'Maths preview',
  },
  {
    key: 'showOutline',
    label: 'Show the outline',
    description: 'The document’s headings, beside the file tree.',
  },
  {
    key: 'breadcrumbs',
    label: 'Show breadcrumbs',
  },
  {
    key: 'nonBlinkingCursor',
    label: 'Stop the cursor blinking',
  },
  {
    key: 'darkModePdf',
    label: 'Dim the PDF in the dark theme',
  },
]

export function EditorSettings() {
  const settings = useSettings()

  return (
    <Card>
      <CardHeader className="flex-col items-start gap-0.5">
        <h2 className="text-base font-semibold">Editor</h2>
        <p className="text-xs text-default-500">
          These follow you between machines. They change as you set them.
        </p>
      </CardHeader>
      <CardBody className="gap-5">
        <div className="grid gap-4 sm:grid-cols-2">
          {CHOICES.map(choice => (
            <Select
              key={choice.key}
              label={choice.label}
              description={choice.description}
              size="sm"
              selectedKeys={[String(settings[choice.key])]}
              onSelectionChange={keys => {
                const chosen = [...keys][0]
                if (chosen !== undefined) {
                  settings.set(
                    choice.key,
                    String(chosen) as UserSettings[typeof choice.key]
                  )
                }
              }}
            >
              {choice.options.map(option => (
                <SelectItem key={option.value}>{option.label}</SelectItem>
              ))}
            </Select>
          ))}
        </div>

        <div>
          <Slider
            label="Font size"
            size="sm"
            minValue={8}
            maxValue={30}
            step={1}
            value={settings.fontSize}
            getValue={value => `${value}px`}
            onChange={value => {
              if (typeof value === 'number') {
                settings.set('fontSize', value)
              }
            }}
          />
          <p
            className="mt-2 rounded border border-divider bg-default-50 px-3 py-2"
            style={{
              fontSize: `${settings.fontSize}px`,
              fontFamily:
                settings.fontFamily === 'monaco'
                  ? "Monaco, Menlo, 'Ubuntu Mono', Consolas, monospace"
                  : settings.fontFamily === 'lucida'
                    ? "'Lucida Console', 'Source Code Pro', monospace"
                    : "'OpenDyslexic Mono', monospace",
              lineHeight:
                settings.lineHeight === 'compact'
                  ? 1.33
                  : settings.lineHeight === 'wide'
                    ? 2
                    : 1.6,
            }}
          >
            {'\\section{Introduction}'}
            <br />
            {'The quick brown fox jumps over the lazy dog.'}
          </p>
        </div>

        <ul className="flex flex-col gap-3">
          {SWITCHES.map(each => (
            <li key={each.key} className="flex items-start justify-between gap-4">
              <div>
                <p className="text-sm">{each.label}</p>
                {each.description ? (
                  <p className="text-xs text-default-500">{each.description}</p>
                ) : null}
              </div>
              <Switch
                size="sm"
                aria-label={each.label}
                isSelected={Boolean(settings[each.key])}
                onValueChange={on => settings.set(each.key, on as never)}
              />
            </li>
          ))}
        </ul>

        <button
          type="button"
          className="self-start text-xs text-default-500 underline underline-offset-2 hover:text-foreground"
          onClick={() => settings.reset()}
        >
          Put everything back to {Object.keys(defaultSettings).length} defaults
        </button>
      </CardBody>
    </Card>
  )
}
