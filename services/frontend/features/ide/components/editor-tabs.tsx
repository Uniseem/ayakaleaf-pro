'use client'

/**
 * The row of open files above the editor.
 *
 * 36px on a light grey ground, with the open one raised out of it in white --
 * measured from the original. The tabs are what makes several files feel open
 * at once rather than one file replacing another, which is what the editor
 * looked like without them.
 *
 * A file is closed by its own button, and closing the one in front moves to
 * whichever was next along, because closing the last tab and being left with
 * nothing is worse than being left with something.
 */

import { useCallback } from 'react'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { EntryIcon } from '@/features/file-tree/entry-icon'

export function EditorTabs() {
  const editor = useEditor()
  const { entryById } = useProject()

  const close = useCallback(
    (id: string, event: React.MouseEvent) => {
      event.stopPropagation()
      editor.closeTab(id)
    },
    [editor]
  )

  if (editor.openTabs.length === 0) {
    return null
  }

  return (
    <div
      role="tablist"
      aria-label="Open files"
      className="flex h-9 shrink-0 items-stretch overflow-x-auto border-b border-[var(--border-divider)] bg-[var(--bg-light-secondary)]"
    >
      {editor.openTabs.map(id => {
        const entry = entryById(id)
        if (!entry) {
          return null
        }
        const active = editor.current?.id === id || editor.currentFile?.id === id
        return (
          <div
            key={id}
            role="tab"
            aria-selected={active}
            tabIndex={0}
            onClick={() => editor.open(entry)}
            onKeyDown={event => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                editor.open(entry)
              }
            }}
            title={entry.path}
            className={[
              'group flex min-w-0 cursor-pointer items-center gap-1.5 border-r border-[var(--border-divider)] px-3 text-[14px] leading-5',
              active
                ? 'bg-[var(--bg-light-primary)] text-[var(--content-primary)]'
                : 'text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]',
            ].join(' ')}
          >
            <EntryIcon kind={entry.kind} name={entry.name} />
            <span className="max-w-[160px] truncate">{entry.name}</span>
            <button
              type="button"
              aria-label={`Close ${entry.name}`}
              onClick={event => close(id, event)}
              className="flex h-4 w-4 shrink-0 items-center justify-center rounded-[2px] text-[var(--content-secondary)] opacity-0 hover:bg-[var(--hover-interaction)] group-hover:opacity-100 focus-visible:opacity-100"
            >
              <svg viewBox="0 0 16 16" className="h-3 w-3" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
                <path d="M4 4l8 8M12 4l-8 8" strokeLinecap="round" />
              </svg>
            </button>
          </div>
        )
      })}
    </div>
  )
}
