'use client'

/**
 * Making a copy of a project, from clone-project-modal.
 */

import { memo, useCallback, useMemo, useState, type FormEvent } from 'react'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Notification } from '@/components/ol/notification'
import { Button } from '@/components/ol/button'
import { FormControl, OLFormGroup, OLFormLabel } from '@/components/ol/form-control'
import { api, ApiError } from '@/lib/api'
import { useTranslation } from '@/lib/i18n'

export type ClonedProject = { project_id: string; id?: string; name?: string }

function CloneProjectModalContent({
  handleHide,
  inFlight,
  setInFlight,
  handleAfterCloned,
  projectId,
  projectName,
}: {
  handleHide: () => void
  inFlight: boolean
  setInFlight: (inFlight: boolean) => void
  handleAfterCloned: (clonedProject: ClonedProject) => void
  projectId: string
  projectName: string
}) {
  const { t } = useTranslation()
  const [error, setError] = useState<string | boolean>()
  const [clonedProjectName, setClonedProjectName] = useState(`${projectName} (Copy)`)

  const valid = useMemo(() => clonedProjectName.trim().length > 0, [clonedProjectName])

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!valid) {
      return
    }
    setError(false)
    setInFlight(true)

    api<{ project: { id: string } }>(`/api/projects/${projectId}/clone`, {
      method: 'POST',
      body: { name: clonedProjectName, projectName: clonedProjectName },
    })
      .then(data => {
        const id = (data as { project?: { id: string }; project_id?: string; id?: string }).project?.id
        handleAfterCloned({ project_id: id ?? (data as { project_id?: string }).project_id ?? (data as { id?: string }).id ?? '' })
      })
      .catch(thrown => {
        if (thrown instanceof ApiError && thrown.status === 400) {
          setError(thrown.message)
        } else {
          setError(true)
        }
      })
      .finally(() => setInFlight(false))
  }

  return (
    <>
      <OLModalHeader>
        <OLModalTitle>{t('copy_project')}</OLModalTitle>
      </OLModalHeader>

      <OLModalBody>
        <form id="clone-project-form" onSubmit={handleSubmit}>
          <OLFormGroup controlId="clone-project-form-name">
            <OLFormLabel htmlFor="clone-project-form-name">{t('new_name')}</OLFormLabel>
            <FormControl
              id="clone-project-form-name"
              type="text"
              required
              value={clonedProjectName}
              onChange={event => setClonedProjectName(event.target.value)}
              autoFocus
            />
          </OLFormGroup>
        </form>

        {error ? (
          <Notification content={typeof error === 'string' && error.length ? error : t('generic_something_went_wrong')} type="error" />
        ) : null}
      </OLModalBody>

      <OLModalFooter>
        <Button variant="secondary" disabled={inFlight} onClick={handleHide}>
          {t('cancel')}
        </Button>
        <Button variant="primary" disabled={inFlight || !valid} form="clone-project-form" type="submit">
          {inFlight ? <>{t('copying')}…</> : t('copy')}
        </Button>
      </OLModalFooter>
    </>
  )
}

export const CloneProjectModal = memo(function CloneProjectModal({
  show,
  handleHide,
  handleAfterCloned,
  projectId,
  projectName,
}: {
  show: boolean
  handleHide: () => void
  handleAfterCloned: (clonedProject: ClonedProject) => void
  projectId: string
  projectName: string
}) {
  const [inFlight, setInFlight] = useState(false)

  const onHide = useCallback(() => {
    if (!inFlight) {
      handleHide()
    }
  }, [handleHide, inFlight])

  return (
    <OLModal animation show={show} onHide={onHide} id="clone-project-modal" backdrop={inFlight ? 'static' : undefined}>
      <CloneProjectModalContent
        handleHide={onHide}
        inFlight={inFlight}
        setInFlight={setInFlight}
        handleAfterCloned={handleAfterCloned}
        projectId={projectId}
        projectName={projectName}
      />
    </OLModal>
  )
})

/** The copy dialog as the editor opens it: the copy is opened afterwards. */
export function EditorCloneProjectModalWrapper({
  show,
  handleHide,
  openProject,
  projectId,
  projectName,
}: {
  show: boolean
  handleHide: () => void
  openProject: (projectId: string) => void
  projectId: string
  projectName: string
}) {
  const handleAfterCloned = useCallback(
    ({ project_id: id }: ClonedProject) => {
      openProject(id)
    },
    [openProject]
  )
  return (
    <CloneProjectModal handleHide={handleHide} show={show} handleAfterCloned={handleAfterCloned} projectId={projectId} projectName={projectName} />
  )
}
