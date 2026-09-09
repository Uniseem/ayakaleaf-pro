'use client'

/**
 * A Bootstrap tooltip, from shared/components/tooltip.tsx.
 *
 * Shown on hover and focus after a short delay, hidden on the same with a
 * slightly shorter one so that moving between two neighbouring buttons does
 * not flicker; hidden at once on click, because the click did the thing the
 * tooltip was describing; and dismissed with Escape. The bubble is rendered
 * into the body and positioned from the trigger's box, which is what Popper
 * did for the original.
 */

import {
  cloneElement,
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
} from 'react'
import { createPortal } from 'react-dom'

const DEFAULT_DELAY_SHOW = 300
const DEFAULT_DELAY_HIDE = 290

export type Placement = 'top' | 'bottom' | 'left' | 'right'

export type TooltipProps = {
  description: ReactNode
  id: string
  overlayProps?: {
    placement?: Placement
    delay?: number | { show: number; hide: number }
    trigger?: 'click' | Array<'hover' | 'focus' | 'click'>
  }
  tooltipProps?: { className?: string }
  hidden?: boolean
  children: ReactElement<Record<string, unknown>>
}

const bsPlacement: Record<Placement, string> = {
  top: 'top',
  bottom: 'bottom',
  left: 'start',
  right: 'end',
}

const popperPlacement: Record<Placement, string> = {
  top: 'top',
  bottom: 'bottom',
  left: 'left',
  right: 'right',
}

type Box = { top: number; left: number }

export function Tooltip({
  id,
  description,
  children,
  tooltipProps,
  overlayProps,
  hidden,
}: TooltipProps) {
  const [show, setShow] = useState(false)
  const [box, setBox] = useState<Box | null>(null)
  const [arrow, setArrow] = useState<number | null>(null)
  const triggerRef = useRef<HTMLElement | null>(null)
  const bubbleRef = useRef<HTMLDivElement | null>(null)
  const timer = useRef<number | null>(null)
  const generated = useId()

  const placement = overlayProps?.placement ?? 'top'
  const delay = overlayProps?.delay
  let delayShow = DEFAULT_DELAY_SHOW
  let delayHide = DEFAULT_DELAY_HIDE
  if (delay !== undefined) {
    delayShow = typeof delay === 'number' ? delay : delay.show
    delayHide = typeof delay === 'number' ? Math.max(delay - 10, 0) : delay.hide
  }
  const clickOnly = overlayProps?.trigger === 'click'

  const clearTimer = () => {
    if (timer.current !== null) {
      window.clearTimeout(timer.current)
      timer.current = null
    }
  }

  const open = useCallback(() => {
    clearTimer()
    timer.current = window.setTimeout(() => setShow(true), delayShow)
  }, [delayShow])

  const close = useCallback(() => {
    clearTimer()
    timer.current = window.setTimeout(() => setShow(false), delayHide)
  }, [delayHide])

  useEffect(() => clearTimer, [])

  useEffect(() => {
    if (hidden) {
      setShow(false)
    }
  }, [hidden])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (show && event.key === 'Escape') {
        setShow(false)
        event.stopPropagation()
      }
    }
    document.addEventListener('keydown', handleKeyDown, true)
    return () => document.removeEventListener('keydown', handleKeyDown, true)
  }, [show])

  // Where the bubble goes: worked out once it exists, from both boxes.
  useLayoutEffect(() => {
    if (!show) {
      setBox(null)
      return
    }
    const trigger = triggerRef.current
    const bubble = bubbleRef.current
    if (!trigger || !bubble) {
      return
    }
    const place = () => {
      const t = trigger.getBoundingClientRect()
      const b = bubble.getBoundingClientRect()
      const gap = 0
      let top = 0
      let left = 0
      switch (placement) {
        case 'top':
          top = t.top - b.height - gap
          left = t.left + t.width / 2 - b.width / 2
          break
        case 'bottom':
          top = t.bottom + gap
          left = t.left + t.width / 2 - b.width / 2
          break
        case 'left':
          top = t.top + t.height / 2 - b.height / 2
          left = t.left - b.width - gap
          break
        case 'right':
          top = t.top + t.height / 2 - b.height / 2
          left = t.right + gap
          break
      }
      // Keep it on screen, and let the arrow slide to stay on the trigger.
      const margin = 4
      const maxLeft = window.innerWidth - b.width - margin
      const maxTop = window.innerHeight - b.height - margin
      const clampedLeft = Math.min(Math.max(left, margin), maxLeft)
      const clampedTop = Math.min(Math.max(top, margin), maxTop)
      if (placement === 'top' || placement === 'bottom') {
        setArrow(t.left + t.width / 2 - clampedLeft - 6.4)
      } else {
        setArrow(t.top + t.height / 2 - clampedTop - 6.4)
      }
      setBox({ top: clampedTop + window.scrollY, left: clampedLeft + window.scrollX })
    }
    place()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [show, placement, description])

  const child = children
  const childProps = child.props as Record<string, unknown>

  const chain =
    (...handlers: Array<unknown>) =>
    (...args: unknown[]) => {
      for (const handler of handlers) {
        if (typeof handler === 'function') {
          handler(...args)
        }
      }
    }

  const setRef = (node: HTMLElement | null) => {
    triggerRef.current = node
    const ref = (child as unknown as { ref?: unknown }).ref
    if (typeof ref === 'function') {
      ref(node)
    } else if (ref && typeof ref === 'object') {
      ;(ref as { current: HTMLElement | null }).current = node
    }
  }

  const hideTooltip = (event: React.MouseEvent) => {
    if (event.currentTarget instanceof HTMLElement) {
      event.currentTarget.blur()
    }
    clearTimer()
    setShow(false)
  }

  const trigger = cloneElement(child, {
    ref: setRef,
    'aria-describedby': show ? `${id}-tooltip` : childProps['aria-describedby'],
    ...(clickOnly
      ? {
          onClick: chain(childProps.onClick, () => setShow(value => !value)),
        }
      : {
          onMouseEnter: chain(childProps.onMouseEnter, open),
          onMouseLeave: chain(childProps.onMouseLeave, close),
          onFocus: chain(childProps.onFocus, open),
          onBlur: chain(childProps.onBlur, close),
          onClick: chain(childProps.onClick, hideTooltip),
        }),
  })

  const visible = show && !hidden

  return (
    <>
      {trigger}
      {visible && typeof document !== 'undefined'
        ? createPortal(
            <div
              ref={bubbleRef}
              id={`${id}-tooltip`}
              role="tooltip"
              className={[
                'tooltip',
                'fade',
                `bs-tooltip-${bsPlacement[placement]}`,
                box ? 'show' : '',
                tooltipProps?.className ?? '',
              ]
                .filter(Boolean)
                .join(' ')}
              data-popper-placement={popperPlacement[placement]}
              style={{
                position: 'absolute',
                top: box?.top ?? 0,
                left: box?.left ?? 0,
                display: hidden ? 'none' : 'block',
              }}
              key={generated}
            >
              <div
                className="tooltip-arrow"
                style={
                  placement === 'top' || placement === 'bottom'
                    ? { position: 'absolute', left: arrow ?? 0 }
                    : { position: 'absolute', top: arrow ?? 0 }
                }
              />
              <div className="tooltip-inner">{description}</div>
            </div>,
            document.body
          )
        : null}
    </>
  )
}

export default Tooltip
