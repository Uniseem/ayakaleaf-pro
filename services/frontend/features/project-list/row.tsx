'use client'

/**
 * One project, as a row of the table.
 *
 * A table and not a list of cards: the original is a table, with the columns
 * it sorts by as its headers, and the actions as icon buttons at the end of
 * the row rather than behind a menu.
 */

import Link from 'next/link'
import type { ProjectSummary, Tag } from '@/lib/projects'

/** A time somebody can read at a glance rather than a date they must parse. */
export function whenReadable(iso: string): string {
  const at = Date.parse(iso)
  if (Number.isNaN(at)) {
    return ''
  }
  const seconds = Math.round((Date.now() - at) / 1000)
  if (seconds < 60) {
    return 'a few seconds ago'
  }
  if (seconds < 3600) {
    const minutes = Math.round(seconds / 60)
    return `${minutes} minute${minutes === 1 ? '' : 's'} ago`
  }
  if (seconds < 86400) {
    const hours = Math.round(seconds / 3600)
    return `${hours} hour${hours === 1 ? '' : 's'} ago`
  }
  if (seconds < 86400 * 7) {
    const days = Math.round(seconds / 86400)
    return `${days} day${days === 1 ? '' : 's'} ago`
  }
  return new Date(at).toLocaleDateString()
}

export function ProjectRow({
  project,
  tags,
  isSelected,
  onSelect,
  onRename,
  onCopy,
  onDelete,
  onArchive,
  onTrash,
}: {
  project: ProjectSummary
  tags: Tag[]
  isSelected: boolean
  onSelect: (on: boolean) => void
  onRename: () => void
  onCopy: () => void
  onDelete: () => void
  onArchive: (on: boolean) => void
  onTrash: (on: boolean) => void
}) {
  const isOwner = project.access === 'owner'

  return (
    <tr className="border-b border-[var(--border-divider)] last:border-b-0 hover:bg-[var(--bg-light-secondary)]">
      <td className="w-10 px-3 py-3 align-top">
        <input
          type="checkbox"
          checked={isSelected}
          onChange={event => onSelect(event.target.checked)}
          aria-label={`Select ${project.name}`}
          className="h-4 w-4 accent-[var(--bg-accent-01)]"
        />
      </td>

      <td className="px-2 py-3 align-top">
        <Link
          href={`/projects/${project.id}`}
          className="text-[16px] leading-6 text-[var(--link-web)] hover:underline"
        >
          {project.name}
        </Link>
        {tags.length > 0 ? (
          <div className="mt-1 flex flex-wrap gap-1.5">
            {tags.map(tag => (
              <span
                key={tag.id}
                className="inline-flex items-center gap-1 rounded-full px-2 py-px text-[12px] leading-4"
                style={{
                  backgroundColor: (tag.color || '#495365') + '1f',
                  color: tag.color || '#495365',
                }}
              >
                {tag.name}
              </span>
            ))}
          </div>
        ) : null}
      </td>

      <td className="hidden px-2 py-3 align-top text-[16px] leading-6 text-[var(--content-primary)] sm:table-cell">
        {isOwner ? 'You' : (project.lastUpdatedBy ?? '—')}
      </td>

      <td className="hidden px-2 py-3 align-top text-[16px] leading-6 text-[var(--content-primary)] md:table-cell">
        {whenReadable(project.lastUpdated)}
        {project.lastUpdatedBy ? ` by ${project.lastUpdatedBy}` : ''}
      </td>

      <td className="px-3 py-3 align-top">
        <div className="flex items-center justify-end gap-1">
          <IconButton label="Make a copy" onClick={onCopy}>
            <CopyIcon />
          </IconButton>
          {isOwner ? (
            <IconButton label="Rename" onClick={onRename}>
              <RenameIcon />
            </IconButton>
          ) : null}
          <IconButton
            label={project.archived ? 'Unarchive' : 'Archive'}
            onClick={() => onArchive(!project.archived)}
          >
            <ArchiveIcon />
          </IconButton>
          <IconButton
            label={project.trashed ? 'Restore' : 'Move to trash'}
            onClick={() => onTrash(!project.trashed)}
          >
            <TrashIcon />
          </IconButton>
          {isOwner && project.trashed ? (
            <IconButton label="Delete forever" onClick={onDelete} danger>
              <DeleteIcon />
            </IconButton>
          ) : null}
        </div>
      </td>
    </tr>
  )
}

function IconButton({
  label,
  onClick,
  danger,
  children,
}: {
  label: string
  onClick: () => void
  danger?: boolean
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      onClick={onClick}
      className={`flex h-7 w-7 items-center justify-center rounded-[4px] hover:bg-[var(--hover-interaction)] ${
        danger ? 'text-[var(--content-danger)]' : 'text-[var(--content-secondary)]'
      }`}
    >
      {children}
    </button>
  )
}

const stroke = {
  className: 'h-4 w-4',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.5,
  viewBox: '0 0 16 16',
  'aria-hidden': true,
} as const

function CopyIcon() {
  return (
    <svg {...stroke}>
      <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
      <path d="M10.5 5.5v-1a1.5 1.5 0 0 0-1.5-1.5H4a1.5 1.5 0 0 0-1.5 1.5V9A1.5 1.5 0 0 0 4 10.5h1" />
    </svg>
  )
}

function RenameIcon() {
  return (
    <svg {...stroke}>
      <path d="M11.2 2.8a1.6 1.6 0 0 1 2.3 2.3L6 12.6l-3 .7.7-3z" strokeLinejoin="round" />
    </svg>
  )
}

function ArchiveIcon() {
  return (
    <svg {...stroke}>
      <rect x="2" y="3" width="12" height="3" rx="0.8" />
      <path d="M3 6.5v6A1.5 1.5 0 0 0 4.5 14h7a1.5 1.5 0 0 0 1.5-1.5v-6" />
      <path d="M6.5 9h3" strokeLinecap="round" />
    </svg>
  )
}

function TrashIcon() {
  return (
    <svg {...stroke}>
      <path d="M3 4.5h10" strokeLinecap="round" />
      <path d="M6 4.5V3.2A.7.7 0 0 1 6.7 2.5h2.6a.7.7 0 0 1 .7.7v1.3" />
      <path d="M4.3 4.5 5 13a1 1 0 0 0 1 .9h4a1 1 0 0 0 1-.9l.7-8.5" />
    </svg>
  )
}

function DeleteIcon() {
  return (
    <svg {...stroke}>
      <path d="M4 4l8 8M12 4l-8 8" strokeLinecap="round" />
    </svg>
  )
}
