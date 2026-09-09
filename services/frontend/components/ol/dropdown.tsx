'use client'

/**
 * Bootstrap's dropdown, as the original wraps it in
 * shared/components/dropdown/dropdown-menu.tsx.
 *
 * The markup and class names are Bootstrap's -- .dropdown, .dropdown-toggle,
 * ul.dropdown-menu, .dropdown-item -- so the ported stylesheet applies. The
 * behaviour is react-bootstrap's: the toggle opens it, a click outside or
 * Escape closes it, arrow keys move between items, an item's eventKey goes
 * to the Dropdown's onSelect, and the menu closes after a selection unless
 * told not to. The menu sits under (or beside) the toggle without Popper:
 * it is positioned by the static rules, and flipped the other way when it
 * would run off the screen.
 */

import {
  createContext,
  forwardRef,
  useCallback,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ElementType,
  type MouseEventHandler,
  type ReactNode,
} from 'react'
import MaterialIcon from './material-icon'

type Align = 'start' | 'end'
type Drop = 'down' | 'up' | 'end' | 'start'

type DropdownContextValue = {
  id: string
  show: boolean
  setShow: (show: boolean) => void
  align: Align
  drop: Drop
  autoClose: boolean | 'inside' | 'outside'
  onSelect?: (eventKey: string | undefined, event: React.SyntheticEvent) => void
  toggleRef: React.RefObject<HTMLElement | null>
  menuRef: React.RefObject<HTMLElement | null>
  rootRef: React.RefObject<HTMLElement | null>
  registerToggle: () => () => void
  hasToggle: boolean
  focusFirstItemOnShow: boolean | 'keyboard'
  openedByKeyboard: React.MutableRefObject<boolean>
}

const DropdownContext = createContext<DropdownContextValue | null>(null)

export type DropdownProps = {
  align?: Align | { sm: Align } | { md: Align } | { lg: Align } | { xl: Align }
  as?: ElementType
  children: ReactNode
  className?: string
  onSelect?: (eventKey: string | undefined, event: React.SyntheticEvent) => void
  onToggle?: (show: boolean) => void
  show?: boolean
  autoClose?: boolean | 'inside' | 'outside'
  drop?: Drop
  focusFirstItemOnShow?: false | true | 'keyboard'
  onKeyDown?: (event: React.KeyboardEvent) => void
  role?: string
  id?: string
}

function resolveAlign(align: DropdownProps['align']): Align {
  if (!align) return 'start'
  if (typeof align === 'string') return align
  return Object.values(align)[0] ?? 'start'
}

