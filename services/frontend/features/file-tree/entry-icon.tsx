'use client'

/**
 * The icon beside a name in the tree.
 *
 * Kind first, then extension: a folder is a folder, and beyond that the useful
 * distinction is what opening it will do -- a document opens in the editor, an
 * image opens in the viewer, and a bibliography is neither.
 */

import { extensionOf } from './tree'

type Kind = 'folder' | 'doc' | 'file'

const IMAGE = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'eps', 'pdf'])
const BIB = new Set(['bib', 'bbl', 'bst'])

export function EntryIcon({
  kind,
  name,
  collapsed,
}: {
  kind: Kind
  name: string
  collapsed?: boolean
}) {
  const shared = {
    viewBox: '0 0 16 16',
    className: 'h-3.5 w-3.5 shrink-0 text-[var(--content-secondary-themed)]',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.4,
    'aria-hidden': true,
  } as const

  if (kind === 'folder') {
    return (
      <svg {...shared}>
        <path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h2.6l1.2 1.5h5.2A1.5 1.5 0 0 1 14 6v5.5A1.5 1.5 0 0 1 12.5 13h-9A1.5 1.5 0 0 1 2 11.5z" />
        {collapsed ? null : <path d="M2 7h12" strokeLinecap="round" />}
      </svg>
    )
  }

  const extension = extensionOf(name)

  if (IMAGE.has(extension)) {
    return (
      <svg {...shared}>
        <rect x="2" y="3" width="12" height="10" rx="1.5" />
        <circle cx="5.75" cy="6.5" r="1" />
        <path d="M2.5 11.5 6 8.5l2.5 2 2-1.5 3 2.5" strokeLinejoin="round" />
      </svg>
    )
  }

  if (BIB.has(extension)) {
    return (
      <svg {...shared}>
        <path d="M3 3.5h4.2a1.8 1.8 0 0 1 1.8 1.8V13a1.4 1.4 0 0 0-1.4-1.4H3z" />
        <path d="M13 3.5H8.8A1.8 1.8 0 0 0 7 5.3V13a1.4 1.4 0 0 1 1.4-1.4H13z" />
      </svg>
    )
  }

  // A document. The line down the middle is the fold of a page, which is what
  // separates it from an image at this size.
  return (
    <svg {...shared}>
      <path d="M4 2.5h5L12.5 6v7.5A1 1 0 0 1 11.5 14h-7a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1z" />
      <path d="M9 2.5V6h3.5" strokeLinejoin="round" />
    </svg>
  )
}
