'use client'

/**
 * The notification box, from shared/components/notification.tsx: an icon for
 * the kind, the content, an optional action beside or below it, and a close
 * button when it can be dismissed.
 */

import { useState, type ReactElement, type ReactNode } from 'react'
import MaterialIcon from './material-icon'
import { useTranslation } from '@/lib/i18n'

export type NotificationType = 'info' | 'success' | 'warning' | 'error' | 'offer'

export type NotificationProps = {
  action?: ReactElement
  ariaLive?: 'polite' | 'off' | 'assertive'
  className?: string
  content: ReactNode
  customIcon?: ReactElement | null
  iconPlacement?: 'top' | 'center'
  disclaimer?: ReactElement | string
  isDismissible?: boolean
  isActionBelowContent?: boolean
  onDismiss?: () => void
  title?: ReactNode
  type: NotificationType
  id?: string
}

export function NotificationIcon({
  notificationType,
  customIcon,
  iconPlacement,
}: {
  notificationType: NotificationType
  customIcon?: ReactElement
  iconPlacement?: 'top' | 'center'
}) {
  let icon = <MaterialIcon type="info" />
  if (customIcon) {
    icon = customIcon
  } else if (notificationType === 'success') {
    icon = <MaterialIcon type="check_circle" />
  } else if (notificationType === 'warning') {
    icon = <MaterialIcon type="warning" />
  } else if (notificationType === 'error') {
    icon = <MaterialIcon type="error" />
  } else if (notificationType === 'offer') {
    icon = <MaterialIcon type="campaign" />
  }
  return (
    <div className={['notification-icon', iconPlacement ? `notification-icon-${iconPlacement}` : ''].filter(Boolean).join(' ')}>
      {icon}
    </div>
  )
}

export function Notification({
  action,
  ariaLive,
  className = '',
  content,
  customIcon,
  iconPlacement = 'top',
  disclaimer,
  isActionBelowContent,
  isDismissible,
  onDismiss,
  title,
  type,
  id,
}: NotificationProps) {
  const { t } = useTranslation()
  const [show, setShow] = useState(true)

  if (!show) {
    return null
  }

  return (
    <div
      className={['notification', `notification-type-${type || 'info'}`, isActionBelowContent ? 'notification-cta-below-content' : '', className]
        .filter(Boolean)
        .join(' ')}
      aria-live={ariaLive || 'off'}
      role="alert"
      id={id}
    >
      {customIcon !== null ? (
        <NotificationIcon notificationType={type || 'info'} customIcon={customIcon} iconPlacement={iconPlacement} />
      ) : null}

      <div className="notification-content-and-cta">
        <div className="notification-content">
          {title ? (
            typeof title === 'string' ? (
              <p>
                <b>{title}</b>
              </p>
            ) : (
              title
            )
          ) : null}
          {content}
        </div>
        {action ? <div className="notification-cta">{action}</div> : null}
        {disclaimer ? <div className="notification-disclaimer">{disclaimer}</div> : null}
      </div>

      {isDismissible ? (
        <div className="notification-close-btn">
          <button
            type="button"
            aria-label={t('close')}
            onClick={() => {
              setShow(false)
              onDismiss?.()
            }}
          >
            <MaterialIcon type="close" />
          </button>
        </div>
      ) : null}
    </div>
  )
}

/** The original's OLNotification, which is the same thing. */
export const OLNotification = Notification

export default Notification
