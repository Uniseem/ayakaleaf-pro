'use client'

import { memo } from 'react'
import { ToolbarButtonMenu } from './button-menu'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'
import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'
import { useCodeMirrorViewContext } from '../codemirror-context'
import { wrapInDisplayMath, wrapInInlineMath } from '../../extensions/toolbar/commands'
import { ListGroupItem } from '@/components/ol/list-group'

export const MathDropdown = memo(function MathDropdown() {
  const { t } = useTranslation()
  const view = useCodeMirrorViewContext()

  return (
    <ToolbarButtonMenu id="toolbar-math" label={t('toolbar_insert_math')} icon={<MaterialIcon type="calculate" />}>
      <ListGroupItem
        aria-label={t('toolbar_insert_inline_math')}
        onClick={event => {
          emitToolbarEvent(view, 'toolbar-inline-math')
          event.preventDefault()
          wrapInInlineMath(view)
          view.focus()
        }}
      >
        <MaterialIcon type="123" />
        <span>{t('inline')}</span>
      </ListGroupItem>
      <ListGroupItem
        aria-label={t('toolbar_insert_display_math')}
        onClick={event => {
          emitToolbarEvent(view, 'toolbar-display-math')
          event.preventDefault()
          wrapInDisplayMath(view)
          view.focus()
        }}
      >
        <MaterialIcon type="view_day" />
        <span>{t('display')}</span>
      </ListGroupItem>
    </ToolbarButtonMenu>
  )
})
