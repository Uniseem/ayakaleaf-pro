'use client'

/**
 * The pieces the rail is made of, from ide-react/components/rail:
 * a tab, an action button or dropdown at the foot, the count badge, the
 * header a panel puts across its top, the help menu, and the overflow menu
 * for tabs that do not fit.
 */

import { forwardRef, useCallback, type ComponentProps, type ReactElement, type ReactNode } from 'react'
import { Tooltip } from '@/components/ol/tooltip'
import MaterialIcon from '@/components/ol/material-icon'
import { IconButton } from '@/components/ol/icon-button'
import { OLBadge } from '@/components/ol/badge'
import { Dropdown, DropdownDivider, DropdownItem, DropdownMenu, DropdownToggle } from '@/components/ol/dropdown'
import type { AvailableUnfilledIcon } from '@/lib/unfilled-symbols'
import { useTranslation } from '@/lib/i18n'
import { useRailContext, type RailTabKey } from '@/features/ide/contexts/rail-context'
import { useSite } from '@/features/ide/contexts/site-context'

export type CustomRailTabIcon = React.FC<{ open: boolean; title: string }>

export type RailElement = {
  icon: AvailableUnfilledIcon | CustomRailTabIcon
  key: RailTabKey
  component: ReactElement | null
  indicator?: ReactElement
  title: string
  hide?: boolean | (() => boolean)
  disabled?: boolean
  mountOnFirstLoad?: boolean
  ref?: React.RefObject<HTMLButtonElement | null>
}

export function shouldIncludeElement({ hide }: { hide?: boolean | (() => boolean) }): boolean {
  if (typeof hide === 'function') {
    return !hide()
  }
  return !hide
}

export const RailTab = forwardRef<
  HTMLButtonElement,
  {
    icon: RailElement['icon']
    eventKey?: string
    open: boolean
    indicator?: ReactElement
    title: string
    disabled?: boolean
    onSelect?: (key: string) => void
  } & Omit<ComponentProps<'button'>, 'onSelect'>
>(function RailTab({ icon, className, eventKey, open, indicator, title, disabled, onSelect, ...props }, ref) {
  return (
    <Tooltip id={`rail-tab-tooltip-${eventKey}`} description={title} overlayProps={{ delay: 0, placement: 'right' }}>
      <button
        {...props}
        ref={ref}
        type="button"
        role="tab"
        aria-selected={open}
        aria-disabled={disabled || undefined}
        disabled={disabled}
        onClick={() => {
          if (!disabled && eventKey) {
            onSelect?.(eventKey)
          }
        }}
        className={['nav-link', 'ide-rail-tab-link', className ?? '', open ? 'open-rail' : '', disabled ? 'disabled' : '']
          .filter(Boolean)
          .join(' ')}
      >
        <RailTabIcon icon={icon} title={title} open={open} />
        {indicator}
      </button>
    </Tooltip>
  )
})

function RailTabIcon({ icon, title, open }: { icon: RailElement['icon']; title: string; open: boolean }) {
  if (typeof icon === 'string') {
    return open ? (
      <MaterialIcon type={icon} className="ide-rail-tab-link-icon" accessibilityLabel={title} />
    ) : (
      <MaterialIcon type={icon} className="ide-rail-tab-link-icon" unfilled accessibilityLabel={title} />
    )
  }
  const Component = icon
  return <Component open={open} title={title} />
}

type RailActionButton = {
  key: string
  icon: AvailableUnfilledIcon
  title: string
  action: () => void
  indicator?: ReactElement
  hide?: boolean
}

type RailDropdown = {
  key: string
  icon: AvailableUnfilledIcon
  title: string
  dropdown: ReactElement
  indicator?: ReactElement
  hide?: boolean
}

export type RailAction = RailDropdown | RailActionButton

