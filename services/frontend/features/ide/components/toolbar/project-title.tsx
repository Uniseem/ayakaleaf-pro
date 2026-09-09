'use client'

/**
 * The project's name in the middle of the toolbar, from
 * ide-react/components/toolbar/project-title.tsx.
 *
 * The name is a dropdown: downloads, a copy, and a rename. Renaming turns
 * the name into a field in place, committed with Enter or by leaving it and
 * abandoned with Escape.
 */

import { useCallback, useEffect, useRef, useState, type ChangeEventHandler } from 'react'
import { useTranslation } from '@/lib/i18n'
import { Dropdown, DropdownDivider, DropdownMenu, DropdownToggle, OLDropdownMenuItem } from '@/components/ol/dropdown'
import MaterialIcon from '@/components/ol/material-icon'
import { FormControl } from '@/components/ol/form-control'
import { Tooltip } from '@/components/ol/tooltip'
import { useProject } from '@/features/ide/contexts/project-context'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { EditorCloneProjectModalWrapper } from '@/features/clone-project-modal/clone-project-modal'

export function ToolbarProjectTitle() {
  const { t } = useTranslation()
  const { project, isOwner, setName } = useProject()
  const hasRenamePermissions = isOwner
  const [isRenaming, setIsRenaming] = useState(false)

  const onRename = useCallback(
    (name: string) => {
      if (name && name !== project.name) {
        void setName(name)
      }
      setIsRenaming(false)
    },
    [setName, project.name]
  )
  const onCancel = useCallback(() => setIsRenaming(false), [])

  if (isRenaming) {
    return (
      <EditableLabel
        onChange={onRename}
        onCancel={onCancel}
        initialText={project.name}
        maxLength={150}
        className="ide-redesign-toolbar-editable-project-name"
      />
    )
  }

  return (
    <Dropdown align="end" className="ide-redesign-toolbar-project-dropdown">
      <DropdownToggle
        id="project-title-options"
        aria-label={t('project_title_options')}
        className="ide-redesign-toolbar-project-dropdown-toggle ide-redesign-toolbar-dropdown-toggle-subdued fw-bold ide-redesign-toolbar-button-subdued"
      >
        <span className="ide-redesign-toolbar-project-name" translate="no">
          {project.name}
        </span>
        <MaterialIcon type="keyboard_arrow_down" />
      </DropdownToggle>
      <DropdownMenu renderOnMount>
        <DownloadProjectPDF />
        <DownloadProjectZip />
        <DropdownDivider />
        <DuplicateProject />
        <OLDropdownMenuItem onClick={() => setIsRenaming(true)} disabled={!hasRenamePermissions}>
          {t('rename')}
        </OLDropdownMenuItem>
      </DropdownMenu>
    </Dropdown>
  )
}

function EditableLabel({
  initialText,
  className,
  onChange,
  onCancel,
  maxLength,
}: {
  initialText: string
  className?: string
  onChange: (name: string) => void
  onCancel: () => void
  maxLength?: number
}) {
  const [name, setNameValue] = useState(initialText)
  const inputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    inputRef.current?.select()
  }, [])

  const onInputChange: ChangeEventHandler<HTMLInputElement> = useCallback(event => {
    setNameValue(event.target.value)
  }, [])

  const finishRenaming = useCallback(() => {
    onChange(name)
  }, [onChange, name])

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Enter') {
        event.preventDefault()
        finishRenaming()
      }
      if (event.key === 'Escape') {
        event.preventDefault()
        onCancel()
      }
    },
    [finishRenaming, onCancel]
  )

  return (
    <FormControl
      className={className}
      ref={inputRef}
      type="text"
      value={name}
      onChange={onInputChange}
      onKeyDown={onKeyDown}
      onBlur={finishRenaming}
      maxLength={maxLength}
    />
  )
}

export function DownloadProjectZip() {
  const { t } = useTranslation()
  const { projectId } = useProject()
  const href = `/api/projects/${projectId}/download/zip`

  useCommandProvider(() => [{ id: 'download-as-source-zip', href, label: t('download_as_source_zip') }], [t, href])

  return (
    <OLDropdownMenuItem href={href} target="_blank" rel="noreferrer">
      {t('download_as_source_zip')}
    </OLDropdownMenuItem>
  )
}

export function DownloadProjectPDF() {
  const { t } = useTranslation()
  const { pdfUrl } = useCompile()
  const pdfDownloadUrl = pdfUrl

  useCommandProvider(
    () => [{ id: 'download-pdf', disabled: !pdfUrl, href: pdfDownloadUrl || pdfUrl || undefined, label: t('download_as_pdf') }],
    [t, pdfUrl, pdfDownloadUrl]
  )

  const button = (
    <OLDropdownMenuItem href={pdfDownloadUrl || pdfUrl || undefined} target="_blank" rel="noreferrer" disabled={!pdfUrl}>
      {t('download_as_pdf')}
    </OLDropdownMenuItem>
  )

  if (!pdfUrl) {
    return (
      <Tooltip id="tooltip-download-pdf-unavailable" description={t('please_compile_pdf_before_download')} overlayProps={{ placement: 'right', delay: 0 }}>
        <span>{button}</span>
      </Tooltip>
    )
  }
  return button
}

export function DuplicateProject() {
  const { t } = useTranslation()
  const { projectId, project } = useProject()
  const [showModal, setShowModal] = useState(false)

  const openProject = useCallback((id: string) => {
    window.location.assign(`/projects/${id}`)
  }, [])

  return (
    <>
      <OLDropdownMenuItem onClick={() => setShowModal(true)}>{t('make_a_copy')}</OLDropdownMenuItem>
      <EditorCloneProjectModalWrapper
        show={showModal}
        handleHide={() => setShowModal(false)}
        openProject={openProject}
        projectId={projectId}
        projectName={project.name}
      />
    </>
  )
}
