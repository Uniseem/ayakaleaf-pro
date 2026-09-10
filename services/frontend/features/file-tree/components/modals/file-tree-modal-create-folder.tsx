'use client'

import { useEffect, useRef, useState } from 'react'
import { useTranslation } from '@/lib/i18n'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { OLFormControl, OLFormGroup, OLFormLabel } from '@/components/ol/form-control'
import { useFileTreeActionable } from '../../contexts/file-tree-actionable'
import { DuplicateFilenameError } from '../../errors'
import { isCleanFilename } from '../../util/safe-path'

export function FileTreeModalCreateFolder() {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [validName, setValidName] = useState(true)

  const { isCreatingFolder, inFlight, finishCreatingFolder, cancel, error } = useFileTreeActionable()

  useEffect(() => {
    if (!isCreatingFolder) {
      setName('')
      setValidName(true)
    }
  }, [isCreatingFolder])

  if (!isCreatingFolder) {
    return null
  }

  function handleCreateFolder() {
    if (validName && name.trim()) {
      void finishCreatingFolder(name.trim())
    }
  }

  function errorMessage() {
    if (error instanceof DuplicateFilenameError) {
      return t('file_already_exists')
    }
    return t('generic_something_went_wrong')
  }

  return (
    <OLModal show onHide={cancel}>
      <OLModalHeader>
        <OLModalTitle>{t('new_folder')}</OLModalTitle>
      </OLModalHeader>

      <OLModalBody>
        <InputName
          name={name}
          setName={setName}
          setValidName={setValidName}
          handleCreateFolder={handleCreateFolder}
        />
        {!validName ? (
          <div
            role="alert"
            aria-label={t('files_cannot_include_invalid_characters')}
            className="alert alert-danger file-tree-modal-alert"
          >
            {t('files_cannot_include_invalid_characters')}
          </div>
        ) : null}
        {error ? (
          <div role="alert" aria-label={errorMessage()} className="alert alert-danger file-tree-modal-alert">
            {errorMessage()}
          </div>
        ) : null}
      </OLModalBody>

      <OLModalFooter>
        {inFlight ? (
          <Button variant="primary" disabled isLoading loadingLabel={t('creating')} />
        ) : (
          <>
            <Button variant="secondary" onClick={cancel}>
              {t('cancel')}
            </Button>
            <Button variant="primary" onClick={handleCreateFolder} disabled={!validName || !name.trim()}>
              {t('create')}
            </Button>
          </>
        )}
      </OLModalFooter>
    </OLModal>
  )
}

function InputName({
  name,
  setName,
  setValidName,
  handleCreateFolder,
}: {
  name: string
  setName: (name: string) => void
  setValidName: (valid: boolean) => void
  handleCreateFolder: () => void
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => inputRef.current?.focus())
    return () => window.cancelAnimationFrame(frame)
  }, [])

  return (
    <OLFormGroup controlId="new-folder-name">
      <OLFormLabel>{t('new_folder')}</OLFormLabel>
      <OLFormControl
        ref={inputRef}
        type="text"
        value={name}
        onChange={event => {
          setValidName(isCleanFilename(event.target.value.trim()))
          setName(event.target.value)
        }}
        onKeyDown={event => {
          if (event.key === 'Enter') {
            handleCreateFolder()
          }
        }}
        onFocus={event => event.target.setSelectionRange(0, -1)}
      />
    </OLFormGroup>
  )
}

export default FileTreeModalCreateFolder
