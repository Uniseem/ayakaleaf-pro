import { FC, useCallback, useEffect, useRef, useState } from 'react'
import { useFigureModalContext } from '../figure-modal-context'
import { useCurrentProjectFolders } from '@/features/source-editor/hooks/use-current-project-folders'
import { File } from '../../../utils/file'
import classNames from '@/lib/cx'
import { FileRelocator } from '../file-relocator'
import { useTranslation } from '@/lib/i18n'
import { useCodeMirrorViewContext } from '../../codemirror-context'
import { waitForFileTreeUpdate } from '../../../extensions/figure-modal'
import { useProject } from '@/features/ide/contexts/project-context'
import { uploadFile } from '@/lib/editor'
import { OLFormGroup } from '@/components/ol/form-control'
import { Button as OLButton } from '@/components/ol/button'
import MaterialIcon from '@/components/ol/material-icon'
import { Spinner as OLSpinner } from '@/components/ol/spinner'

export enum FileUploadStatus {
  ERROR,
  SUCCESS,
  NOT_ATTEMPTED,
  UPLOADING,
}

// What \includegraphics can use without further packages, plus the vector
// formats the compiler accepts.
const ACCEPT = 'image/*,.pdf,.eps,.svg'

/**
 * Upload from the computer.
 *
 * The file is not sent when it is chosen: the modal's Insert button is what
 * uploads it, because choosing a file and then cancelling should leave no
 * trace in the project. `getPath` is therefore the upload -- the modal awaits
 * it, and what comes back is the path to write into the document.
 */
