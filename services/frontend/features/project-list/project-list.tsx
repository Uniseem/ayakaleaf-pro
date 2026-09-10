'use client'

/**
 * Everything somebody has.
 *
 * The list is held here and filtered, sorted and searched in the browser. That
 * is a deliberate limit: it is right for the number of projects a person has,
 * and it makes every control instant. If somebody ever has enough projects for
 * it to stop being right, the fix is paging in the API, not a spinner on every
 * click.
 *
 * Laid out as the original is: a 200px sidebar, a title, a search field with
 * the magnifier inside it, and a table whose column headers are how it is
 * sorted.
 */

import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { useCallback, useMemo, useState } from 'react'
import {
  cloneProject,
  compareProjects,
  createProject,
  deleteProject,
  listProjects,
  listTags,
  matchesFilter,
  renameProject,
  setArchived,
  setTrashed,
  tagProject,
  untagProject,
  type Filter,
  type ProjectSummary,
  type SortKey,
  type Tag,
} from '@/lib/projects'
import { messageFor } from '@/lib/api'
import { usePersistedState } from '@/lib/hooks'
import { Button, TextField } from '@/components/ui'
import { ProjectSidebar } from './sidebar'
import { ProjectRow } from './row'

/** Identifies a dialog, so switching target remounts the form. */
function askKey(ask: Ask): string {
  switch (ask.kind) {
    case 'new':
      return 'new'
    case 'delete':
      return 'delete:' + ask.projects.map(p => p.id).join(',')
    default:
      return `${ask.kind}:${ask.project.id}`
  }
}

type Ask =
  | { kind: 'new' }
  | { kind: 'rename'; project: ProjectSummary }
  | { kind: 'copy'; project: ProjectSummary }
  | { kind: 'delete'; projects: ProjectSummary[] }

const HEADINGS: { key: SortKey; label: string; className: string }[] = [
  { key: 'name', label: 'Title', className: 'px-2' },
  { key: 'owner', label: 'Owner', className: 'hidden px-2 sm:table-cell' },
  { key: 'lastUpdated', label: 'Last modified', className: 'hidden px-2 md:table-cell' },
]

