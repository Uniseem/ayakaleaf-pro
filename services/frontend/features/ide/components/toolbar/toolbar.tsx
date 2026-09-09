'use client'

/**
 * The bar across the top of the editor, from ide-react/components/toolbar.
 *
 * Three parts: the logo and the menus on the left, the project's name in the
 * middle, and what you do to the project on the right. History and focus
 * mode each replace the bar with a shorter one.
 */

import { useCallback } from 'react'
import { useTranslation } from '@/lib/i18n'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { Tooltip } from '@/components/ol/tooltip'
import { IconButton } from '@/components/ol/icon-button'
import { ToolbarMenuBar } from './menu-bar'
import { ToolbarProjectTitle } from './project-title'
import { OnlineUsers } from './online-users'
import { BackToEditorButton, ChangeLayoutButton, ShareProjectButton, ShowHistoryButton, ToolbarLogos } from './buttons'

export function Toolbar() {
  const { view, restoreView, focusMode, setFocusMode, pdfLayout, setView } = useLayout()
  const { isRestrictedTokenMember } = useProject()
  const { t } = useTranslation()

  const handleBackToEditorClick = useCallback(() => {
    restoreView()
  }, [restoreView])

  const handleExitFocusMode = useCallback(() => {
    setFocusMode(false)
  }, [setFocusMode])

  const handleSwitchView = useCallback(() => {
    setView(view === 'pdf' ? 'editor' : 'pdf')
  }, [view, setView])

  if (focusMode) {
    const showViewSwitcher = pdfLayout === 'flat'
    const switchTooltip = view === 'pdf' ? t('switch_to_editor') : t('switch_to_pdf')
    const switchIcon = view === 'pdf' ? 'edit' : 'picture_as_pdf'

    return (
      <nav className="ide-redesign-toolbar" aria-label={t('project_actions')}>
        <div className="ide-redesign-toolbar-menu">
          <ToolbarLogos />
        </div>
        <ToolbarProjectTitle />
        <div className="ide-redesign-toolbar-actions">
          {showViewSwitcher ? (
            <div className="ide-redesign-toolbar-button-container">
              <Tooltip id="tooltip-switch-view" description={switchTooltip} overlayProps={{ delay: 0, placement: 'bottom' }}>
                <IconButton
                  icon={switchIcon}
                  className="ide-redesign-toolbar-button-subdued ide-redesign-toolbar-button-icon"
                  onClick={handleSwitchView}
                  accessibilityLabel={switchTooltip}
                />
              </Tooltip>
            </div>
          ) : null}
          <ChangeLayoutButton />
          <div className="ide-redesign-toolbar-button-container">
            <Tooltip id="tooltip-exit-focus-mode" description={t('exit_focus_mode')} overlayProps={{ delay: 0, placement: 'bottom' }}>
              <IconButton
                icon="close_fullscreen"
                className="ide-redesign-toolbar-button-subdued ide-redesign-toolbar-button-icon"
                onClick={handleExitFocusMode}
                accessibilityLabel={t('exit_focus_mode')}
              />
            </Tooltip>
          </div>
        </div>
      </nav>
    )
  }

  if (view === 'history') {
    return (
      <nav className="ide-redesign-toolbar" aria-label={t('project_actions')}>
        <div className="d-flex align-items-center">
          <BackToEditorButton onClick={handleBackToEditorClick} />
        </div>
        <ToolbarProjectTitle />
        <div />
      </nav>
    )
  }

  return (
    <nav className="ide-redesign-toolbar" aria-label={t('project_actions')}>
      <div className="ide-redesign-toolbar-menu">
        <ToolbarLogos />
        <ToolbarMenuBar />
      </div>
      <ToolbarProjectTitle />
      <div className="ide-redesign-toolbar-actions">
        <OnlineUsers />
        {!isRestrictedTokenMember ? <ShowHistoryButton /> : null}
        <ChangeLayoutButton />
        <ShareProjectButton />
      </div>
    </nav>
  )
}