export function Dropdown({
  align,
  as: Component = 'div',
  children,
  className,
  onSelect,
  onToggle,
  show: controlledShow,
  autoClose = true,
  drop = 'down',
  focusFirstItemOnShow = 'keyboard',
  onKeyDown,
  role,
  id: givenId,
}: DropdownProps) {
  const generated = useId()
  const id = givenId ?? generated
  const [ownShow, setOwnShow] = useState(false)
  const controlled = controlledShow !== undefined
  const show = controlled ? controlledShow : ownShow
  const toggleRef = useRef<HTMLElement | null>(null)
  const menuRef = useRef<HTMLElement | null>(null)
  const rootRef = useRef<HTMLElement | null>(null)
  const openedByKeyboard = useRef(false)
  const [toggleCount, setToggleCount] = useState(0)

  const setShow = useCallback(
    (next: boolean) => {
      if (!controlled) {
        setOwnShow(next)
      }
      onToggle?.(next)
    },
    [controlled, onToggle]
  )

  const registerToggle = useCallback(() => {
    setToggleCount(count => count + 1)
    return () => setToggleCount(count => count - 1)
  }, [])

  // A click outside closes it. Listening on mousedown rather than click
  // means a drag that starts outside also closes it, as Bootstrap does.
  useEffect(() => {
    if (!show) {
      return
    }
    const handle = (event: MouseEvent) => {
      const target = event.target as Node
      if (rootRef.current?.contains(target) || menuRef.current?.contains(target)) {
        return
      }
      if (autoClose === true || autoClose === 'outside') {
        setShow(false)
      }
    }
    document.addEventListener('mousedown', handle, true)
    return () => document.removeEventListener('mousedown', handle, true)
  }, [show, autoClose, setShow])

  const handleKeyDown = (event: React.KeyboardEvent) => {
    onKeyDown?.(event)
    if (event.defaultPrevented) {
      return
    }
    const items = () =>
      Array.from(
        menuRef.current?.querySelectorAll<HTMLElement>(
          '.dropdown-item:not(.disabled):not([aria-disabled="true"]):not(:disabled)'
        ) ?? []
      )
    switch (event.key) {
      case 'Escape':
        if (show) {
          event.preventDefault()
          event.stopPropagation()
          setShow(false)
          toggleRef.current?.focus()
        }
        break
      case 'ArrowDown':
      case 'ArrowUp': {
        event.preventDefault()
        if (!show) {
          openedByKeyboard.current = true
          setShow(true)
          return
        }
        const list = items()
        if (list.length === 0) {
          return
        }
        const index = list.indexOf(document.activeElement as HTMLElement)
        const next =
          event.key === 'ArrowDown'
            ? list[Math.min(index + 1, list.length - 1)]
            : list[Math.max(index - 1, 0)]
        next?.focus()
        break
      }
      case 'Tab':
        if (show && (autoClose === true || autoClose === 'outside')) {
          setShow(false)
        }
        break
    }
  }

  const value = useMemo<DropdownContextValue>(
    () => ({
      id,
      show,
      setShow,
      align: resolveAlign(align),
      drop,
      autoClose,
      onSelect,
      toggleRef,
      menuRef,
      rootRef,
      registerToggle,
      hasToggle: toggleCount > 0,
      focusFirstItemOnShow,
      openedByKeyboard,
    }),
    [id, show, setShow, align, drop, autoClose, onSelect, registerToggle, toggleCount, focusFirstItemOnShow]
  )

  return (
    <DropdownContext.Provider value={value}>
      <Component
        ref={rootRef}
        className={[
          drop === 'end' ? 'dropend' : drop === 'up' ? 'dropup' : drop === 'start' ? 'dropstart' : 'dropdown',
          show ? 'show' : '',
          className ?? '',
        ]
          .filter(Boolean)
          .join(' ')}
        onKeyDown={handleKeyDown}
        role={role}
      >
        {children}
      </Component>
    </DropdownContext.Provider>
  )
}

export function useDropdownContext() {
  return useContext(DropdownContext)
}

export type DropdownToggleProps = {
  children?: ReactNode
  className?: string
  disabled?: boolean
  split?: boolean
  id?: string
  variant?: 'primary' | 'secondary' | 'danger' | 'link' | 'ghost'
  as?: ElementType
  size?: 'sm' | 'lg'
  tabIndex?: number
  'aria-label'?: string
  onMouseEnter?: MouseEventHandler
  onClick?: MouseEventHandler
  bsPrefix?: string
  title?: string
}

export const DropdownToggle = forwardRef<HTMLElement, DropdownToggleProps>(
  function DropdownToggle(
    { children, className, disabled, id, variant, as: Component = 'button', size, onClick, bsPrefix, split, ...props },
    ref
  ) {
    const context = useContext(DropdownContext)
    const registerToggle = context?.registerToggle
    useLayoutEffect(() => registerToggle?.(), [registerToggle])

    const setRef = (node: HTMLElement | null) => {
      if (context) {
        context.toggleRef.current = node
      }
      if (typeof ref === 'function') {
        ref(node)
      } else if (ref) {
        ref.current = node
      }
    }

    const isButton = Component === 'button'
    return (
      <Component
        ref={setRef}
        id={id ?? (context ? `${context.id}-toggle` : undefined)}
        type={isButton ? 'button' : undefined}
        className={[
          bsPrefix ?? 'dropdown-toggle',
          split ? 'dropdown-toggle-split' : '',
          isButton && !className?.includes('ide-rail-tab-link') ? 'btn' : '',
          variant ? `btn-${variant}` : '',
          size ? `btn-${size}` : '',
          context?.show ? 'show' : '',
          className ?? '',
        ]
          .filter(Boolean)
          .join(' ')}
        aria-expanded={context?.show ?? false}
        aria-haspopup="true"
        disabled={disabled}
        onClick={(event: React.MouseEvent) => {
          onClick?.(event)
          if (event.defaultPrevented || disabled) {
            return
          }
          if (context) {
            context.openedByKeyboard.current = false
            context.setShow(!context.show)
          }
        }}
        {...props}
      >
        {children}
      </Component>
    )
  }
)

