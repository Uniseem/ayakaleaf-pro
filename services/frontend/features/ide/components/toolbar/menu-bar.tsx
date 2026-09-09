'use client'

/**
 * The File / Edit / Insert / View / Format / Help menus, from
 * ide-react/components/toolbar/menu-bar.tsx.
 *
 * Most entries are commands read from the registry; the View menu's layout
 * and editor-setting toggles are the exception, because they are settings
 * rather than things done to the document.
 */

import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from '@/lib/i18n'
import { DropdownDivider, DropdownHeader, DropdownItem } from '@/components/ol/dropdown'
import { MenuBar, MenuBarDropdown, MenuBarOption } from '@/components/ol/menu-bar'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useSettings } from '@/features/ide/contexts/settings-context'
import { useRailContext } from '@/features/ide/contexts/rail-context'
import { useCommandProvider } from '@/features/ide/contexts/command-registry-context'
import { useSite } from '@/features/ide/contexts/site-context'
import { ChangeLayoutOptions } from './buttons'
import CommandDropdown, { CommandSection, type MenuSectionStructure, type MenuStructure } from './command-dropdown'
import { WordCountModal } from '@/features/word-count/word-count-modal'
import { EditorCloneProjectModalWrapper } from '@/features/clone-project-modal/clone-project-modal'

