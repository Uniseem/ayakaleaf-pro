'use client'

import { memo, useCallback } from 'react'
import { ToolbarButtonMenu } from './button-menu'
import MaterialIcon from '@/components/ol/material-icon'
import { ListGroupItem } from '@/components/ol/list-group'
import { useTranslation } from '@/lib/i18n'
import { emitToolbarEvent } from '../../extensions/toolbar/utils/analytics'
import { useCodeMirrorViewContext } from '../codemirror-context'
import { insertFigure } from '../../extensions/toolbar/commands'
import { useProject } from '@/features/ide/contexts/project-context'
import { useSite } from '@/features/ide/contexts/site-context'
import { ToolbarButton } from './toolbar-button'

/** Where the figure modal takes its image from. */
export enum FigureModalSource {
  NONE = 'none',
  FILE_UPLOAD = 'file_upload',
  FILE_TREE = 'file_tree',
  FROM_URL = 'from_url',
  OTHER_PROJECT = 'other_project',
  EDIT_FIGURE = 'edit_figure',
}

export const InsertFigureDropdown = memo(function InsertFigureDropdown() {
  const { t } = useTranslation()
  const view = useCodeMirrorViewContext()
  const { canWrite: write } = useProject()
  const { hasLinkUrlFeature } = useSite()

  const openFigureModal = useCallback(
    (source: FigureModalSource, sourceName: string) => {
      emitToolbarEvent(view, `toolbar-figure-modal-${sourceName}`)
      window.dispatchEvent(
        new CustomEvent('figure-modal:open', {
          detail: { source },
        })
      )
    },
    [view]
  )

  if (!write) {
    return (
      <ToolbarButton
        id="toolbar-figure"
        label={t('toolbar_insert_figure')}
        command={() => openFigureModal(FigureModalSource.FILE_TREE, 'current-project')}
        icon="add_photo_alternate"
      />
    )
  }

  return (
    <ToolbarButtonMenu
      id="toolbar-figure"
      label={t('toolbar_insert_figure')}
      icon={<MaterialIcon type="add_photo_alternate" />}
      altCommand={insertFigure}
    >
      <ListGroupItem onClick={() => openFigureModal(FigureModalSource.FILE_UPLOAD, 'file-upload')}>
        <MaterialIcon type="upload" />
        {t('upload_from_computer')}
      </ListGroupItem>
      <ListGroupItem onClick={() => openFigureModal(FigureModalSource.FILE_TREE, 'current-project')}>
        <MaterialIcon type="inbox" />
        {t('from_project_files')}
      </ListGroupItem>
      {hasLinkUrlFeature && (
        <ListGroupItem onClick={() => openFigureModal(FigureModalSource.FROM_URL, 'from-url')}>
          <MaterialIcon type="public" />
          {t('from_url')}
        </ListGroupItem>
      )}
    </ToolbarButtonMenu>
  )
})
