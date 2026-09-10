'use client'

/**
 * Adding files, from file-tree/components/modals/file-tree-modal-create-file
 * and the file-tree-create pieces under it.
 *
 * A list of ways to add something down the left and the chosen one on the
 * right, because "new file" and "upload" are the same act with different
 * sources and switching between them should not close anything.
 *
 * The original also offers "from another project" and "from an external URL".
 * Both make a linked file, which this deployment's API does not keep, so they
 * are not offered rather than offered and refused.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import cx from '@/lib/cx'
import { useTranslation } from '@/lib/i18n'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { OLFormControl, OLFormGroup, OLFormLabel } from '@/components/ol/form-control'
import { Notification } from '@/components/ol/notification'
import MaterialIcon from '@/components/ol/material-icon'
import { messageFor } from '@/lib/api'
import { useFileTreeActionable, type NewFileCreateMode } from '../../contexts/file-tree-actionable'
import { BlockedFilenameError, DuplicateFilenameError, InvalidFilenameError } from '../../errors'
import { isCleanFilename } from '../../util/safe-path'

export function FileTreeModalCreateFile() {
  const { t } = useTranslation()
  const { isCreatingFile, cancel } = useFileTreeActionable()

  if (!isCreatingFile) {
    return null
  }

  return (
    <OLModal size="lg" onHide={cancel} show>
      <OLModalHeader>
        <OLModalTitle>{t('add_files')}</OLModalTitle>
      </OLModalHeader>
      <FileTreeModalCreateFileBody />
    </OLModal>
  )
}

function FileTreeModalCreateFileBody() {
  const { t } = useTranslation()
  const { newFileCreateMode } = useFileTreeActionable()

  return (
    <>
      <OLModalBody className="modal-new-file">
        <div className="modal-new-file-body">
          <ul className="modal-new-file-list list-unstyled">
            <FileTreeModalCreateFileMode mode="doc" icon="note_add" label={t('new_file')} />
            <FileTreeModalCreateFileMode mode="upload" icon="upload" label={t('upload')} />
          </ul>
          <div className="modal-new-file-body-mode">
            {newFileCreateMode === 'upload' ? <FileTreeUploadDoc /> : <FileTreeCreateNewDoc />}
          </div>
        </div>
      </OLModalBody>
    </>
  )
}

function FileTreeModalCreateFileMode({
  mode,
  icon,
  label,
}: {
  mode: NewFileCreateMode
  icon: string
  label: string
}) {
  const { newFileCreateMode, startCreatingFile } = useFileTreeActionable()

  return (
    <li className={cx({ active: newFileCreateMode === mode })}>
      <Button
        variant="link"
        onClick={() => startCreatingFile(mode)}
        className="modal-new-file-mode"
        leadingIcon={icon}
      >
        {label}
      </Button>
    </li>
  )
}

/** The name box, with the two things that can be wrong with a name. */
function FileTreeCreateNameInput({
  name,
  setName,
  label,
  placeholder,
  error,
  inFlight,
}: {
  name: string
  setName: (value: string) => void
  label?: string
  placeholder?: string
  error?: unknown
  inFlight: boolean
}) {
  const { t } = useTranslation()
  const [touched, setTouched] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const validName = isCleanFilename(name.trim())

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      const input = inputRef.current
      if (input) {
        input.focus()
        const dot = input.value.lastIndexOf('.')
        input.setSelectionRange(0, dot === -1 ? input.value.length : dot)
      }
    })
    return () => window.cancelAnimationFrame(frame)
  }, [])

  return (
    <OLFormGroup controlId="new-doc-name">
      <OLFormLabel>{label || t('file_name')}</OLFormLabel>

      <OLFormControl
        type="text"
        placeholder={placeholder || t('file_name')}
        required
        value={name}
        onChange={event => {
          setTouched(true)
          setName(event.target.value)
        }}
        ref={inputRef}
        disabled={inFlight}
      />

      {touched && !validName && (
        <Notification
          type="error"
          className="row-spaced-small"
          content={t('files_cannot_include_invalid_characters')}
        />
      )}

      {error != null && <ErrorMessage error={error} />}
    </OLFormGroup>
  )
}

function ErrorMessage({ error }: { error: unknown }) {
  const { t } = useTranslation()

  let message: string
  if (error instanceof DuplicateFilenameError) {
    message = t('file_already_exists')
  } else if (error instanceof InvalidFilenameError) {
    message = t('files_cannot_include_invalid_characters')
  } else if (error instanceof BlockedFilenameError) {
    message = t('blocked_filename')
  } else {
    message = messageFor(error)
  }

  return <Notification type="error" className="row-spaced-small" content={message} />
}

function FileTreeCreateNewDoc() {
  const { t } = useTranslation()
  const { finishCreatingDoc, inFlight, error, cancel } = useFileTreeActionable()
  const [name, setName] = useState('name.tex')

  const valid = isCleanFilename(name.trim())

  const create = useCallback(() => {
    if (valid && !inFlight) {
      void finishCreatingDoc({ name: name.trim() })
    }
  }, [valid, inFlight, finishCreatingDoc, name])

  return (
    <form
      className="form-controls"
      onSubmit={event => {
        event.preventDefault()
        create()
      }}
      noValidate
    >
      <FileTreeCreateNameInput name={name} setName={setName} error={error} inFlight={inFlight} />
      <OLModalFooter>
        <Button variant="secondary" onClick={cancel} disabled={inFlight}>
          {t('cancel')}
        </Button>
        <Button
          variant="primary"
          type="submit"
          disabled={!valid || inFlight}
          isLoading={inFlight}
          loadingLabel={t('creating')}
        >
          {t('create')}
        </Button>
      </OLModalFooter>
    </form>
  )
}

function FileTreeUploadDoc() {
  const { t } = useTranslation()
  const { finishUploadingFiles, inFlight, error, cancel } = useFileTreeActionable()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)
  const [chosen, setChosen] = useState<File[]>([])

  const take = useCallback((list: FileList | null) => {
    setChosen(list ? Array.from(list) : [])
  }, [])

  return (
    <div className="modal-new-file-uploader">
      <div
        className={cx('modal-new-file-dropzone', { 'modal-new-file-dropzone-over': dragging })}
        onDragOver={event => {
          event.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={event => {
          event.preventDefault()
          setDragging(false)
          take(event.dataTransfer.files)
        }}
      >
        <MaterialIcon type="upload" className="modal-new-file-dropzone-icon" />
        <p>{t('drag_here')}</p>
        <Button variant="secondary" onClick={() => inputRef.current?.click()} disabled={inFlight}>
          {t('select_from_your_computer')}
        </Button>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="visually-hidden"
          onChange={event => take(event.target.files)}
        />
      </div>

      {chosen.length > 0 && (
        <ul className="modal-new-file-upload-list list-unstyled">
          {chosen.map(file => (
            <li key={file.name}>{file.name}</li>
          ))}
        </ul>
      )}

      {error != null && <ErrorMessage error={error} />}

      <OLModalFooter>
        <Button variant="secondary" onClick={cancel} disabled={inFlight}>
          {t('cancel')}
        </Button>
        <Button
          variant="primary"
          onClick={() => void finishUploadingFiles(chosen)}
          disabled={!chosen.length || inFlight}
          isLoading={inFlight}
          loadingLabel={t('upload')}
        >
          {t('upload')}
        </Button>
      </OLModalFooter>
    </div>
  )
}

export default FileTreeModalCreateFile