export function ToolbarMenuBar() {
  const { t } = useTranslation()
  const { setView, view } = useLayout()
  const { pdfUrl } = useCompile()
  const { projectId, project } = useProject()
  const site = useSite()
  const wordCountEnabled = Boolean(pdfUrl)
  const [showWordCountModal, setShowWordCountModal] = useState(false)
  const [showCloneProjectModal, setShowCloneProjectModal] = useState(false)

  const openProject = useCallback((id: string) => {
    window.location.assign(`/projects/${id}`)
  }, [])

  useCommandProvider(
    () => [
      {
        type: 'command',
        label: t('show_version_history'),
        handler: () => {
          setView(view === 'history' ? 'editor' : 'history')
        },
        id: 'show_version_history',
      },
      {
        type: 'command',
        label: t('word_count_lower'),
        disabled: !wordCountEnabled,
        handler: () => {
          setShowWordCountModal(true)
        },
        id: 'word_count',
      },
      {
        type: 'command',
        label: t('make_a_copy'),
        handler: () => {
          setShowCloneProjectModal(true)
        },
        id: 'copy_project',
      },
    ],
    [t, setView, view, wordCountEnabled]
  )

  const fileMenuStructure: MenuStructure = useMemo(
    () => [
      { id: 'file-file-tree', children: ['new_file', 'new_folder', 'upload_file', 'copy_project'] },
      { id: 'file-tools', children: ['show_version_history', 'word_count'] },
      { id: 'submit', children: ['submit-project', 'manage-template'] },
      {
        id: 'file-download',
        children: [
          {
            id: 'file-download-group',
            title: t('download'),
            children: ['download-as-source-zip', 'download-pdf', 'export-as-docx', 'export-as-markdown', 'export-as-html'],
          },
        ],
      },
      { id: 'settings', children: ['open-settings'] },
    ],
    [t]
  )

  const editMenuStructure: MenuStructure = useMemo(
    () => [
      { id: 'edit-undo-redo', children: ['undo', 'redo'] },
      { id: 'edit-search', children: ['find', 'select-all'] },
    ],
    []
  )

  const insertMenuStructure: MenuStructure = useMemo(
    () => [
      {
        id: 'insert-latex',
        children: [
          { id: 'insert-math-group', title: t('math'), children: ['insert-inline-math', 'insert-display-math'] },
          'insert-symbol',
          {
            id: 'insert-figure-group',
            title: t('figure'),
            children: [
              'insert-figure-from-computer',
              'insert-figure-from-project-files',
              'insert-figure-from-another-project',
              'insert-figure-from-url',
            ],
          },
          'insert-table',
          'insert-citation',
          'insert-link',
          'insert-cross-reference',
        ],
      },
      { id: 'insert-comment', children: ['comment'] },
    ],
    [t]
  )

  const formatMenuStructure: MenuStructure = useMemo(
    () => [
      { id: 'format-text', children: ['format-bold', 'format-italics'] },
      {
        id: 'format-list',
        children: ['format-bullet-list', 'format-numbered-list', 'format-increase-indentation', 'format-decrease-indentation'],
      },
      {
        id: 'format-paragraph',
        title: t('paragraph_styles'),
        children: [
          'format-style-normal',
          'format-style-section',
          'format-style-subsection',
          'format-style-subsubsection',
          'format-style-paragraph',
          'format-style-subparagraph',
        ],
      },
    ],
    [t]
  )

  const pdfControlsMenuSectionStructure: MenuSectionStructure = useMemo(
    () => ({
      title: t('pdf_preview'),
      id: 'pdf-controls',
      children: ['view-pdf-presentation-mode', 'view-pdf-zoom-in', 'view-pdf-zoom-out', 'view-pdf-fit-width', 'view-pdf-fit-height'],
    }),
    [t]
  )

  const settings = useSettings()
  const { mathPreview, breadcrumbs, editorTabs } = settings

  const toggleMathPreview = useCallback(() => settings.set('mathPreview', !mathPreview), [settings, mathPreview])
  const toggleBreadcrumbs = useCallback(() => settings.set('breadcrumbs', !breadcrumbs), [settings, breadcrumbs])
  const toggleEditorTabs = useCallback(() => settings.set('editorTabs', !editorTabs), [settings, editorTabs])

  const { setActiveModal } = useRailContext()
  const openKeyboardShortcutsModal = useCallback(() => setActiveModal('keyboard-shortcuts'), [setActiveModal])
  const openContactUsModal = useCallback(() => setActiveModal('contact-us'), [setActiveModal])

  return (
    <>
      <MenuBar className="ide-redesign-toolbar-menu-bar" id="toolbar-menu-bar-item">
        <CommandDropdown menu={fileMenuStructure} title={t('file')} id="file" />
        <CommandDropdown menu={editMenuStructure} title={t('edit')} id="edit" />
        <CommandDropdown menu={insertMenuStructure} title={t('insert')} id="insert" />
        <MenuBarDropdown title={t('view')} id="view" className="ide-redesign-toolbar-dropdown-toggle-subdued ide-redesign-toolbar-button-subdued">
          <ChangeLayoutOptions />
          <DropdownDivider />
          <DropdownHeader>Editor settings</DropdownHeader>
          <MenuBarOption
            eventKey="show_breadcrumbs"
            title={t('show_breadcrumbs')}
            leadingIcon={breadcrumbs ? 'check' : <DropdownItem.EmptyLeadingIcon />}
            onClick={toggleBreadcrumbs}
          />
          <MenuBarOption
            eventKey="show_editor_tabs"
            title={t('show_editor_tabs')}
            leadingIcon={editorTabs ? 'check' : <DropdownItem.EmptyLeadingIcon />}
            onClick={toggleEditorTabs}
          />
          <MenuBarOption
            eventKey="show_equation_preview"
            title={t('show_equation_preview')}
            leadingIcon={mathPreview ? 'check' : <DropdownItem.EmptyLeadingIcon />}
            onClick={toggleMathPreview}
          />
          <CommandSection section={pdfControlsMenuSectionStructure} includeDivider />
        </MenuBarDropdown>
        <CommandDropdown menu={formatMenuStructure} title={t('format')} id="format" />
        <MenuBarDropdown title={t('help')} id="help" className="ide-redesign-toolbar-dropdown-toggle-subdued ide-redesign-toolbar-button-subdued">
          <MenuBarOption eventKey="keyboard_shortcuts" title={t('keyboard_shortcuts')} onClick={openKeyboardShortcutsModal} />
          {site.wikiEnabled ? (
            <MenuBarOption title={t('documentation')} eventKey="documentation" href="/learn" target="_blank" rel="noopener noreferrer" />
          ) : null}
          {site.showSupport ? (
            <>
              <DropdownDivider />
              <MenuBarOption eventKey="contact_us" title={t('contact_us')} onClick={openContactUsModal} />
            </>
          ) : null}
        </MenuBarDropdown>
      </MenuBar>
      <WordCountModal show={showWordCountModal} handleHide={() => setShowWordCountModal(false)} />
      <EditorCloneProjectModalWrapper
        show={showCloneProjectModal}
        handleHide={() => setShowCloneProjectModal(false)}
        openProject={openProject}
        projectId={projectId}
        projectName={project.name}
      />
    </>
  )
}
