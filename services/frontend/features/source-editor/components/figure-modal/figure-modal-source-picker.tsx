import { FC } from 'react'
import {
  FigureModalSource,
  useFigureModalContext,
} from './figure-modal-context'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from '@/components/ol/material-icon'
import { useProject } from '@/features/ide/contexts/project-context'

/**
 * Where to get the replacement image from.
 *
 * Two of the original's four sources -- another project, and a URL -- create
 * linked files, which this deployment has no server support for: a linked file
 * has to be refetched and refreshed, and there is nothing to do that. The
 * original hides those buttons when the features are off, and so does this.
 */
export const FigureModalSourcePicker: FC = () => {
  const { t } = useTranslation()
  const { canWrite } = useProject()

  return (
    <div className="figure-modal-source-button-grid">
      {canWrite && (
        <FigureModalSourceButton
          type={FigureModalSource.FILE_UPLOAD}
          title={t('replace_from_computer')}
          icon="upload"
        />
      )}
      <FigureModalSourceButton
        type={FigureModalSource.FILE_TREE}
        title={t('replace_from_project_files')}
        icon="inbox"
      />
    </div>
  )
}

const FigureModalSourceButton: FC<{
  type: FigureModalSource
  title: string
  icon: string
}> = ({ type, title, icon }) => {
  const { dispatch } = useFigureModalContext()
  return (
    <button
      type="button"
      className="figure-modal-source-button"
      onClick={() => {
        dispatch({ source: type, sourcePickerShown: false, getPath: undefined })
      }}
    >
      <MaterialIcon type={icon} className="figure-modal-source-button-icon" />
      <span className="figure-modal-source-button-title">{title}</span>
      <MaterialIcon type="chevron_right" />
    </button>
  )
}
