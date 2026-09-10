import {
  FigureModalSource,
  useFigureModalContext,
} from './figure-modal-context'
import { FC } from 'react'
import { useTranslation } from '@/lib/i18n'
import { Button as OLButton } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'

export const FigureModalFooter: FC<{
  onInsert: () => void
  onCancel: () => void
  onDelete: () => void
}> = ({ onInsert, onCancel, onDelete }) => {
  const { t } = useTranslation()

  return (
    <>
      <HelpToggle />
      <OLButton variant="secondary" onClick={onCancel}>
        {t('cancel')}
      </OLButton>
      <FigureModalAction onInsert={onInsert} onDelete={onDelete} />
    </>
  )
}

const HelpToggle = () => {
  const { t } = useTranslation()
  const { helpShown, dispatch } = useFigureModalContext()
  if (helpShown) {
    return (
      <OLButton
        variant="link"
        className="figure-modal-help-link me-auto"
        onClick={() => dispatch({ helpShown: false })}
      >
        <span>
          <MaterialIcon type="arrow_left_alt" className="align-text-bottom" />
        </span>{' '}
        {t('back')}
      </OLButton>
    )
  }
  return (
    <OLButton
      variant="link"
      className="figure-modal-help-link me-auto"
      onClick={() => dispatch({ helpShown: true })}
    >
      <span>
        <MaterialIcon type="help" className="align-text-bottom" />
      </span>{' '}
      {t('help')}
    </OLButton>
  )
}

const FigureModalAction: FC<{
  onInsert: () => void
  onDelete: () => void
}> = ({ onInsert, onDelete }) => {
  const { t } = useTranslation()
  const { helpShown, getPath, source, sourcePickerShown } =
    useFigureModalContext()

  if (helpShown) {
    return null
  }

  if (sourcePickerShown) {
    return (
      <OLButton variant="danger" onClick={onDelete}>
        {t('delete_figure')}
      </OLButton>
    )
  }

  if (source === FigureModalSource.EDIT_FIGURE) {
    return (
      <OLButton
        variant="primary"
        onClick={() => {
          onInsert()
          }}
      >
        {t('done')}
      </OLButton>
    )
  }

  return (
    <OLButton
      variant="primary"
      disabled={getPath === undefined}
      onClick={() => {
        onInsert()
      }}
    >
      {t('insert_figure')}
    </OLButton>
  )
}
