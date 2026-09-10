import { FC } from 'react'
import { useTranslation } from '@/lib/i18n'
import { useIncludedFile } from '@/features/source-editor/hooks/use-included-file'
import MaterialIcon from '@/components/ol/material-icon'
import { Button } from '@/components/ol/button'

export const IncludeTooltipContent: FC = () => {
  const { t } = useTranslation()
  const { openIncludedFile } = useIncludedFile('IncludeArgument')

  return (
    <div className="ol-cm-command-tooltip-content">
      <Button
        variant="link"
        type="button"
        className="ol-cm-command-tooltip-link"
        onClick={openIncludedFile}
      >
        <MaterialIcon type="edit" />
        {t('open_file')}
      </Button>
    </div>
  )
}
