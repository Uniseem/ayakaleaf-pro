'use client'

/**
 * A toast: a notification that appears over the page for a moment. From
 * shared/components/ol/ol-toast, on top of Bootstrap's toast behaviour: it
 * fades in, hides itself after a delay when asked to, and reports when it
 * has gone so the container can drop it.
 */

import { useCallback, useEffect, useState, type CSSProperties, type ReactNode } from 'react'
import cx from '@/lib/cx'
import { NotificationIcon, type NotificationType } from './notification'
import { useTranslation } from '@/lib/i18n'
import MaterialIcon from './material-icon'

export type OLToastProps = {
  type: NotificationType
  className?: string
  title?: string
  content: string | ReactNode
  isDismissible?: boolean
  onDismiss?: () => void
  autoHide?: boolean
  delay?: number
}

const FADE_DURATION = 150

export const OLToast = ({ type = 'info', className = '', content, title, isDismissible, onDismiss, autoHide, delay = 5000 }: OLToastProps) => {
  const { t } = useTranslation()
  const [show, setShow] = useState(true)
  const [showing, setShowing] = useState(true)

  const toastClassName = cx('notification', `notification-type-${type}`, className, 'toast-content')

  const handleClose = useCallback(() => {
    setShow(false)
  }, [])

  // the fade in
  useEffect(() => {
    const timer = window.setTimeout(() => setShowing(false), 20)
    return () => window.clearTimeout(timer)
  }, [])

  // the automatic hide
  useEffect(() => {
    if (!autoHide || !show) {
      return
    }
    const timer = window.setTimeout(handleClose, delay)
    return () => window.clearTimeout(timer)
  }, [autoHide, delay, show, handleClose])

  // gone once the fade out has finished
  useEffect(() => {
    if (show) {
      return
    }
    const timer = window.setTimeout(() => onDismiss?.(), FADE_DURATION)
    return () => window.clearTimeout(timer)
  }, [show, onDismiss])

  return (
    <div
      className={cx('toast', 'fade', { show, showing })}
      role="alert"
      aria-live="assertive"
      aria-atomic="true"
      style={{ '--toast-fade-duration': `${FADE_DURATION}ms` } as CSSProperties}
    >
      <div className={toastClassName}>
        <NotificationIcon notificationType={type} />

        <div className="notification-content-and-cta">
          <div className="notification-content">
            {title && (
              <p>
                <b>{title}</b>
              </p>
            )}
            {content}
          </div>
        </div>

        {isDismissible && (
          <div className="notification-close-btn">
            <button aria-label={t('close')} onClick={handleClose}>
              <MaterialIcon type="close" />
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

export const OLToastContainer = ({ children, className, style }: { children: ReactNode; className?: string; style?: CSSProperties }) => {
  return (
    <div className={cx('toast-container', className)} style={style}>
      {children}
    </div>
  )
}
