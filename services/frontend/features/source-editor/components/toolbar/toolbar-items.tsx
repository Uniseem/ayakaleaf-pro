'use client'

import { memo } from 'react'
import type { EditorState } from '@codemirror/state'
import { redo, undo } from '@codemirror/commands'
import { ToolbarButton } from './toolbar-button'
import * as commands from '../../extensions/toolbar/commands'
import { SectionHeadingDropdown } from './section-heading-dropdown'
import { InsertFigureDropdown } from './insert-figure-dropdown'
import { useTranslation } from '@/lib/i18n'
import { MathDropdown } from './math-dropdown'
import { InsertListDropdown } from './insert-list-dropdown'
import { LegacyTableDropdown } from './table-inserter-dropdown-legacy'
import { withinFormattingCommand } from '../../utils/tree-operations/formatting'
import { isMac } from '@/lib/os'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditorPropertiesContext } from '@/features/ide/contexts/editor-properties-context'
import { useSite } from '@/features/ide/contexts/site-context'
import { isCursorOnEmptyLine } from '../../utils/misc'

const addCommentFromToolbar = () => commands.addComment('toolbar')

export const ToolbarItems = memo(function ToolbarItems({
  state,
  overflowed,
  languageName,
  visual,
  listDepth,
}: {
  state: EditorState
  overflowed?: Set<string>
  languageName?: string
  visual: boolean
  listDepth: number
}) {
  const { t } = useTranslation()
  const { showSymbolPalette, toggleSymbolPalette } = useEditorPropertiesContext()
  const { features, canWrite, canReview } = useProject()
  const { symbolPaletteAvailable } = useSite()
  const isActive = withinFormattingCommand(state)

  const canComment = canWrite || canReview

  const showGroup = (group: string) => !overflowed || overflowed.has(group)

  return (
    <>
      {showGroup('group-history') && (
        <div className="ol-cm-toolbar-button-group" aria-label={t('toolbar_undo_redo_actions')}>
          <ToolbarButton id="toolbar-undo" label={t('toolbar_undo')} command={undo} icon="undo" shortcut={isMac ? '⌘Z' : 'Ctrl+Z'} />
          <ToolbarButton id="toolbar-redo" label={t('toolbar_redo')} command={redo} icon="redo" shortcut={isMac ? '⇧⌘Z' : 'Ctrl+Y'} />
        </div>
      )}
      {languageName === 'latex' && (
        <>
          {showGroup('group-section') && (
            <div className="ol-cm-toolbar-button-group" data-overflow="group-section" aria-label={t('toolbar_text_formatting')}>
              <SectionHeadingDropdown />
            </div>
          )}
          {showGroup('group-format') && (
            <div className="ol-cm-toolbar-button-group" aria-label={t('toolbar_text_style')}>
              <ToolbarButton
                id="toolbar-format-bold"
                label={t('toolbar_bold')}
                command={commands.toggleBold}
                active={isActive('\\textbf')}
                icon="format_bold"
                shortcut={isMac ? '⌘B' : 'Ctrl+B'}
              />
              <ToolbarButton
                id="toolbar-format-italic"
                label={t('toolbar_italic')}
                command={commands.toggleItalic}
                active={isActive('\\textit')}
                icon="format_italic"
                shortcut={isMac ? '⌘I' : 'Ctrl+I'}
              />
            </div>
          )}
          {showGroup('group-math') && (
            <div className="ol-cm-toolbar-button-group" data-overflow="group-math" aria-label={t('toolbar_insert_math_and_symbols')}>
              <MathDropdown />
              {symbolPaletteAvailable && (
                <ToolbarButton
                  id="toolbar-toggle-symbol-palette"
                  label={t('toolbar_insert_symbol')}
                  active={showSymbolPalette}
                  command={toggleSymbolPalette}
                  icon="Ω"
                  textIcon
                  className="ol-cm-toolbar-button-math"
                />
              )}
            </div>
          )}
          {showGroup('group-misc') && (
            <div className="ol-cm-toolbar-button-group" data-overflow="group-misc" aria-label={t('toolbar_insert_misc')}>
              <ToolbarButton id="toolbar-href" label={t('toolbar_insert_link')} command={commands.wrapInHref} icon="add_link" />
              {features.trackChangesVisible && canComment && (
                <ToolbarButton
                  id="toolbar-add-comment"
                  label={t('add_comment')}
                  disabled={isCursorOnEmptyLine(state)}
                  command={addCommentFromToolbar}
                  icon="add_comment"
                />
              )}
              <ToolbarButton id="toolbar-ref" label={t('toolbar_insert_cross_reference')} command={commands.insertRef} icon="sell" />
              <ToolbarButton id="toolbar-cite" label={t('toolbar_insert_citation')} command={commands.insertCite} icon="book_5" />
              <InsertFigureDropdown />
              <LegacyTableDropdown />
            </div>
          )}
          {showGroup('group-list') && (
            <div className="ol-cm-toolbar-button-group" data-overflow="group-list" aria-label={t('toolbar_list_indentation')}>
              <InsertListDropdown />
              {listDepth >= 1 && (
                <>
                  <ToolbarButton
                    id="toolbar-format-indent-decrease"
                    label={t('toolbar_decrease_indent')}
                    command={commands.indentDecrease}
                    icon="format_indent_decrease"
                    shortcut={visual ? (isMac ? '⌘[' : 'Ctrl+[') : undefined}
                    disabled={listDepth < 2}
                  />
                  <ToolbarButton
                    id="toolbar-format-indent-increase"
                    label={t('toolbar_increase_indent')}
                    command={commands.indentIncrease}
                    icon="format_indent_increase"
                    shortcut={visual ? (isMac ? '⌘]' : 'Ctrl+]') : undefined}
                  />
                </>
              )}
            </div>
          )}
        </>
      )}
    </>
  )
})
