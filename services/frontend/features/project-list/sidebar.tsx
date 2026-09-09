'use client'

/**
 * The lists down the side of the projects page.
 *
 * Two groups: where a project is (everything, yours, shared with you, archived,
 * binned) and what it has been labelled. They are separate because they answer
 * different questions and combining them into one list of "views" makes both
 * harder to scan.
 */

import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from '@heroui/react'
import { useState } from 'react'
import { createTag, deleteTag, renameTag, type Filter, type Tag } from '@/lib/projects'
import { messageFor } from '@/lib/api'

const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'All projects' },
  { key: 'owned', label: 'Your projects' },
  { key: 'shared', label: 'Shared with you' },
  { key: 'archived', label: 'Archived' },
  { key: 'trashed', label: 'Bin' },
]

/** The colours a tag may be, kept few so a list of them stays readable. */
const COLOURS = [
  '#0b6bcb', '#0f766e', '#7c3aed', '#b4532a', '#b91c1c', '#4d7c0f', '#7b8794',
]

export function ProjectSidebar({
  filter,
  onFilter,
  counts,
  tags,
  tagId,
  onTag,
  onChanged,
}: {
  filter: Filter
  onFilter: (filter: Filter) => void
  counts: Record<Filter, number>
  tags: Tag[]
  tagId: string | null
  onTag: (id: string | null) => void
  onChanged: () => Promise<void> | void
}) {
  const [editing, setEditing] = useState<Tag | 'new' | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function run(work: () => Promise<unknown>) {
    setBusy(true)
    setError(null)
    try {
      await work()
      await onChanged()
      setEditing(null)
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setBusy(false)
    }
  }

  return (
    <aside className="hidden w-56 shrink-0 border-r border-divider p-3 sm:block">
      <nav aria-label="Filter projects">
        <ul className="flex flex-col gap-0.5">
          {FILTERS.map(each => (
            <li key={each.key}>
              <button
                type="button"
                onClick={() => onFilter(each.key)}
                className={[
                  'flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm',
                  filter === each.key && !tagId
                    ? 'bg-default-200 font-medium'
                    : 'hover:bg-default-100',
                ].join(' ')}
              >
                <span>{each.label}</span>
                <span className="text-xs text-default-400">{counts[each.key]}</span>
              </button>
            </li>
          ))}
        </ul>
      </nav>

      <div className="mt-5">
        <div className="mb-1 flex items-center justify-between px-2">
          <span className="text-xs font-semibold uppercase tracking-wide text-default-500">
            Tags
          </span>
          <Button
            size="sm"
            variant="light"
            isIconOnly
            className="h-6 w-6 min-w-6"
            aria-label="New tag"
            onPress={() => setEditing('new')}
          >
            +
          </Button>
        </div>

        {tags.length === 0 ? (
          <p className="px-2 py-1 text-xs text-default-400">
            None yet. A tag is a way to group projects that are not otherwise
            related.
          </p>
        ) : (
          <ul className="flex flex-col gap-0.5">
            {tags.map(tag => (
              <li key={tag.id} className="group flex items-center">
                <button
                  type="button"
                  onClick={() => onTag(tagId === tag.id ? null : tag.id)}
                  className={[
                    'flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm',
                    tagId === tag.id ? 'bg-default-200 font-medium' : 'hover:bg-default-100',
                  ].join(' ')}
                >
                  <span
                    aria-hidden
                    className="h-2.5 w-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: tag.color || COLOURS[0] }}
                  />
                  <span className="truncate">{tag.name}</span>
                  <span className="ml-auto text-xs text-default-400">
                    {tag.projectIds.length}
                  </span>
                </button>
                <Dropdown placement="bottom-end">
                  <DropdownTrigger>
                    <Button
                      isIconOnly
                      size="sm"
                      variant="light"
                      className="h-6 w-6 min-w-6 opacity-0 group-hover:opacity-100 data-[focus-visible=true]:opacity-100"
                      aria-label={`Actions for ${tag.name}`}
                    >
                      ⋯
                    </Button>
                  </DropdownTrigger>
                  <DropdownMenu
                    aria-label={`Actions for ${tag.name}`}
                    onAction={key => {
                      if (key === 'rename') {
                        setEditing(tag)
                      } else if (key === 'delete') {
                        void run(() => deleteTag(tag.id))
                      }
                    }}
                  >
                    <DropdownItem key="rename">Rename</DropdownItem>
                    <DropdownItem key="delete" color="danger" className="text-danger">
                      Delete tag
                    </DropdownItem>
                  </DropdownMenu>
                </Dropdown>
              </li>
            ))}
          </ul>
        )}
      </div>

      {editing ? (
        <TagDialog
          key={editing === 'new' ? 'new' : editing.id}
          tag={editing === 'new' ? null : editing}
          busy={busy}
          error={error}
          onCancel={() => {
            setEditing(null)
            setError(null)
          }}
          onConfirm={(name, colour) =>
            void run(() =>
              editing === 'new'
                ? createTag(name, colour)
                : renameTag(editing.id, name, colour)
            )
          }
        />
      ) : null}
    </aside>
  )
}

function TagDialog({
  tag,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  tag: Tag | null
  busy: boolean
  error: string | null
  onCancel: () => void
  onConfirm: (name: string, colour: string) => void
}) {
  const [name, setName] = useState(tag?.name ?? '')
  const [colour, setColour] = useState(tag?.color ?? COLOURS[0] ?? '#0b6bcb')

  return (
    <Modal isOpen onClose={onCancel} size="sm">
      <ModalContent>
        <form
          onSubmit={event => {
            event.preventDefault()
            if (name.trim() && !busy) {
              onConfirm(name.trim(), colour)
            }
          }}
        >
          <ModalHeader>{tag ? 'Rename tag' : 'New tag'}</ModalHeader>
          <ModalBody>
            <Input autoFocus label="Name" value={name} onValueChange={setName} />
            <div>
              <span className="mb-1 block text-xs text-default-500">Colour</span>
              <div className="flex gap-1.5">
                {COLOURS.map(each => (
                  <button
                    key={each}
                    type="button"
                    aria-label={`Colour ${each}`}
                    aria-pressed={colour === each}
                    onClick={() => setColour(each)}
                    className={[
                      'h-6 w-6 rounded-full border-2',
                      colour === each ? 'border-foreground' : 'border-transparent',
                    ].join(' ')}
                    style={{ backgroundColor: each }}
                  />
                ))}
              </div>
            </div>
            {error ? <p className="text-sm text-danger">{error}</p> : null}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onCancel} isDisabled={busy}>
              Cancel
            </Button>
            <Button color="primary" type="submit" isLoading={busy} isDisabled={!name.trim()}>
              {tag ? 'Rename' : 'Create'}
            </Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  )
}
