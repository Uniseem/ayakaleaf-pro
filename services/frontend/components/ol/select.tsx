'use client'

/**
 * The select, from shared/components/select.tsx: a read-only form control
 * that opens a dropdown list under itself, with a subtitle, a leading icon
 * and a disabled state per item, and a tick on the chosen one.
 *
 * The original builds it on downshift; this is the same interaction --
 * Enter or Space or a click opens it, arrows move, Enter chooses, Escape
 * closes -- without the library.
 */

import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { FormControl } from './form-control'
import MaterialIcon from './material-icon'
import { DropdownItem } from './dropdown'
import { Spinner } from './spinner'
import { useTranslation } from '@/lib/i18n'

export type SelectProps<T> = {
  items: T[]
  itemToString: (item: T | null | undefined) => string
  label?: ReactNode
  name?: string
  defaultText?: string
  defaultItem?: T | null
  itemToSubtitle?: (item: T | null | undefined) => string
  itemToKey: (item: T) => string
  itemToLeadingIcon?: (item: T | null | undefined) => ReactNode
  onSelectedItemChanged?: (item: T | null | undefined) => void
  selected?: T | null
  disabled?: boolean
  itemToDisabled?: (item: T | null | undefined) => boolean
  optionalLabel?: boolean
  loading?: boolean
  selectedIcon?: boolean
  dataTestId?: string
  size?: 'sm' | 'lg'
  id?: string
}

export function Select<T>({
  items,
  itemToString = item => (item === null || item === undefined ? '' : String(item)),
  label,
  name,
  defaultText = 'Items',
  defaultItem,
  itemToSubtitle,
  itemToKey,
  itemToLeadingIcon,
  onSelectedItemChanged,
  selected,
  disabled = false,
  itemToDisabled,
  optionalLabel = false,
  loading = false,
  selectedIcon = false,
  dataTestId,
  size,
  id,
}: SelectProps<T>) {
  const { t } = useTranslation()
  const generated = useId()
  const toggleId = id ?? `select-${generated}`
  const [selectedItem, setSelectedItem] = useState<T | null | undefined>(selected ?? defaultItem)
  const [isOpen, setIsOpen] = useState(false)
  const [highlighted, setHighlighted] = useState(-1)
  const rootRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    setSelectedItem(selected)
  }, [selected])

  useEffect(() => {
    if (!isOpen) {
      return
    }
    const handle = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handle, true)
    return () => document.removeEventListener('mousedown', handle, true)
  }, [isOpen])

  const choose = useCallback(
    (item: T) => {
      if (itemToDisabled?.(item)) {
        return
      }
      setSelectedItem(item)
      onSelectedItemChanged?.(item)
      setIsOpen(false)
    },
    [itemToDisabled, onSelectedItemChanged]
  )

  const open = () => {
    setHighlighted(Math.max(0, items.findIndex(item => item === selectedItem)))
    setIsOpen(true)
  }

  const onKeyDown = (event: React.KeyboardEvent) => {
    if ((event.key === 'Enter' || event.key === ' ') && !isOpen) {
      event.preventDefault()
      open()
    } else if (event.key === 'Escape' && isOpen) {
      event.stopPropagation()
      setIsOpen(false)
    } else if (event.key === 'ArrowDown') {
      event.preventDefault()
      if (!isOpen) {
        open()
      } else {
        setHighlighted(index => Math.min(index + 1, items.length - 1))
      }
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      if (!isOpen) {
        open()
      } else {
        setHighlighted(index => Math.max(index - 1, 0))
      }
    } else if (event.key === 'Enter' && isOpen) {
      event.preventDefault()
      const item = items[highlighted]
      if (item !== undefined) {
        choose(item)
      }
    } else if (event.key === 'Tab' && isOpen) {
      setIsOpen(false)
    }
  }

  let value: string
  if (selectedItem || defaultItem) {
    value = itemToString(selectedItem || defaultItem)
  } else {
    value = defaultText
  }

  return (
    <div className="select-wrapper" ref={rootRef}>
      {label ? (
        <label className="form-label" htmlFor={toggleId}>
          {label} {optionalLabel ? <span className="fw-normal">({t('optional')})</span> : null}{' '}
          {loading ? <Spinner size="sm" /> : null}
        </label>
      ) : null}
      {name ? <input type="hidden" name={name} value={selectedItem || defaultItem ? itemToString(selectedItem || defaultItem) : ''} /> : null}
      <FormControl
        data-testid={dataTestId}
        id={toggleId}
        type="button"
        role="combobox"
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        disabled={disabled}
        onKeyDown={onKeyDown}
        onClick={() => (isOpen ? setIsOpen(false) : open())}
        className="select-trigger"
        value={value}
        readOnly
        append={<MaterialIcon type={isOpen ? 'keyboard_arrow_up' : 'keyboard_arrow_down'} className="align-text-bottom" />}
        size={size}
      />
      <ul className={['dropdown-menu', 'w-100', isOpen ? 'show' : ''].filter(Boolean).join(' ')} role="listbox">
        {isOpen
          ? items.map((item, index) => {
              const itemDisabled = itemToDisabled?.(item) || false
              return (
                <li role="none" key={itemToKey(item)}>
                  <DropdownItem
                    as="button"
                    type="button"
                    role="option"
                    className={highlighted === index ? 'select-highlighted' : ''}
                    active={selectedItem === item}
                    trailingIcon={selectedIcon && selectedItem === item ? 'check' : undefined}
                    leadingIcon={itemToLeadingIcon ? itemToLeadingIcon(item) : undefined}
                    description={itemToSubtitle ? itemToSubtitle(item) || undefined : undefined}
                    disabled={itemDisabled}
                    onMouseEnter={() => setHighlighted(index)}
                    onClick={event => {
                      event.preventDefault()
                      choose(item)
                    }}
                  >
                    {itemToString(item)}
                  </DropdownItem>
                </li>
              )
            })
          : null}
      </ul>
    </div>
  )
}

export default Select