export const FigureModalUploadFileSource: FC = () => {
  const { t } = useTranslation()
  const view = useCodeMirrorViewContext()
  const { dispatch, pastedImageData } = useFigureModalContext()
  const { projectId, refresh } = useProject()
  const { rootFile } = useCurrentProjectFolders()
  const [folder, setFolder] = useState<File | null>(null)
  const [nameDirty, setNameDirty] = useState<boolean>(false)
  const [file, setFile] = useState<globalThis.File | null>(null)
  const [name, setName] = useState<string>('')
  const [uploading, setUploading] = useState<boolean>(false)
  const [uploadError, setUploadError] = useState<unknown>(null)
  const [dragging, setDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement | null>(null)

  const dispatchUploadAction = useCallback(
    (name?: string, file?: globalThis.File | null, folder?: File | null) => {
      if (!name || !file) {
        dispatch({ getPath: undefined })
        return
      }
      dispatch({
        getPath: async () => {
          const uploadFolder = folder ?? rootFile
          const target =
            uploadFolder.path === '' && uploadFolder.name === 'rootFolder'
              ? null
              : uploadFolder
          setUploadError(null)
          setUploading(true)
          try {
            const fileTreeUpdate = waitForFileTreeUpdate(view)
            // Renamed on the way out, so the name the user typed is the name
            // in the project rather than whatever the file was called on disk.
            await uploadFile(
              projectId,
              new globalThis.File([file], name, { type: file.type }),
              target?.id
            )
            await refresh()
            await fileTreeUpdate.withTimeout(500)
          } catch (error) {
            setUploadError(error)
            dispatch({
              error: error instanceof Error ? error.message : String(error),
            })
            throw error
          } finally {
            setUploading(false)
          }
          return target
            ? `${target.path ? target.path + '/' : ''}${target.name}/${name}`
            : name
        },
      })
    },
    [dispatch, projectId, refresh, rootFile, view]
  )

  const chooseFile = useCallback(
    (chosen: globalThis.File | null) => {
      if (!chosen) {
        if (!nameDirty) {
          setName('')
        }
        setFile(null)
        dispatchUploadAction(undefined, null, folder)
        return
      }
      const newName = nameDirty ? name : chosen.name
      setName(newName)
      setFile(chosen)
      dispatchUploadAction(newName, chosen, folder)
    },
    [dispatchUploadAction, folder, name, nameDirty]
  )

  // An image pasted or dropped into the editor opens this modal with the
  // image already in hand.
  useEffect(() => {
    if (pastedImageData) {
      chooseFile(
        new globalThis.File([pastedImageData.data], pastedImageData.name, {
          type: pastedImageData.type,
        })
      )
    }
    // only when the pasted image itself changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pastedImageData])

  return (
    <>
      <OLFormGroup>
        <div className="figure-modal-upload">
          {file ? (
            <FileContainer
              name={file.name}
              size={file.size}
              status={
                uploading
                  ? FileUploadStatus.UPLOADING
                  : uploadError
                    ? FileUploadStatus.ERROR
                    : FileUploadStatus.NOT_ATTEMPTED
              }
              onDelete={() => chooseFile(null)}
            />
          ) : (
            <div
              className={classNames('figure-modal-dropzone', {
                'figure-modal-dropzone-dragging': dragging,
              })}
              onDragOver={event => {
                event.preventDefault()
                setDragging(true)
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={event => {
                event.preventDefault()
                setDragging(false)
                chooseFile(event.dataTransfer.files[0] ?? null)
              }}
            >
              <input
                ref={inputRef}
                type="file"
                className="figure-modal-dropzone-input"
                accept={ACCEPT}
                onChange={event => chooseFile(event.target.files?.[0] ?? null)}
              />
              <span>
                {t('drag_here_paste_an_image_or')}{' '}
                <OLButton
                  variant="link"
                  className="p-0"
                  onClick={() => inputRef.current?.click()}
                >
                  {t('select_from_your_computer')}
                </OLButton>
              </span>
            </div>
          )}
        </div>
      </OLFormGroup>
      <FileRelocator
        folder={folder}
        name={name}
        nameDisabled={!file && !nameDirty}
        onFolderChanged={item => dispatchUploadAction(name, file, item ?? rootFile)}
        onNameChanged={name => dispatchUploadAction(name, file, folder)}
        setFolder={setFolder}
        setName={setName}
        setNameDirty={setNameDirty}
      />
    </>
  )
}

export const FileContainer: FC<{
  name: string
  size?: number
  status: FileUploadStatus
  onDelete?: () => any
}> = ({ name, size, status, onDelete }) => {
  const { t } = useTranslation()
  let icon = ''
  switch (status) {
    case FileUploadStatus.ERROR:
      icon = 'cancel'
      break
    case FileUploadStatus.SUCCESS:
      icon = 'check_circle'
      break
    case FileUploadStatus.NOT_ATTEMPTED:
      icon = 'imagesmode'
      break
  }

  return (
    <div className="file-container">
      <div className="file-container-file">
        <span
          className={classNames({
            'text-success': status === FileUploadStatus.SUCCESS,
            'text-danger': status === FileUploadStatus.ERROR,
          })}
        >
          {status === FileUploadStatus.UPLOADING ? (
            <OLSpinner size="sm" />
          ) : (
            <MaterialIcon type={icon} className="align-text-bottom" />
          )}
        </span>
        <div className="file-info">
          <span className="file-name" aria-label={t('file_name_figure_modal')}>
            {name}
          </span>
          {size !== undefined && <FileSize size={size} />}
        </div>
        <OLButton
          variant="link"
          className="p-0 text-decoration-none"
          aria-label={t('remove_or_replace_figure')}
          onClick={() => onDelete && onDelete()}
        >
          <MaterialIcon type="cancel" />
        </OLButton>
      </div>
    </div>
  )
}

const FileSize: FC<{ size: number; className?: string }> = ({ size, className }) => {
  const { t } = useTranslation()
  const BYTE_UNITS: [string, number][] = [
    ['B', 1],
    ['KB', 1e3],
    ['MB', 1e6],
    ['GB', 1e9],
    ['TB', 1e12],
    ['PB', 1e15],
  ]
  const labelIndex =
    size > 0 ? Math.min(Math.floor(Math.log10(size) / 3), BYTE_UNITS.length - 1) : 0

  const [label, bytesPerUnit] = BYTE_UNITS[labelIndex]!
  const sizeInUnits = Math.round(size / bytesPerUnit)
  return (
    <small aria-label={t('file_size')} className={className}>
      {sizeInUnits} {label}
    </small>
  )
}
