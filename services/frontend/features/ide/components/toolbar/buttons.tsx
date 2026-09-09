'use client'

/**
 * The toolbar's buttons, from ide-react/components/toolbar: the logo that
 * is the way back to the project list, the history toggle, the layout menu,
 * the Share button, and the way back from history.
 */

import { useCallback, useState, type ReactNode } from 'react'
import { useTranslation } from '@/lib/i18n'
import { isMac } from '@/lib/os'
import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import { Button } from '@/components/ol/button'
import { IconButton } from '@/components/ol/icon-button'
import { Spinner } from '@/components/ol/spinner'
import { Shortcut } from '@/components/ol/misc'
import { Dropdown, DropdownDivider, DropdownHeader, DropdownItem, DropdownMenu, DropdownToggle } from '@/components/ol/dropdown'
import { useLayout, type IdeLayout, type IdeView } from '@/features/ide/contexts/layout-context'
import { ShareProjectModal } from '@/features/share-project-modal/share-project-modal'

export function ToolbarLogos() {
  const { t } = useTranslation()
  return (
    <div className="ide-redesign-toolbar-logos">
      <Tooltip id="tooltip-home-button" description={t('back_to_your_projects')} overlayProps={{ delay: 0, placement: 'bottom' }}>
        <div className="ide-redesign-toolbar-home-button">
          <a href="/projects" className="ide-redesign-toolbar-home-link">
            <span className="toolbar-ol-logo" aria-label={t('overleaf_logo')} />
            <MaterialIcon type="home" className="toolbar-ol-home-button" />
          </a>
        </div>
      </Tooltip>
    </div>
  )
}

export function ShowHistoryButton() {
  const { t } = useTranslation()
  const { view, setView, restoreView } = useLayout()

  const toggleHistoryOpen = useCallback(() => {
    if (view === 'history') {
      restoreView()
    } else {
      setView('history')
    }
  }, [view, setView, restoreView])

  return (
    <div className="ide-redesign-toolbar-button-container">
      <Tooltip id="tooltip-open-history" description={t('history')} overlayProps={{ delay: 0, placement: 'bottom' }}>
        <IconButton
          icon="history"
          className="ide-redesign-toolbar-button-subdued ide-redesign-toolbar-button-icon"
          onClick={toggleHistoryOpen}
          accessibilityLabel={t('history')}
        />
      </Tooltip>
    </div>
  )
}

export function ChangeLayoutButton() {
  const { t } = useTranslation()
  return (
    <div className="ide-redesign-toolbar-button-container">
      <Dropdown className="toolbar-item layout-dropdown" align="end">
        <Tooltip id="tooltip-open-layout-options" description={t('layout_options')} overlayProps={{ delay: 0, placement: 'bottom' }}>
          <span>
            <DropdownToggle
              id="layout-dropdown-btn"
              className="ide-redesign-toolbar-button-subdued ide-redesign-toolbar-dropdown-toggle-subdued ide-redesign-toolbar-button-icon"
              aria-label={t('layout_options')}
            >
              <MaterialIcon type="space_dashboard" unfilled />
            </DropdownToggle>
          </span>
        </Tooltip>
        <DropdownMenu>
          <ChangeLayoutOptions />
        </DropdownMenu>
      </Dropdown>
    </div>
  )
}

type LayoutOption = 'sideBySide' | 'editorOnly' | 'pdfOnly' | 'detachedPdf' | 'focusMode'

const getActiveLayoutOption = ({
  pdfLayout,
  view,
  detachRole,
}: {
  pdfLayout: IdeLayout
  view: IdeView | null
  detachRole?: 'detacher' | 'detached' | null
}): LayoutOption | null => {
  if (view === 'history') {
    return null
  }
  if (detachRole === 'detacher') {
    return 'detachedPdf'
  }
  if (pdfLayout === 'flat' && (view === 'editor' || view === 'file')) {
    return 'editorOnly'
  }
  if (pdfLayout === 'flat' && view === 'pdf') {
    return 'pdfOnly'
  }
  if (pdfLayout === 'sideBySide') {
    return 'sideBySide'
  }
  return null
}

function LayoutDropdownItem({
  active,
  disabled = false,
  processing = false,
  leadingIcon,
  trailingIcon,
  onClick,
  children,
}: {
  active: boolean
  leadingIcon: ReactNode
  trailingIcon?: ReactNode
  onClick: () => void
  children: ReactNode
  processing?: boolean
  disabled?: boolean
}) {
  if (processing) {
    leadingIcon = <Spinner size="sm" />
  } else if (active) {
    leadingIcon = 'check'
  }

  return (
    <li role="none">
      <DropdownItem
        active={active}
        aria-current={active}
        disabled={disabled}
        onClick={onClick}
        leadingIcon={leadingIcon}
        trailingIcon={trailingIcon}
        className={isMac ? 'dropdown-item-wide' : undefined}
      >
        {children}
      </DropdownItem>
    </li>
  )
}

