'use client'

import { useTranslation } from '@/lib/i18n'
import { Button } from '@/components/ol/button'
import type { GlobalToastGeneratorEntry } from '@/features/ide/components/global-toasts'

export const SynctexFileErrorToast = () => {
  const { t } = useTranslation()

  return (
    <div className="synctex-error-toast-content">
      <span>{t('synctex_failed')}</span>

      <Button href="/learn/how-to/SyncTeX_Errors" target="_blank" variant="secondary" size="sm">
        {t('more_info')}
      </Button>
    </div>
  )
}

export const SynctexRequestErrorToast = () => {
  const { t } = useTranslation()

  return <span>{t('synctex_error_recompile_and_try_again')}</span>
}

const generators: GlobalToastGeneratorEntry[] = [
  {
    key: 'synctex:file-error',
    generator: () => ({
      content: <SynctexFileErrorToast />,
      type: 'warning',
      autoHide: true,
      delay: 4000,
      isDismissible: true,
    }),
  },
  {
    key: 'synctex:request-error',
    generator: () => ({
      content: <SynctexRequestErrorToast />,
      type: 'warning',
      autoHide: true,
      delay: 4000,
      isDismissible: true,
    }),
  },
]

export default generators

export const showFileErrorToast = () => {
  window.dispatchEvent(
    new CustomEvent('ide:show-toast', {
      detail: {
        key: 'synctex:file-error',
      },
    })
  )
}

export const showSynctexRequestErrorToast = () => {
  window.dispatchEvent(
    new CustomEvent('ide:show-toast', {
      detail: {
        key: 'synctex:request-error',
      },
    })
  )
}
