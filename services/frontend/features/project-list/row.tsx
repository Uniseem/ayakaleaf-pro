'use client'

/**
 * One project in the list.
 *
 * The whole row is not a link: it holds a checkbox and a menu, and a link
 * around those swallows their clicks and drags the row when somebody tries to
 * select text. The name is the link, and it is the only thing that is.
 */

import {
  Button,
  Checkbox,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
} from '@heroui/react'
import Link from 'next/link'
import type { ProjectSummary, Tag } from '@/lib/projects'

/** A time somebody can read at a glance rather than a date they must parse. */
function whenReadable(iso: string): string {
  const at = Date.parse(iso)
  if (Number.isNaN(at)) {
    return ''
  }
  const seconds = Math.round((Date.now() - at) / 1000)
  if (seconds < 60) {
    return 'just now'
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
    <li className="group flex items-center gap-3 px-3 py-2.5 hover:bg-default-50">
      <Checkbox
        size="sm"
        isSelected={isSelected}
        onValueChange={onSelect}
        aria-label={`Select ${project.name}`}
      />

      <div className="min-w-0 flex-1">
        <Link
          href={`/projects/${project.id}`}
          className="truncate text-sm font-medium hover:underline"
        >
          {project.name}
        </Link>
        <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
          {!isOwner ? (
            <span className="text-[11px] text-default-400">
              shared with you
              {project.access === 'readOnly' ? ' · read only' : ''}
            </span>
          ) : null}
          {tags.map(tag => (
            <span
              key={tag.id}
              className="inline-flex items-center gap-1 rounded px-1.5 py-px text-[10px]"
              style={{
                backgroundColor: (tag.color || '#7b8794') + '22',
                color: tag.color || '#7b8794',
              }}
            >
              {tag.name}
            </span>
          ))}
        </div>
      </div>

      <time
        className="hidden w-40 text-xs text-default-500 sm:block"
        dateTime={project.lastUpdated}
        title={new Date(project.lastUpdated).toLocaleString()}
      >
        {whenReadable(project.lastUpdated)}
        {project.lastUpdatedBy ? (
          <span className="block text-[11px] text-default-400">
            by {project.lastUpdatedBy}
          </span>
        ) : null}
      </time>

      <Dropdown placement="bottom-end">
        <DropdownTrigger>
          <Button
            isIconOnly
            size="sm"
            variant="light"
            className="h-7 w-7 min-w-7 opacity-0 group-hover:opacity-100 data-[focus-visible=true]:opacity-100"
            aria-label={`Actions for ${project.name}`}
          >
            ⋯
          </Button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label={`Actions for ${project.name}`}
          onAction={key => {
            switch (key) {
              case 'rename':
                onRename()
                break
              case 'copy':
                onCopy()
                break
              case 'archive':
                onArchive(!project.archived)
                break
              case 'trash':
                onTrash(!project.trashed)
                break
              case 'delete':
                onDelete()
                break
            }
          }}
        >
          {isOwner ? <DropdownItem key="rename">Rename</DropdownItem> : null}
          <DropdownItem key="copy">Make a copy</DropdownItem>
          <DropdownItem key="archive">
            {project.archived ? 'Unarchive' : 'Archive'}
          </DropdownItem>
          <DropdownItem key="trash">
            {project.trashed ? 'Restore from bin' : 'Move to bin'}
          </DropdownItem>
          {isOwner && project.trashed ? (
            <DropdownItem key="delete" color="danger" className="text-danger">
              Delete forever
            </DropdownItem>
          ) : null}
        </DropdownMenu>
      </Dropdown>
    </li>
  )
}
