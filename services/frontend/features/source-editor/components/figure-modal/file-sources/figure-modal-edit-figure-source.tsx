import { FC, useEffect } from 'react'
import { FileContainer, FileUploadStatus } from './figure-modal-upload-source'
import {
  useFigureModalContext,
  useFigureModalExistingFigureContext,
} from '../figure-modal-context'
import { useTranslation } from '@/lib/i18n'

export const FigureModalEditFigureSource: FC = () => {
  const { t } = useTranslation()
  const { dispatch } = useFigureModalContext()
  const { name } = useFigureModalExistingFigureContext()

  useEffect(() => {
    if (name === undefined) {
      dispatch({ getPath: undefined })
    } else {
      dispatch({ getPath: async () => name })
    }
  }, [name, dispatch])

  return (
    <FileContainer
      name={name ?? t('unknown')}
      status={FileUploadStatus.SUCCESS}
      onDelete={() => {
        dispatch({ sourcePickerShown: true })
      }}
    />
  )
}
