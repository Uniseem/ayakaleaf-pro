import {
  FigureModalSource,
  useFigureModalContext,
} from './figure-modal-context'
import { FigureModalHelp } from './figure-modal-help'
import { FigureModalFigureOptions } from './figure-modal-options'
import { FigureModalSourcePicker } from './figure-modal-source-picker'
import { FigureModalEditFigureSource } from './file-sources/figure-modal-edit-figure-source'
import { FigureModalCurrentProjectSource } from './file-sources/figure-modal-project-source'
import { FigureModalUploadFileSource } from './file-sources/figure-modal-upload-source'
import { useCallback } from 'react'
import { OLNotification } from '@/components/ol/notification'

// FROM_URL and OTHER_PROJECT insert linked files, which this deployment has
// no server support for; see figure-modal-source-picker.
const sourceModes = new Map([
  [FigureModalSource.FILE_TREE, FigureModalCurrentProjectSource],
  [FigureModalSource.FILE_UPLOAD, FigureModalUploadFileSource],
  [FigureModalSource.EDIT_FIGURE, FigureModalEditFigureSource],
])

export default function FigureModalBody() {
  const { source, helpShown, sourcePickerShown, error, dispatch } =
    useFigureModalContext()
  const Body = sourceModes.get(source)
  const onDismiss = useCallback(() => {
    dispatch({ error: undefined })
  }, [dispatch])

  if (helpShown) {
    return <FigureModalHelp />
  }

  if (sourcePickerShown) {
    return <FigureModalSourcePicker />
  }

  if (!Body) {
    return null
  }

  return (
    <>
      {error && (
        <OLNotification type="error" onDismiss={onDismiss} content={error} />
      )}
      <Body />
      <FigureModalFigureOptions />
    </>
  )
}
