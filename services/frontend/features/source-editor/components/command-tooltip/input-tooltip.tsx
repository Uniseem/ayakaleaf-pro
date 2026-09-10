import { FC } from 'react'
import { useTranslation } from '@/lib/i18n'
import { useIncludedFile } from '@/features/source-editor/hooks/use-included-file'
import { Button } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'

export const InputTooltipContent: FC = () => {
  const { t } = useTranslation()
  const { openIncludedFile } = useIncludedFile('InputArgument')

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
