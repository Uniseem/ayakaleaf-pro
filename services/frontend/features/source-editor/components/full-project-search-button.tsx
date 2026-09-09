'use client'

import { useCallback } from 'react'
import { closeSearchPanel, type SearchQuery } from '@codemirror/search'
import { Button } from '@/components/ol/button'
import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import { useTranslation } from '@/lib/i18n'
import { useCodeMirrorViewContext } from './codemirror-context'
import { useRailContext } from '@/features/ide/contexts/rail-context'

export const FullProjectSearchButton = ({ query }: { query: SearchQuery }) => {
  const view = useCodeMirrorViewContext()
  const { t } = useTranslation()
  const { openTab } = useRailContext()

  const openFullProjectSearch = useCallback(() => {
    openTab('full-project-search')

    closeSearchPanel(view)
    window.setTimeout(() => {
      window.dispatchEvent(new CustomEvent('editor:full-project-search', { detail: query }))
    }, 200)
  }, [query, view, openTab])

  return (
    <>
      <Tooltip id="open-full-project-search" overlayProps={{ placement: 'top' }} description={t('search_all_project_files')}>
        <Button variant="ghost" size="sm" onClick={openFullProjectSearch}>
          <MaterialIcon type="manage_search" accessibilityLabel={t('search_all_project_files')} />
        </Button>
      </Tooltip>
    </>
  )
}