const shortcuts: Record<LayoutOption, string[] | null> = isMac
  ? {
      editorOnly: ['⌃', '⌘', '←'],
      pdfOnly: ['⌃', '⌘', '→'],
      sideBySide: ['⌃', '⌘', '↓'],
      detachedPdf: ['⌃', '⌘', '↑'],
      focusMode: ['⌘', '⇧', 'M'],
    }
  : {
      editorOnly: null,
      pdfOnly: null,
      sideBySide: null,
      detachedPdf: null,
      focusMode: ['⌃', '⇧', 'M'],
    }

export function ChangeLayoutOptions() {
  const { detachIsLinked, detachRole, view, pdfLayout, handleChangeLayout, handleDetach, focusMode, setFocusMode } = useLayout()
  const { t } = useTranslation()
  const focusModeEnabled = false
  const detachable = typeof window !== 'undefined' && 'BroadcastChannel' in window && Boolean(handleDetach)

  const activeLayoutOption = getActiveLayoutOption({ pdfLayout, view, detachRole })
  const waitingForDetachedLink = !detachIsLinked && detachRole === 'detacher'

  return (
    <>
      <DropdownHeader>{t('layout_options')}</DropdownHeader>
      <LayoutDropdownItem
        onClick={() => handleChangeLayout('sideBySide')}
        active={activeLayoutOption === 'sideBySide'}
        leadingIcon="splitscreen_right"
        trailingIcon={shortcuts.sideBySide ? <Shortcut keys={shortcuts.sideBySide} /> : undefined}
      >
        {t('split_view')}
      </LayoutDropdownItem>
      <LayoutDropdownItem
        onClick={() => handleChangeLayout('flat', 'editor')}
        active={activeLayoutOption === 'editorOnly'}
        leadingIcon="edit"
        trailingIcon={shortcuts.editorOnly ? <Shortcut keys={shortcuts.editorOnly} /> : undefined}
      >
        {t('editor_only')}
      </LayoutDropdownItem>
      <LayoutDropdownItem
        onClick={() => handleChangeLayout('flat', 'pdf')}
        active={activeLayoutOption === 'pdfOnly'}
        leadingIcon="picture_as_pdf"
        trailingIcon={shortcuts.pdfOnly ? <Shortcut keys={shortcuts.pdfOnly} /> : undefined}
      >
        {t('pdf_only')}
      </LayoutDropdownItem>
      <LayoutDropdownItem
        onClick={() => handleDetach?.()}
        active={activeLayoutOption === 'detachedPdf' && detachIsLinked}
        disabled={!detachable}
        leadingIcon="open_in_new"
        trailingIcon={shortcuts.detachedPdf ? <Shortcut keys={shortcuts.detachedPdf} /> : undefined}
        processing={waitingForDetachedLink}
      >
        {t('open_pdf_in_separate_tab')}
      </LayoutDropdownItem>
      {focusModeEnabled ? (
        <>
          <DropdownDivider />
          <LayoutDropdownItem
            onClick={() => setFocusMode(!focusMode)}
            active={focusMode}
            leadingIcon="crop_free"
            trailingIcon={shortcuts.focusMode ? <Shortcut keys={shortcuts.focusMode} /> : undefined}
          >
            {t('focus_mode')}
          </LayoutDropdownItem>
        </>
      ) : null}
    </>
  )
}

export function ShareProjectButton() {
  const { t } = useTranslation()
  const [showShareModal, setShowShareModal] = useState(false)

  const handleOpenShareModal = useCallback(() => setShowShareModal(true), [])
  const handleHideShareModal = useCallback(() => setShowShareModal(false), [])

  return (
    <>
      <div className="ide-redesign-toolbar-button-container">
        <Button size="sm" variant="primary" leadingIcon={<MaterialIcon type="person_add" />} onClick={handleOpenShareModal}>
          {t('share')}
        </Button>
      </div>
      <ShareProjectModal show={showShareModal} handleOpen={handleOpenShareModal} handleHide={handleHideShareModal} />
    </>
  )
}

export function BackToEditorButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation()
  return (
    <Button variant="secondary" size="sm" onClick={onClick} className="back-to-editor-btn">
      <MaterialIcon type="arrow_back" className="toolbar-btn-secondary-icon" />
      <span className="toolbar-label">{t('back_to_editor')}</span>
    </Button>
  )
}