export const RailActionElement = forwardRef<HTMLButtonElement, { action: RailAction }>(function RailActionElement(
  { action },
  ref
) {
  const onActionClick = useCallback(() => {
    if ('action' in action) {
      action.action()
    }
  }, [action])

  if (action.hide) {
    return null
  }

  if ('dropdown' in action) {
    return (
      <Dropdown align="end" drop="end">
        <Tooltip id={`rail-dropdown-tooltip-${action.key}`} description={action.title} overlayProps={{ delay: 0, placement: 'right' }}>
          <span>
            <DropdownToggle
              ref={ref as React.Ref<HTMLElement>}
              id={`rail-dropdown-btn-${action.key}`}
              className="ide-rail-tab-link ide-rail-tab-button ide-rail-tab-dropdown"
              as="button"
              aria-label={action.title}
            >
              <RailActionIcon type={action.icon} indicator={action.indicator} />
            </DropdownToggle>
          </span>
        </Tooltip>
        {action.dropdown}
      </Dropdown>
    )
  }

  return (
    <Tooltip id={`rail-tab-tooltip-${action.key}`} description={action.title} overlayProps={{ delay: 0, placement: 'right' }}>
      <button ref={ref} type="button" onClick={onActionClick} className="ide-rail-tab-link ide-rail-tab-button" aria-label={action.title}>
        <RailActionIcon type={action.icon} indicator={action.indicator} />
      </button>
    </Tooltip>
  )
})

function RailActionIcon({ type, indicator }: { type: AvailableUnfilledIcon; indicator?: ReactElement }) {
  return (
    <>
      <MaterialIcon className="ide-rail-tab-link-icon" type={type} unfilled />
      {indicator}
    </>
  )
}

export function RailIndicator({ count, type }: { count: number; type: 'danger' | 'warning' | 'info' | 'success' }) {
  return <OLBadge bg={type}>{count > 99 ? '99+' : Math.floor(count).toString()}</OLBadge>
}

export function RailPanelHeader({ title, actions, onClose }: { title: ReactNode; actions?: ReactElement; onClose?: () => void }) {
  const { t } = useTranslation()
  const { handlePaneCollapse } = useRailContext()

  const handleClose = useCallback(() => {
    handlePaneCollapse()
    onClose?.()
  }, [handlePaneCollapse, onClose])

  return (
    <div className="rail-panel-header">
      <h4 className="rail-panel-title">{title}</h4>
      <div className="rail-panel-header-actions">
        {actions}
        <Tooltip id="close-rail-panel" description={t('close')} overlayProps={{ placement: 'bottom' }}>
          <IconButton onClick={handleClose} className="rail-panel-header-button-subdued" icon="close" accessibilityLabel={t('close')} size="sm" />
        </Tooltip>
      </div>
    </div>
  )
}

export function RailHelpDropdown() {
  const { t } = useTranslation()
  const site = useSite()
  const { setActiveModal } = useRailContext()
  const openKeyboardShortcutsModal = useCallback(() => setActiveModal('keyboard-shortcuts'), [setActiveModal])
  const openContactUsModal = useCallback(() => setActiveModal('contact-us'), [setActiveModal])

  return (
    <DropdownMenu>
      <li role="none">
        <DropdownItem onClick={openKeyboardShortcutsModal}>{t('keyboard_shortcuts')}</DropdownItem>
      </li>
      {site.wikiEnabled ? (
        <li role="none">
          <DropdownItem href="/learn" role="menuitem" target="_blank" rel="noopener noreferrer">
            {t('documentation')}
          </DropdownItem>
        </li>
      ) : null}
      {site.showSupport ? (
        <>
          <DropdownDivider />
          <li role="none">
            <DropdownItem onClick={openContactUsModal}>{t('contact_us')}</DropdownItem>
          </li>
        </>
      ) : null}
    </DropdownMenu>
  )
}

export function RailOverflowDropdown({
  tabs,
  isOpen,
  selectedTab,
  onSelect,
}: {
  tabs: RailElement[]
  isOpen: boolean
  selectedTab: RailTabKey
  onSelect: (key: string) => void
}) {
  return (
    <DropdownMenu className="ide-rail-overflow-dropdown">
      {tabs.filter(shouldIncludeElement).map(({ icon, key, indicator, title, disabled }) => (
        <RailTab
          open={isOpen && selectedTab === key}
          key={key}
          eventKey={key}
          icon={icon}
          indicator={indicator}
          title={title}
          disabled={disabled}
          onSelect={onSelect}
        />
      ))}
    </DropdownMenu>
  )
}
