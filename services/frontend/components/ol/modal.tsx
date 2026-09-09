'use client'

/**
 * Bootstrap's modal, as the original wraps it in shared/components/ol/ol-modal.
 *
 * Rendered into the body: a backdrop and a .modal with a .modal-dialog inside,
 * faded in over a frame. Escape and a click on the backdrop close it unless
 * told otherwise, focus is kept inside while it is open and returned to
 * where it was afterwards, and the page behind stops scrolling.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from '@/lib/i18n'

type ModalContextValue = {
  onHide: () => void
}

const ModalContext = createContext<ModalContextValue | null>(null)

export type OLModalProps = {
  children?: ReactNode
  show?: boolean
  onHide: () => void
  onShow?: () => void
  onExited?: () => void
  size?: 'sm' | 'lg' | 'xl'
  animation?: boolean
  backdrop?: boolean | 'static'
  keyboard?: boolean
  centered?: boolean
  scrollable?: boolean
  className?: string
  dialogClassName?: string
  contentClassName?: string
  id?: string
  'aria-labelledby'?: string
  escapeDeactivates?: boolean
  clickOutsideDeactivates?: boolean
  returnFocusOnDeactivate?: boolean
  initialFocus?: string | HTMLElement | (() => HTMLElement | null)
  enforceFocus?: boolean
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'

/** How many are open, so the body's scroll lock outlives the first to close. */
let openModals = 0

export function OLModal({
  children,
  show = false,
  onHide,
  onShow,
  onExited,
  size,
  animation = true,
  backdrop = true,
  keyboard = true,
  centered,
  scrollable,
  className,
  dialogClassName,
  contentClassName,
  id,
  returnFocusOnDeactivate = true,
  initialFocus,
  'aria-labelledby': labelledBy,
}: OLModalProps) {
  const [mounted, setMounted] = useState(show)
  const [visible, setVisible] = useState(false)
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const contentRef = useRef<HTMLDivElement | null>(null)
  const previouslyFocused = useRef<HTMLElement | null>(null)
  const mouseDownOnBackdrop = useRef(false)

  // Mount, then show a frame later so the fade has something to fade from.
  useEffect(() => {
    if (show) {
      setMounted(true)
      onShow?.()
      const frame = window.requestAnimationFrame(() => setVisible(true))
      return () => window.cancelAnimationFrame(frame)
    }
    setVisible(false)
    if (!mounted) {
      return
    }
    const timer = window.setTimeout(
      () => {
        setMounted(false)
        onExited?.()
      },
      animation ? 300 : 0
    )
    return () => window.clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [show, animation])

  // Body scroll lock and the focus bookkeeping.
  useLayoutEffect(() => {
    if (!mounted) {
      return
    }
    openModals += 1
    document.body.classList.add('modal-open')
    document.body.style.overflow = 'hidden'
    previouslyFocused.current = document.activeElement as HTMLElement | null
    return () => {
      openModals -= 1
      if (openModals === 0) {
        document.body.classList.remove('modal-open')
        document.body.style.overflow = ''
      }
      if (returnFocusOnDeactivate) {
        previouslyFocused.current?.focus?.()
      }
    }
  }, [mounted, returnFocusOnDeactivate])

  // Initial focus: what was asked for, else the first focusable, else the
  // dialog itself so that Escape works.
  useEffect(() => {
    if (!visible) {
      return
    }
    const content = contentRef.current
    if (!content) {
      return
    }
    let target: HTMLElement | null = null
    if (typeof initialFocus === 'function') {
      target = initialFocus()
    } else if (typeof initialFocus === 'string') {
      target = content.querySelector<HTMLElement>(initialFocus)
    } else if (initialFocus) {
      target = initialFocus
    }
    if (!target) {
      target = content.querySelector<HTMLElement>('[autofocus]')
    }
    if (!target) {
      target = content.querySelector<HTMLElement>(FOCUSABLE)
    }
    ;(target ?? dialogRef.current)?.focus()
  }, [visible, initialFocus])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (event.key === 'Escape') {
        if (keyboard) {
          event.stopPropagation()
          onHide()
        }
        return
      }
      if (event.key === 'Tab') {
        const content = contentRef.current
        if (!content) {
          return
        }
        const focusable = Array.from(content.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
          element => element.offsetParent !== null
        )
        if (focusable.length === 0) {
          event.preventDefault()
          return
        }
        const first = focusable[0]!
        const last = focusable[focusable.length - 1]!
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault()
          last.focus()
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault()
          first.focus()
        }
      }
    },
    [keyboard, onHide]
  )

  if (!mounted || typeof document === 'undefined') {
    return null
  }

  const modal = (
    <ModalContext.Provider value={{ onHide }}>
      <div
        className={['modal-backdrop', animation ? 'fade' : '', visible ? 'show' : ''].filter(Boolean).join(' ')}
      />
      <div
        ref={dialogRef}
        id={id}
        className={['modal', animation ? 'fade' : '', visible ? 'show' : '', className ?? '']
          .filter(Boolean)
          .join(' ')}
        role="dialog"
        aria-modal="true"
        aria-labelledby={labelledBy}
        tabIndex={-1}
        style={{ display: 'block' }}
        onKeyDown={handleKeyDown}
        onMouseDown={event => {
          mouseDownOnBackdrop.current = event.target === event.currentTarget
        }}
        onClick={event => {
          if (event.target === event.currentTarget && mouseDownOnBackdrop.current && backdrop !== 'static') {
            onHide()
          }
          mouseDownOnBackdrop.current = false
        }}
      >
        <div
          className={[
            'modal-dialog',
            size ? `modal-${size}` : '',
            centered ? 'modal-dialog-centered' : '',
            scrollable ? 'modal-dialog-scrollable' : '',
            dialogClassName ?? '',
          ]
            .filter(Boolean)
            .join(' ')}
        >
          <div ref={contentRef} className={['modal-content', contentClassName ?? ''].filter(Boolean).join(' ')}>
            {children}
          </div>
        </div>
      </div>
    </ModalContext.Provider>
  )

  return createPortal(modal, document.body)
}

export function OLModalHeader({
  children,
  closeButton = true,
  className,
}: {
  children?: ReactNode
  closeButton?: boolean
  className?: string
}) {
  const { t } = useTranslation()
  const context = useContext(ModalContext)
  return (
    <div className={['modal-header', className ?? ''].filter(Boolean).join(' ')}>
      {children}
      {closeButton ? (
        <button type="button" className="btn-close" aria-label={t('close_dialog')} onClick={() => context?.onHide()} />
      ) : null}
    </div>
  )
}

export function OLModalTitle({
  children,
  className,
  id,
}: {
  children?: ReactNode
  className?: string
  id?: string
}) {
  return (
    <h2 id={id} className={['modal-title', className ?? ''].filter(Boolean).join(' ')}>
      {children}
    </h2>
  )
}

export function OLModalBody({ children, className }: { children?: ReactNode; className?: string }) {
  return <div className={['modal-body', className ?? ''].filter(Boolean).join(' ')}>{children}</div>
}

export function OLModalFooter({ children, className }: { children?: ReactNode; className?: string }) {
  return <div className={['modal-footer', className ?? ''].filter(Boolean).join(' ')}>{children}</div>
}
