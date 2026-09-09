'use client'

import { useMemo } from 'react'
import { useCodeMirrorStateContext, useCodeMirrorViewContext } from '../codemirror-context'
import { findCurrentSectionHeadingLevel, setSectionHeadingLevel } from '../../extensions/toolbar/sections'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'
import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'
import { ToolbarButtonMenu } from './button-menu'
import { ListGroupItem } from '@/components/ol/list-group'

const levels = new Map([
  ['text', 'Normal text'],
  ['section', 'Section'],
  ['subsection', 'Subsection'],
  ['subsubsection', 'Subsubsection'],
  ['paragraph', 'Paragraph'],
  ['subparagraph', 'Subparagraph'],
])

const levelsEntries = [...levels.entries()]

export const SectionHeadingDropdown = () => {
  const state = useCodeMirrorStateContext()
  const view = useCodeMirrorViewContext()
  const { t } = useTranslation()

  const currentLevel = useMemo(() => findCurrentSectionHeadingLevel(state), [state])

  return (
    <ToolbarButtonMenu
      id="section-heading-menu-button"
      label={t('toolbar_section_heading_level')}
      className="ol-cm-toolbar-button-wide"
      icon={
        <>
          <MaterialIcon type="text_fields" style={{ transform: 'scaleX(-1)' }} />
          <MaterialIcon type="expand_more" />
        </>
      }
    >
      {levelsEntries.map(([level, label]) => (
        <ListGroupItem
          role="menuitem"
          key={level}
          active={level === currentLevel?.level}
          onClick={() => {
            emitToolbarEvent(view, 'section-level-change')
            setSectionHeadingLevel(view, level)
            view.focus()
          }}
          className={`ol-cm-section-heading-menu-item section-level-${level}`}
        >
          {label}
        </ListGroupItem>
      ))}
    </ToolbarButtonMenu>
  )
}
