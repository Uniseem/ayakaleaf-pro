'use client'

import { memo } from 'react'
import { Button } from '@/components/ol/button'
import { useTranslation } from '@/lib/i18n'
import { useCompile } from '@/features/ide/contexts/compile-context'

/** Throws away the compiler's cached files, for when a stale one is the problem. */
function PdfClearCacheButton() {
  const { compiling, clearCache, clearingCache } = useCompile()

  const { t } = useTranslation()

  return (
    <Button
      size="sm"
      variant="danger"
      className="logs-pane-actions-clear-cache"
      onClick={() => clearCache()}
      isLoading={clearingCache}
      disabled={clearingCache || compiling}
      leadingIcon="delete"
      loadingLabel={t('clear_cached_files')}
    >
      <span>{t('clear_cached_files')}</span>
    </Button>
  )
}

export default memo(PdfClearCacheButton)
