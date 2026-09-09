'use client'

import { memo } from 'react'
import { ToolbarButtonMenu } from './button-menu'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'
import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'
import { useCodeMirrorViewContext } from '../codemirror-context'
import { toggleBulletList, toggleNumberedList } from '../../extensions/toolbar/commands'
import { ListGroupItem } from '@/components/ol/list-group'
import { Tooltip } from '@/components/ol/tooltip'

export const InsertListDropdown = memo(function InsertListDropdown() {
  const { t } = useTranslation()
  const view = useCodeMirrorViewContext()

  return (
    <ToolbarButtonMenu
      id="toolbar-insert-list"
      label={t('toolbar_insert_list')}
      icon={<MaterialIcon type="format_list_bulleted" />}
      orientation="horizontal"
    >
      <Tooltip id="toolbar-bullet-list" description={t('toolbar_bulleted_list')} overlayProps={{ placement: 'bottom' }}>
        <ListGroupItem
          aria-label={t('toolbar_bulleted_list')}
          onClick={event => {
            emitToolbarEvent(view, 'toolbar-bullet-list')
            event.preventDefault()
            toggleBulletList(view)
            view.focus()
          }}
        >
          <MaterialIcon type="format_list_bulleted" />
        </ListGroupItem>
      </Tooltip>
      <Tooltip id="toolbar-numbered-list" description={t('toolbar_numbered_list')} overlayProps={{ placement: 'bottom' }}>
        <ListGroupItem
          aria-label={t('toolbar_numbered_list')}
          onClick={event => {
            emitToolbarEvent(view, 'toolbar-numbered-list')
            event.preventDefault()
            toggleNumberedList(view)
            view.focus()
          }}
        >
          <MaterialIcon type="format_list_numbered" />
        </ListGroupItem>
      </Tooltip>
    </ToolbarButtonMenu>
  )
})
