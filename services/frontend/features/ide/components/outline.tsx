'use client'

/**
 * The document's headings.
 *
 * Read from the text rather than from the compiled PDF, so it is right while
 * you are typing rather than right after you compile. That is the whole value
 * of it: it is a map of the file you are in now.
 */

import { useMemo } from 'react'
import { ScrollShadow } from '@heroui/react'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { analyse } from '@/features/source-editor/analyse'

export function Outline() {
  const { current } = useEditor()

  const sections = useMemo(
    () => (current ? analyse(current.content).sections : []),
    [current]
  )

  if (!current) {
    return <p className="p-3 text-xs text-default-400">No file open.</p>
  }

  if (sections.length === 0) {
    return (
      <p className="p-3 text-xs text-default-400">
        No sections in this file yet.
      </p>
    )
  }

  // Levels are the index into the section commands, so `part` is 0. Indenting
  // from the shallowest heading present keeps a file that starts at
  // \subsection from being drawn pushed far to the right for no reason.
  const shallowest = Math.min(...sections.map(section => section.level))

  return (
    <ScrollShadow className="h-full">
      <ul className="p-1">
        {sections.map(section => (
          <li key={`${section.line}-${section.from}`}>
            <button
              type="button"
              className="block w-full truncate rounded px-2 py-1 text-left text-xs hover:bg-default-100"
              style={{ paddingLeft: `${(section.level - shallowest) * 10 + 8}px` }}
              title={section.title}
              onClick={() => {
                // The editor listens for this rather than being called: the
                // outline does not hold a reference to the CodeMirror view,
                // and giving it one would tie the two together for one jump.
                window.dispatchEvent(
                  new CustomEvent('ide:goto-line', { detail: { line: section.line } })
                )
              }}
            >
              {section.title || <span className="text-default-400">(untitled)</span>}
            </button>
          </li>
        ))}
      </ul>
    </ScrollShadow>
  )
}