export type DropdownMenuProps = {
  as?: ElementType
  children?: ReactNode
  className?: string
  id?: string
  renderOnMount?: boolean
  show?: boolean
  flip?: boolean
  tabIndex?: number
  onKeyDown?: (event: React.KeyboardEvent) => void
  role?: string
  'aria-labelledby'?: string
  popperConfig?: unknown
  disabled?: boolean
}

export const DropdownMenu = forwardRef<HTMLElement, DropdownMenuProps>(
  function DropdownMenu(
    { as: Component = 'ul', children, className, id, renderOnMount, show: forcedShow, flip = true, role = 'menu', ...props },
    ref
  ) {
    const context = useContext(DropdownContext)
    const show = forcedShow ?? context?.show ?? false
    const [flipped, setFlipped] = useState(false)
    const inner = useRef<HTMLElement | null>(null)

    const setRef = (node: HTMLElement | null) => {
      inner.current = node
      if (context) {
        context.menuRef.current = node
      }
      if (typeof ref === 'function') {
        ref(node)
      } else if (ref) {
        ref.current = node
      }
    }

    // Flip when the menu would leave the screen, which is what Popper did.
    useLayoutEffect(() => {
      if (!show || !inner.current || !flip) {
        setFlipped(false)
        return
      }
      const box = inner.current.getBoundingClientRect()
      const drop = context?.drop ?? 'down'
      if (drop === 'down') {
        setFlipped(box.bottom > window.innerHeight && box.height < (context?.toggleRef.current?.getBoundingClientRect().top ?? 0))
      } else if (drop === 'end') {
        setFlipped(box.right > window.innerWidth)
      } else {
        setFlipped(false)
      }
    }, [show, flip, context])

    // Focus the first item when opened from the keyboard.
    useEffect(() => {
      if (!show || !context) {
        return
      }
      const wanted =
        context.focusFirstItemOnShow === true ||
        (context.focusFirstItemOnShow === 'keyboard' && context.openedByKeyboard.current)
      if (wanted) {
        const first = inner.current?.querySelector<HTMLElement>(
          '.dropdown-item:not(.disabled):not([aria-disabled="true"])'
        )
        first?.focus()
      }
    }, [show, context])

    if (!show && !renderOnMount) {
      return null
    }

    const drop = context?.drop ?? 'down'
    const align = context?.align ?? 'start'
    const placement =
      drop === 'end'
        ? flipped
          ? 'left-start'
          : 'right-start'
        : drop === 'up' || flipped
          ? `top-${align}`
          : `bottom-${align}`

    const style: React.CSSProperties =
      drop === 'down' && flipped
        ? { top: 'auto', bottom: '100%', marginBottom: 'var(--spacing-01)' }
        : drop === 'end' && flipped
          ? { left: 'auto', right: '100%', marginRight: 'var(--spacing-01)' }
          : {}

    return (
      <Component
        ref={setRef}
        id={id}
        role={role}
        className={[
          'dropdown-menu',
          show ? 'show' : '',
          align === 'end' ? 'dropdown-menu-end' : '',
          context?.hasToggle ? 'dropdown-menu-popper' : '',
          className ?? '',
        ]
          .filter(Boolean)
          .join(' ')}
        data-bs-popper="static"
        data-popper-placement={placement}
        style={style}
        aria-labelledby={props['aria-labelledby'] ?? (context ? `${context.id}-toggle` : undefined)}
        tabIndex={props.tabIndex}
        onKeyDown={props.onKeyDown}
      >
        {children}
      </Component>
    )
  }
)

export type DropdownItemProps = {
  active?: boolean
  as?: ElementType
  type?: string
  description?: ReactNode
  disabled?: boolean
  eventKey?: string | number
  href?: string
  leadingIcon?: string | ReactNode
  onClick?: MouseEventHandler
  onMouseEnter?: MouseEventHandler
  trailingIcon?: string | ReactNode
  variant?: 'default' | 'danger'
  className?: string
  role?: string
  tabIndex?: number
  target?: string
  download?: boolean | string
  rel?: string
  translate?: 'yes' | 'no'
  children?: ReactNode
  'aria-current'?: boolean
  'data-testid'?: string
  id?: string
  title?: string
}

