'use client'

/**
 * A menu bar, from shared/components/menu-bar.
 *
 * Several dropdowns in a row that behave as one: once any is open, moving
 * the pointer onto another opens that one instead, and a nested submenu
 * opens on hover from inside a menu. One selection is held for the whole
 * bar so that exactly one menu is open at a time.
 */

import {
  createContext,
  forwardRef,
  useCallback,
  useContext,
  useEffect,
  useState,
  type Dispatch,
  type MouseEventHandler,
  type ReactNode,
  type SetStateAction,
} from 'react'
import { Dropdown, DropdownItem, DropdownListItem, DropdownMenu, DropdownToggle } from './dropdown'
import MaterialIcon from './material-icon'

type NestableDropdownContextType = {
  selected: string | null
  setSelected: Dispatch<SetStateAction<string | null>>
  menuId: string
}

const NestableDropdownContext = createContext<NestableDropdownContextType | undefined>(undefined)

export function NestableDropdownContextProvider({ id, children }: { id: string; children: ReactNode }) {
  const [selected, setSelected] = useState<string | null>(null)

  useEffect(() => {
    return () => setSelected(null)
  }, [])

  return (
    <NestableDropdownContext.Provider value={{ selected, setSelected, menuId: id }}>
      {children}
    </NestableDropdownContext.Provider>
  )
}

export function useNestableDropdown() {
  const context = useContext(NestableDropdownContext)
  if (!context) {
    throw new Error('useNestableDropdown must be used within a NestableDropdownContextProvider')
  }
  return context
}

export function MenuBar({
  children,
  id,
  className,
  ...props
}: {
  children: ReactNode
  id: string
  className?: string
} & React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div {...props} className={className} role="menubar">
      <NestableDropdownContextProvider id={id}>{children}</NestableDropdownContextProvider>
    </div>
  )
}

export function MenuBarDropdown({
  title,
  children,
  id,
  className,
  align = 'start',
}: {
  title: string
  children: ReactNode
  id: string
  className?: string
  align?: 'start' | 'end'
}) {
  const { menuId, selected, setSelected } = useNestableDropdown()

  const onToggle = useCallback(
    (show: boolean) => {
      setSelected(show ? id : null)
    },
    [id, setSelected]
  )

  const onHover = useCallback(() => {
    setSelected(previous => (previous === null ? null : id))
  }, [id, setSelected])

  const active = selected === id
  return (
    <Dropdown show={active} align={align} onToggle={onToggle} autoClose>
      <DropdownToggle
        id={`${menuId}-${id}`}
        variant="secondary"
        className={[className ?? '', 'menu-bar-toggle'].filter(Boolean).join(' ')}
        onMouseEnter={onHover}
      >
        {title}
      </DropdownToggle>
      {active ? (
        <DropdownMenu renderOnMount id={`${menuId}-${id}-menu`}>
          <NestableDropdownContextProvider id={`${menuId}-${id}`}>{children}</NestableDropdownContextProvider>
        </DropdownMenu>
      ) : null}
    </Dropdown>
  )
}

const NestedDropdownToggle = forwardRef<
  HTMLAnchorElement,
  { children?: ReactNode; className?: string; onMouseEnter?: MouseEventHandler; id?: string; onClick?: MouseEventHandler }
>(function NestedDropdownToggle({ children, className, onMouseEnter, id }, ref) {
  return (
    <a
      id={id}
      href="#"
      ref={ref}
      onMouseEnter={onMouseEnter}
      onClick={event => {
        event.preventDefault()
        onMouseEnter?.(event)
      }}
      className={[className ?? '', 'nested-dropdown-toggle', 'dropdown-item'].filter(Boolean).join(' ')}
      role="menuitem"
      aria-haspopup
    >
      {children}
      <MaterialIcon type="chevron_right" />
    </a>
  )
})

export function NestedMenuBarDropdown({ children, id, title }: { children: ReactNode; id: string; title: string }) {
  const { menuId, selected, setSelected } = useNestableDropdown()
  const select = useCallback(() => setSelected(id), [id, setSelected])
  const onToggle = useCallback(
    (show: boolean) => {
      if (show) {
        setSelected(id)
      }
    },
    [setSelected, id]
  )
  const active = selected === id
  return (
    <Dropdown align="start" drop="end" show={active} autoClose onToggle={onToggle} as="li" role="none">
      <DropdownToggle
        id={`${menuId}-${id}`}
        onMouseEnter={select}
        className={active ? 'nested-dropdown-toggle-shown' : ''}
        as={NestedDropdownToggle}
        bsPrefix="nested-dropdown-toggle"
      >
        {title}
      </DropdownToggle>
      {active ? (
        <DropdownMenu renderOnMount id={`${menuId}-${id}-menu`}>
          <NestableDropdownContextProvider id={`${menuId}-${id}`}>{children}</NestableDropdownContextProvider>
        </DropdownMenu>
      ) : null}
    </Dropdown>
  )
}

export function MenuBarOption({
  title,
  onClick,
  href,
  disabled,
  leadingIcon,
  trailingIcon,
  target,
  rel,
  eventKey,
}: {
  title: string
  onClick?: MouseEventHandler
  disabled?: boolean
  leadingIcon?: ReactNode
  trailingIcon?: ReactNode
  href?: string
  target?: string
  rel?: string
  eventKey?: string
}) {
  const { setSelected } = useNestableDropdown()
  return (
    <DropdownListItem>
      <DropdownItem
        onMouseEnter={() => setSelected(null)}
        onClick={onClick}
        disabled={disabled}
        leadingIcon={leadingIcon}
        trailingIcon={trailingIcon}
        href={href}
        rel={rel}
        target={target}
        eventKey={eventKey}
      >
        {title}
      </DropdownItem>
    </DropdownListItem>
  )
}
