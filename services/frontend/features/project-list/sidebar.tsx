'use client'

/**
 * The lists down the side of the projects page.
 *
 * 200px wide, transparent, with the new-project button at the top and the
 * filters below it as 40px rows that turn green when they are the one you are
 * on. Tags come after, under a small-caps heading. All measured from the
 * original rather than chosen.
 */

import { Dropdown, DropdownItem, DropdownMenu, DropdownToggle } from '@/components/ol/dropdown'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { useState } from 'react'
import { createTag, deleteTag, renameTag, type Filter, type Tag } from '@/lib/projects'
import { messageFor } from '@/lib/api'
import { Button, TextField } from '@/components/ui'

const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'All projects' },
  { key: 'owned', label: 'Your projects' },
  { key: 'shared', label: 'Shared with you' },
  { key: 'archived', label: 'Archived projects' },
  { key: 'trashed', label: 'Trashed projects' },
]

/** What the new-project menu can be asked for. */
export type NewProject = 'blank' | 'example' | 'upload'

function Caret() {
  return (
    <svg viewBox="0 0 16 16" className="h-3 w-3" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <path d="m4 6 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

/** The colours a tag may be, kept few so a list of them stays readable. */
const COLOURS = ['#098842', '#366cbf', '#b83a33', '#8f5514', '#495365', '#1e6b41', '#28518f']

export function ProjectSidebar({
  filter,
  onFilter,
  tags,
  tagId,
  onTag,
  onChanged,
  onNewProject,
}: {
  filter: Filter
  onFilter: (filter: Filter) => void
  tags: Tag[]
  tagId: string | null
  onTag: (id: string | null) => void
  onChanged: () => Promise<void> | void
  onNewProject: (kind: NewProject) => void
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

  const row =
    'flex w-full items-center gap-2 rounded-[8px] px-3 py-2 text-left text-[16px] leading-6'

  return (
    <aside className="hidden w-[200px] shrink-0 flex-col gap-4 px-3 py-4 md:flex">
      {/* A menu rather than a button, because "new project" is three
          different questions: start from nothing, start from something that
          already works, or bring one you have. The original asks the same
          three, and the first is what the button does if you just click it. */}
      <Dropdown className="w-full">
        <DropdownToggle
          bsPrefix="new-project-toggle"
          aria-label="New project"
          className="flex h-9 w-full items-center justify-center gap-1.5 rounded-full bg-[var(--bg-accent-01)] px-4 text-[16px] font-semibold leading-6 text-white hover:bg-[var(--bg-accent-02)]"
        >
          New project
          <Caret />
        </DropdownToggle>
        <DropdownMenu>
          <li role="none">
            <DropdownItem as="button" onClick={() => onNewProject('blank')}>
              Blank project
            </DropdownItem>
          </li>
          <li role="none">
            <DropdownItem as="button" onClick={() => onNewProject('example')}>
              Example project
            </DropdownItem>
          </li>
          <li role="none">
            <DropdownItem as="button" onClick={() => onNewProject('upload')}>
              Upload project
            </DropdownItem>
          </li>
        </DropdownMenu>
      </Dropdown>

      <nav aria-label="Filter projects">
        <ul className="flex flex-col">
          {FILTERS.map(each => {
            const active = filter === each.key && !tagId
            return (
              <li key={each.key}>
                <button
                  type="button"
                  onClick={() => onFilter(each.key)}
                  className={`${row} ${
                    active
                      ? 'bg-[var(--bg-accent-03)] font-bold text-[var(--link-web)]'
                      : 'text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]'
                  }`}
                >
                  {each.label}
                </button>
              </li>
            )
          })}
        </ul>
      </nav>

      <div>
        <h2 className="px-3 pb-1 text-[12px] font-semibold uppercase leading-4 tracking-wide text-[var(--content-secondary)]">
          Organize tags
        </h2>
        <ul className="flex flex-col">
          {tags.map(tag => {
            const active = tagId === tag.id
            return (
              <li key={tag.id} className="group flex items-center">
                <button
                  type="button"
                  onClick={() => onTag(active ? null : tag.id)}
                  className={`${row} min-w-0 flex-1 ${
                    active
                      ? 'bg-[var(--bg-accent-03)] font-bold text-[var(--link-web)]'
                      : 'text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]'
                  }`}
                >
                  <span
                    aria-hidden
                    className="h-2.5 w-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: tag.color || COLOURS[0] }}
                  />
                  <span className="truncate">{tag.name}</span>
                  <span className="ml-auto text-[14px] font-normal text-[var(--content-secondary)]">
                    {tag.projectIds.length}
                  </span>
                </button>
                <Dropdown align="end">
                  <DropdownToggle
                    bsPrefix="tag-actions-toggle"
                    aria-label={`Actions for ${tag.name}`}
                    className="flex h-6 w-6 shrink-0 items-center justify-center rounded-[4px] text-[var(--content-secondary)] opacity-0 hover:bg-[var(--hover-interaction)] group-hover:opacity-100"
                  >
                    ⋯
                  </DropdownToggle>
                  <DropdownMenu>
                    <li role="none">
                      <DropdownItem as="button" onClick={() => setEditing(tag)}>
                        Rename
                      </DropdownItem>
                    </li>
                    <li role="none">
                      <DropdownItem
                        as="button"
                        variant="danger"
                        onClick={() => void run(() => deleteTag(tag.id))}
                      >
                        Delete tag
                      </DropdownItem>
                    </li>
                  </DropdownMenu>
                </Dropdown>
              </li>
            )
          })}
        </ul>
        <button
          type="button"
          onClick={() => setEditing('new')}
          className="mt-1 flex items-center gap-1.5 px-3 py-1 text-[16px] leading-6 text-[var(--link-web)] hover:underline"
        >
          <span aria-hidden className="text-[18px] leading-none">
            +
          </span>
          New tag
        </button>
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
  const [colour, setColour] = useState(tag?.color ?? COLOURS[0] ?? '#098842')

  return (
    <OLModal show onHide={onCancel}>
        <form
          onSubmit={event => {
            event.preventDefault()
            if (name.trim() && !busy) {
              onConfirm(name.trim(), colour)
            }
          }}
        >
          <OLModalHeader>
            <OLModalTitle>{tag ? 'Rename tag' : 'New tag'}</OLModalTitle>
          </OLModalHeader>
          <OLModalBody className="flex flex-col gap-4">
            <TextField
              autoFocus
              label="Name"
              value={name}
              onChange={event => setName(event.target.value)}
            />
            <div>
              <span className="mb-1 block text-[14px] font-medium leading-5">Colour</span>
              <div className="flex gap-2">
                {COLOURS.map(each => (
                  <button
                    key={each}
                    type="button"
                    aria-label={`Colour ${each}`}
                    aria-pressed={colour === each}
                    onClick={() => setColour(each)}
                    className={`h-6 w-6 rounded-full border-2 ${
                      colour === each ? 'border-[var(--content-primary)]' : 'border-transparent'
                    }`}
                    style={{ backgroundColor: each }}
                  />
                ))}
              </div>
            </div>
            {error ? (
              <p className="text-[14px] text-[var(--content-danger)]">{error}</p>
            ) : null}
          </OLModalBody>
          <OLModalFooter>
            <Button kind="ghost" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" loading={busy} disabled={!name.trim()}>
              {tag ? 'Rename' : 'Create'}
            </Button>
          </OLModalFooter>
        </form>
    </OLModal>
  )
}