function DropdownItemInner(
  {
    active,
    children,
    className,
    description,
    leadingIcon,
    trailingIcon,
    as,
    eventKey,
    href,
    disabled,
    onClick,
    variant,
    role = 'menuitem',
    type,
    ...props
  }: DropdownItemProps,
  ref: React.ForwardedRef<HTMLElement>
) {
  const context = useContext(DropdownContext)

  let leadingIconComponent: ReactNode = null
  if (leadingIcon) {
    leadingIconComponent =
      typeof leadingIcon === 'string' ? (
        <MaterialIcon className="dropdown-item-leading-icon" type={leadingIcon} />
      ) : (
        <span className="dropdown-item-leading-icon" aria-hidden="true">
          {leadingIcon}
        </span>
      )
  }

  let trailingIconComponent: ReactNode = null
  if (trailingIcon) {
    if (typeof trailingIcon === 'string') {
      const trailingIconType = active ? 'check' : trailingIcon
      trailingIconComponent = (
        <MaterialIcon className="dropdown-item-trailing-icon" type={trailingIconType} />
      )
    } else {
      trailingIconComponent = (
        <span className="dropdown-item-trailing-icon" aria-hidden="true">
          {trailingIcon}
        </span>
      )
    }
  }

  const Component: ElementType = as ?? (href ? 'a' : 'button')
  const isButton = Component === 'button'

  const handleClick = (event: React.MouseEvent) => {
    if (disabled) {
      event.preventDefault()
      return
    }
    onClick?.(event)
    if (event.defaultPrevented) {
      return
    }
    if (context) {
      context.onSelect?.(eventKey === undefined ? undefined : String(eventKey), event)
      if (context.autoClose === true || context.autoClose === 'inside') {
        context.setShow(false)
      }
    }
  }

  return (
    <Component
      ref={ref}
      role={role}
      href={href}
      type={isButton ? type ?? 'button' : type}
      className={[
        'dropdown-item',
        active ? 'active' : '',
        disabled ? 'disabled' : '',
        className ?? '',
      ]
        .filter(Boolean)
        .join(' ')}
      aria-disabled={disabled || undefined}
      disabled={isButton ? disabled : undefined}
      onClick={handleClick}
      {...(variant === 'danger' ? { variant: 'danger' } : {})}
      {...props}
    >
      {leadingIconComponent}
      {description ? (
        <span className="dropdown-item-description-container">
          {children}
          <span className="dropdown-item-description">{description}</span>
        </span>
      ) : (
        children
      )}
      {trailingIconComponent}
    </Component>
  )
}

function EmptyLeadingIcon() {
  return <span className="dropdown-item-leading-icon-empty" />
}

export const DropdownItem = Object.assign(forwardRef(DropdownItemInner), {
  EmptyLeadingIcon,
})

export function DropdownDivider({ as: Component = 'li', className }: { as?: ElementType; className?: string }) {
  return (
    <Component role="separator" className={['dropdown-divider', className ?? ''].filter(Boolean).join(' ')} />
  )
}

export function DropdownHeader({
  as: Component = 'li',
  className,
  children,
  ...props
}: {
  as?: ElementType
  className?: string
  children?: ReactNode
  'aria-hidden'?: boolean | 'true' | 'false'
}) {
  return (
    <Component
      role="heading"
      className={['dropdown-header', className ?? ''].filter(Boolean).join(' ')}
      {...props}
    >
      {children}
    </Component>
  )
}

/** A list item wrapper, so items sit inside the ul as the original does. */
export function DropdownListItem({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <li role="none" className={className}>
      {children}
    </li>
  )
}

/**
 * The original's OLDropdownMenuItem: an item wrapped in its own <li>.
 */
export const OLDropdownMenuItem = forwardRef<HTMLElement, DropdownItemProps>(
  function OLDropdownMenuItem(props, ref) {
    return (
      <DropdownListItem>
        <DropdownItem {...props} ref={ref} />
      </DropdownListItem>
    )
  }
)

export const DropdownToggleCustom = forwardRef<
  HTMLElement,
  DropdownToggleProps
>(function DropdownToggleCustom({ children, className, ...props }, ref) {
  return (
    <DropdownToggle ref={ref} className={['custom-toggle', className ?? ''].filter(Boolean).join(' ')} {...props}>
      {children}
      <MaterialIcon type="expand_more" />
    </DropdownToggle>
  )
})
