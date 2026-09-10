import { useCallback, useEffect, useState, type RefObject } from 'react'
import { useTranslation } from '@/lib/i18n'
import { OLFormControl } from '@/components/ol/form-control'
import { useDebounced } from '@/lib/hooks'

/**
 * The search field.
 *
 * Debounced because every keystroke re-ranks every symbol; the field itself
 * stays immediate, so typing never feels behind.
 */
export function SymbolPaletteSearch({
  setInput,
  inputRef,
}: {
  setInput: (input: string) => void
  inputRef: RefObject<HTMLInputElement | null>
}) {
  const [localInput, setLocalInput] = useState('')

  const debouncedLocalInput = useDebounced(localInput, 250)

  useEffect(() => {
    setInput(debouncedLocalInput)
  }, [debouncedLocalInput, setInput])

  const { t } = useTranslation()

  const inputRefCallback = useCallback(
    (element: HTMLInputElement | null) => {
      inputRef.current = element
    },
    [inputRef]
  )

  return (
    <OLFormControl
      className="symbol-palette-search"
      type="search"
      ref={inputRefCallback}
      id="symbol-palette-input"
      aria-label="Search"
      value={localInput}
      placeholder={t('search') + '…'}
      style={{ maxWidth: '130px' }}
      onChange={event => {
        setLocalInput(event.target.value)
      }}
    />
  )
}
