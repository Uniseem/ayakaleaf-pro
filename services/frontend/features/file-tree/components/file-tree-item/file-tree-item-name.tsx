'use client'

/**
 * A row's name, which becomes an input while it is being renamed.
 *
 * From file-tree/components/file-tree-item/file-tree-item-name. The input
 * selects the stem and not the extension when it opens, because renaming
 * report.tex to summary.tex should not mean retyping ".tex".
 */

import { useEffect, useRef, useState } from 'react'
import { useFileTreeActionable } from '../../contexts/file-tree-actionable'

export function FileTreeItemName({
  name,
  isSelected,
  setIsDraggable,
}: {
  name: string
  isSelected: boolean
  setIsDraggable: (isDraggable: boolean) => void
}) {
  const { isRenaming, finishRenaming, error, cancel } = useFileTreeActionable()

  const isRenamingEntity = isRenaming && isSelected && !error

  useEffect(() => {
    // A drag that starts inside a text input is a text selection.
    setIsDraggable(!isRenamingEntity)
  }, [setIsDraggable, isRenamingEntity])

  if (isRenamingEntity) {
    return <InputName initialValue={name} finishRenaming={finishRenaming} cancel={cancel} />
  }
  return <DisplayName name={name} />
}

function DisplayName({ name }: { name: string }) {
  return (
    <div className="item-name">
      <span>{name}</span>
    </div>
  )
}

function InputName({
  initialValue,
  finishRenaming,
  cancel,
}: {
  initialValue: string
  finishRenaming: (value: string) => void
  cancel: () => void
}) {
  const [value, setValue] = useState(initialValue)
  const inputRef = useRef<HTMLInputElement>(null)

  // The menu that started the rename takes the focus back to its own button
  // when it closes, as menus are supposed to. Claiming the focus on the next
  // frame puts the cursor where the person is about to type.
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      inputRef.current?.focus()
    })
    return () => window.cancelAnimationFrame(frame)
  }, [])

  function handleFocus(event: React.FocusEvent<HTMLInputElement>) {
    const lastDotIndex = event.target.value.lastIndexOf('.')
    event.target.setSelectionRange(0, lastDotIndex === -1 ? event.target.value.length : lastDotIndex)
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter') {
      finishRenaming(value)
    }
    if (event.key === 'Escape') {
      cancel()
    }
  }

  return (
    <span className="rename-input">
      <input
        type="text"
        value={value}
        onKeyDown={handleKeyDown}
        onChange={event => setValue(event.target.value)}
        onBlur={() => finishRenaming(value)}
        onFocus={handleFocus}
        onClick={event => event.stopPropagation()}
        ref={inputRef}
      />
    </span>
  )
}

export default FileTreeItemName
