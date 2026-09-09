'use client'

import type { EditorState } from '@codemirror/state'
import { searchPanelOpen } from '@codemirror/search'
import * as commands from '../../extensions/toolbar/commands'
import { ToolbarButton } from './toolbar-button'
import { useTranslation } from '@/lib/i18n'
import { isMac } from '@/lib/os'

export const ToggleSearchButton = ({ state }: { state: EditorState }) => {
  const { t } = useTranslation()

  return (
    <ToolbarButton
      id="toolbar-toggle-search"
      label={t('toolbar_search_file')}
      command={commands.toggleSearch}
      active={searchPanelOpen(state)}
      icon="search"
      shortcut={isMac ? '⌘F' : 'Ctrl+F'}
    />
  )
}
