'use client'

/**
 * A popover anchored to an element, the way react-bootstrap's Overlay and
 * Popover pair works: shown below (or above) a target, contained inside a
 * given element, and closed by a click or a press of Escape outside it.
 */

import {
  forwardRef,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
  type Ref,
} from 'react'
import { createPortal } from 'react-dom'
import cx from '@/lib/cx'

export type OverlayPlacement = 'top' | 'bottom' | 'left' | 'right'

export type OverlayProps = {
  show: boolean
  target: Element | null | undefined
  placement?: OverlayPlacement
  /** Where the popover is put in the DOM. Defaults to the body. */
  container?: Element | null
  containerPadding?: number
  rootClose?: boolean
  transition?: boolean
  onHide?: () => void
  onEntered?: () => void
  children: ReactNode
}

type Box = { top: number; left: number; placement: OverlayPlacement }

function setRef<T>(ref: Ref<T> | undefined, value: T) {
  if (typeof ref === 'function') {
    ref(value)
  } else if (ref) {
    ;(ref as { current: T | null }).current = value
  }
}

export function Overlay({
  show,
  target,
  placement = 'bottom',
  container,
  containerPadding = 8,
  rootClose,
  onHide,
  onEntered,
  children,
}: OverlayProps) {
  const [box, setBox] = useState<Box | null>(null)
  const popoverRef = useRef<HTMLDivElement | null>(null)
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  const parent = container ?? (mounted ? document.body : null)

  const measure = useCallback(() => {
    if (!show || !target || !parent || !popoverRef.current) {
      return
    }
    const targetRect = target.getBoundingClientRect()
    const parentRect = parent.getBoundingClientRect()
    const popoverRect = popoverRef.current.getBoundingClientRect()
    // relative to the container's padding box, allowing for its scroll
    const originLeft = parentRect.left - parent.scrollLeft
    const originTop = parentRect.top - parent.scrollTop

    let actual = placement
    let top = 0
    let left = 0

    const below = targetRect.bottom - originTop
    const above = targetRect.top - originTop - popoverRect.height
    const centred = targetRect.left - originLeft + targetRect.width / 2 - popoverRect.width / 2

    switch (placement) {
      case 'top':
        top = above
        left = centred
        if (targetRect.top - popoverRect.height < parentRect.top) {
          top = below
          actual = 'bottom'
        }
        break
      case 'left':
        top = targetRect.top - originTop + targetRect.height / 2 - popoverRect.height / 2
        left = targetRect.left - originLeft - popoverRect.width
        break
      case 'right':
        top = targetRect.top - originTop + targetRect.height / 2 - popoverRect.height / 2
        left = targetRect.right - originLeft
        break
      default:
        top = below
        left = centred
        if (targetRect.bottom + popoverRect.height > parentRect.bottom && targetRect.top - popoverRect.height >= parentRect.top) {
          top = above
          actual = 'top'
        }
    }

    // keep it inside the container horizontally
    const maxLeft = parentRect.width - popoverRect.width - containerPadding
    left = Math.max(containerPadding, Math.min(left, maxLeft))

    setBox(previous =>
      previous && previous.top === top && previous.left === left && previous.placement === actual
        ? previous
        : { top, left, placement: actual }
    )
  }, [show, target, parent, placement, containerPadding])

  useLayoutEffect(() => {
    if (show) {
      measure()
    } else {
      setBox(null)
    }
  }, [show, measure, children])

  useEffect(() => {
    if (!show) {
      return
    }
    window.addEventListener('resize', measure)
    window.addEventListener('scroll', measure, true)
    return () => {
      window.removeEventListener('resize', measure)
      window.removeEventListener('scroll', measure, true)
    }
  }, [show, measure])

  useEffect(() => {
    if (show && box) {
      onEntered?.()
    }
    // only when it first appears
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [show, box !== null])

  // close on a click or Escape outside the popover and its target
  useEffect(() => {
    if (!show || !rootClose) {
      return
    }
    const onMouseDown = (event: MouseEvent) => {
      const node = event.target as Node
      if (popoverRef.current?.contains(node) || target?.contains(node)) {
        return
      }
      onHide?.()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onHide?.()
      }
    }
    document.addEventListener('mousedown', onMouseDown, true)
    document.addEventListener('keydown', onKeyDown, true)
    return () => {
      document.removeEventListener('mousedown', onMouseDown, true)
      document.removeEventListener('keydown', onKeyDown, true)
    }
  }, [show, rootClose, onHide, target])

  if (!show || !parent) {
    return null
  }

  const style: CSSProperties = {
    position: 'absolute',
    top: box?.top ?? 0,
    left: box?.left ?? 0,
    visibility: box ? 'visible' : 'hidden',
  }

  return createPortal(
    <OverlayPositioner ref={popoverRef} style={style} placement={box?.placement ?? placement}>
      {children}
    </OverlayPositioner>,
    parent
  )
}

const OverlayPositioner = forwardRef<HTMLDivElement, { style: CSSProperties; placement: OverlayPlacement; children: ReactNode }>(
  function OverlayPositioner({ style, placement, children }, ref) {
    return (
      <div ref={ref} style={style} data-popper-placement={placement} className="ol-overlay">
        {children}
      </div>
    )
  }
)

export type PopoverProps = {
  id?: string
  className?: string
  role?: string
  placement?: OverlayPlacement
  children: ReactNode
}

const bsPlacement: Record<OverlayPlacement, string> = {
  top: 'top',
  bottom: 'bottom',
  left: 'start',
  right: 'end',
}

export const Popover = forwardRef<HTMLDivElement, PopoverProps>(function Popover(
  { id, className, role, placement = 'bottom', children },
  ref
) {
  return (
    <div
      ref={node => setRef(ref, node)}
      id={id}
      role={role ?? 'tooltip'}
      className={cx('popover', `bs-popover-${bsPlacement[placement]}`, className)}
    >
      <div className="popover-arrow" />
      <div className="popover-body">{children}</div>
    </div>
  )
})

export default Overlay