export function ProjectList({
  initial,
  initialTags,
  userId,
}: {
  initial: ProjectSummary[]
  initialTags: Tag[]
  userId: string
}) {
  const [projects, setProjects] = useState(initial)
  const [tags, setTags] = useState(initialTags)
  const [filter, setFilter] = usePersistedState<Filter>('projects.filter', 'all')
  const [tagId, setTagId] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [sort, setSort] = usePersistedState<{ key: SortKey; ascending: boolean }>(
    'projects.sort',
    { key: 'lastUpdated', ascending: false }
  )
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [ask, setAsk] = useState<Ask | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    const [freshProjects, freshTags] = await Promise.all([
      listProjects(),
      // Tags are optional: an instance that has never had one answers with
      // nothing, and that is not a reason to fail loading the projects.
      listTags().catch(() => null),
    ])
    setProjects(freshProjects)
    if (freshTags) {
      setTags(freshTags)
    }
    setSelected(new Set())
  }, [])

  const run = useCallback(
    async (work: () => Promise<unknown>) => {
      setBusy(true)
      setError(null)
      try {
        await work()
        await refresh()
        setAsk(null)
      } catch (thrown) {
        setError(messageFor(thrown))
      } finally {
        setBusy(false)
      }
    },
    [refresh]
  )

  const shown = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const tagged = tagId ? tags.find(tag => tag.id === tagId) : null
    return projects
      .filter(project => matchesFilter(project, filter, userId))
      .filter(project => !tagged || tagged.projectIds.includes(project.id))
      .filter(project => !needle || project.name.toLowerCase().includes(needle))
      .sort((a, b) => compareProjects(a, b, sort.key, sort.ascending))
  }, [projects, filter, tagId, tags, query, sort, userId])

  const total = useMemo(
    () => projects.filter(project => matchesFilter(project, filter, userId)).length,
    [projects, filter, userId]
  )

  const chosen = shown.filter(project => selected.has(project.id))
  const allChosen = shown.length > 0 && chosen.length === shown.length

  const title = {
    all: 'All projects',
    owned: 'Your projects',
    shared: 'Shared with you',
    archived: 'Archived projects',
    trashed: 'Trashed projects',
  }[filter]

  const tagName = tagId ? tags.find(tag => tag.id === tagId)?.name : null

  return (
    <div className="flex flex-1">
      <ProjectSidebar
        filter={filter}
        onFilter={next => {
          setFilter(next)
          setTagId(null)
          setSelected(new Set())
        }}
        tags={tags}
        tagId={tagId}
        onTag={id => {
          setTagId(id)
          setSelected(new Set())
        }}
        onChanged={refresh}
        onNewProject={() => setAsk({ kind: 'new' })}
      />

      <main className="min-w-0 flex-1 px-4 py-4">
        <h1 className="mb-4 text-[20px] font-bold leading-7 text-[var(--content-secondary)]">
          {tagName ?? title}
        </h1>

        <div className="mb-3 flex flex-wrap items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <SearchIcon />
            <input
              type="search"
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder={`Search in ${(tagName ?? title).toLowerCase()}…`}
              aria-label="Search projects"
              className="h-[38px] w-full rounded-[4px] border border-[var(--border-primary)] bg-[var(--bg-light-primary)] py-1.5 pl-9 pr-2 text-[16px] leading-6 text-[var(--content-primary)] placeholder:text-[var(--content-placeholder)] focus:border-[var(--border-active)] focus:outline-none"
            />
          </div>

          {chosen.length > 0 ? (
            <BulkActions
              chosen={chosen}
              filter={filter}
              tags={tags}
              busy={busy}
              onRun={run}
              onDelete={() => setAsk({ kind: 'delete', projects: chosen })}
            />
          ) : null}
        </div>

        {error ? (
          <p className="mb-3 rounded-[4px] bg-[var(--bg-danger-03)] px-4 py-3 text-[14px] leading-5 text-[var(--content-danger)]">
            {error}
          </p>
        ) : null}

        <div className="overflow-hidden rounded-[8px] border border-[var(--border-divider)] bg-[var(--bg-light-primary)]">
          <table className="w-full table-auto">
            <thead>
              <tr className="border-b border-[var(--border-divider)]">
                <th className="w-10 px-3 py-2.5 text-left">
                  <input
                    type="checkbox"
                    checked={allChosen}
                    ref={element => {
                      if (element) {
                        element.indeterminate = chosen.length > 0 && !allChosen
                      }
                    }}
                    onChange={event =>
                      setSelected(
                        event.target.checked ? new Set(shown.map(p => p.id)) : new Set()
                      )
                    }
                    aria-label="Select every project shown"
                    className="h-4 w-4 accent-[var(--bg-accent-01)]"
                  />
                </th>
                {HEADINGS.map(heading => (
                  <th
                    key={heading.key}
                    className={`${heading.className} py-2.5 text-left text-[16px] font-normal leading-6 text-[var(--content-secondary)]`}
                  >
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 hover:underline"
                      onClick={() =>
                        setSort(current =>
                          current.key === heading.key
                            ? { key: heading.key, ascending: !current.ascending }
                            : { key: heading.key, ascending: heading.key === 'name' }
                        )
                      }
                    >
                      {heading.label}
                      {sort.key === heading.key ? (
                        <span aria-hidden>{sort.ascending ? '↑' : '↓'}</span>
                      ) : null}
                    </button>
                  </th>
                ))}
                <th className="px-3 py-2.5 text-right text-[16px] font-normal leading-6 text-[var(--content-secondary)]">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {shown.map(project => (
                <ProjectRow
                  key={project.id}
                  project={project}
                  tags={tags.filter(tag => tag.projectIds.includes(project.id))}
                  isSelected={selected.has(project.id)}
                  onSelect={on =>
                    setSelected(previous => {
                      const next = new Set(previous)
                      if (on) {
                        next.add(project.id)
                      } else {
                        next.delete(project.id)
                      }
                      return next
                    })
                  }
                  onRename={() => setAsk({ kind: 'rename', project })}
                  onCopy={() => setAsk({ kind: 'copy', project })}
                  onDelete={() => setAsk({ kind: 'delete', projects: [project] })}
                  onArchive={on => void run(() => setArchived(project.id, on))}
                  onTrash={on => void run(() => setTrashed(project.id, on))}
                />
              ))}
            </tbody>
          </table>

          {shown.length === 0 ? (
            <p className="px-3 py-10 text-center text-[16px] leading-6 text-[var(--content-secondary)]">
              {query
                ? 'No projects match that search.'
                : filter === 'trashed'
                  ? 'No trashed projects.'
                  : filter === 'archived'
                    ? 'No archived projects.'
                    : 'No projects yet.'}
            </p>
          ) : null}
        </div>

        {shown.length > 0 ? (
          <p className="py-4 text-center text-[16px] leading-6 text-[var(--content-secondary)]">
            Showing {shown.length} out of {total} project{total === 1 ? '' : 's'}.
          </p>
        ) : null}
      </main>

      {ask ? (
        <AskDialog
          key={askKey(ask)}
          ask={ask}
          busy={busy}
          error={error}
          onCancel={() => {
            setAsk(null)
            setError(null)
          }}
          onConfirm={name => {
            switch (ask.kind) {
              case 'new':
                void run(() => createProject(name))
                break
              case 'rename':
                void run(() => renameProject(ask.project.id, name))
                break
              case 'copy':
                void run(() => cloneProject(ask.project.id, name))
                break
              case 'delete':
                void run(() =>
                  Promise.all(ask.projects.map(project => deleteProject(project.id)))
                )
                break
            }
          }}
        />
      ) : null}
    </div>
  )
}

function SearchIcon() {
  return (
    <svg
      viewBox="0 0 16 16"
      className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--content-placeholder)]"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      aria-hidden
    >
      <circle cx="7" cy="7" r="4.2" />
      <path d="m10.2 10.2 3.3 3.3" strokeLinecap="round" />
    </svg>
  )
}

function BulkActions({
  chosen,
  filter,
  tags,
  busy,
  onRun,
  onDelete,
}: {
  chosen: ProjectSummary[]
  filter: Filter
  tags: Tag[]
  busy: boolean
  onRun: (work: () => Promise<unknown>) => void
  onDelete: () => void
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="text-[14px] leading-5 text-[var(--content-secondary)]">
        {chosen.length} selected
      </span>

      {tags.length > 0 ? (
        <select
          aria-label="Apply a tag"
          value=""
          disabled={busy}
          onChange={event => {
            const tag = tags.find(each => each.id === event.target.value)
            if (!tag) {
              return
            }
            // If every chosen project already has it, this removes it. One
            // control that does the obvious thing in both directions.
            const everyone = chosen.every(project => tag.projectIds.includes(project.id))
            onRun(() =>
              Promise.all(
                chosen.map(project =>
                  everyone
                    ? untagProject(tag.id, project.id)
                    : tagProject(tag.id, project.id)
                )
              )
            )
          }}
          className="h-9 rounded-full border-2 border-[var(--border-primary)] bg-transparent px-3 text-[14px] leading-5"
        >
          <option value="">Tag</option>
          {tags.map(tag => (
            <option key={tag.id} value={tag.id}>
              {tag.name}
            </option>
          ))}
        </select>
      ) : null}

      {filter === 'trashed' ? (
        <>
          <Button
            kind="secondary"
            disabled={busy}
            onClick={() =>
              onRun(() => Promise.all(chosen.map(p => setTrashed(p.id, false))))
            }
          >
            Restore
          </Button>
          <Button kind="danger" onClick={onDelete}>
            Delete forever
          </Button>
        </>
      ) : filter === 'archived' ? (
        <Button
          kind="secondary"
          disabled={busy}
          onClick={() =>
            onRun(() => Promise.all(chosen.map(p => setArchived(p.id, false))))
          }
        >
          Unarchive
        </Button>
      ) : (
        <>
          <Button
            kind="secondary"
            disabled={busy}
            onClick={() =>
              onRun(() => Promise.all(chosen.map(p => setArchived(p.id, true))))
            }
          >
            Archive
          </Button>
          <Button
            kind="secondary"
            disabled={busy}
            onClick={() =>
              onRun(() => Promise.all(chosen.map(p => setTrashed(p.id, true))))
            }
          >
            Trash
          </Button>
        </>
      )}
    </div>
  )
}

function AskDialog({
  ask,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  ask: Ask
  busy: boolean
  error: string | null
  onCancel: () => void
  onConfirm: (name: string) => void
}) {
  const suggested =
    ask.kind === 'rename'
      ? ask.project.name
      : ask.kind === 'copy'
        ? `${ask.project.name} (copy)`
        : ''
  const [name, setName] = useState(suggested)

  if (ask.kind === 'delete') {
    const many = ask.projects.length > 1
    return (
      <OLModal show onHide={onCancel}>
          <OLModalHeader>
            <OLModalTitle>
              Delete {many ? `${ask.projects.length} projects` : ask.projects[0]?.name}?
            </OLModalTitle>
          </OLModalHeader>
          <OLModalBody>
            <p className="text-[16px] leading-6 text-[var(--content-primary)]">
              This cannot be undone. Everything in {many ? 'them' : 'it'} goes too,
              including the history.
            </p>
            {error ? (
              <p className="text-[14px] text-[var(--content-danger)]">{error}</p>
            ) : null}
          </OLModalBody>
          <OLModalFooter>
            <Button kind="ghost" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
            <Button kind="danger" loading={busy} onClick={() => onConfirm('')}>
              Delete
            </Button>
          </OLModalFooter>
      </OLModal>
    )
  }

  const title =
    ask.kind === 'new' ? 'New project' : ask.kind === 'copy' ? 'Copy project' : 'Rename project'

  return (
    <OLModal show onHide={onCancel}>
        <form
          onSubmit={event => {
            event.preventDefault()
            if (name.trim() && !busy) {
              onConfirm(name.trim())
            }
          }}
        >
          <OLModalHeader>
            <OLModalTitle>{title}</OLModalTitle>
          </OLModalHeader>
          <OLModalBody>
            <TextField
              autoFocus
              label="Name"
              value={name}
              onChange={event => setName(event.target.value)}
              placeholder="My paper"
            />
            {error ? (
              <p className="text-[14px] text-[var(--content-danger)]">{error}</p>
            ) : null}
          </OLModalBody>
          <OLModalFooter>
            <Button kind="ghost" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" loading={busy} disabled={!name.trim()}>
              {ask.kind === 'new' ? 'Create' : ask.kind === 'copy' ? 'Copy' : 'Rename'}
            </Button>
          </OLModalFooter>
        </form>
    </OLModal>
  )
}
