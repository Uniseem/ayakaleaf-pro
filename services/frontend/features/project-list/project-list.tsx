'use client'

/**
 * Everything somebody has.
 *
 * The list is held here and filtered, sorted and searched in the browser. That
 * is a deliberate limit: it is right for the number of projects a person has,
 * and it makes every control instant. If somebody ever has enough projects for
 * it to stop being right, the fix is paging in the API, not a spinner on every
 * click.
 */

import {
  Button,
  Checkbox,
  Chip,
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
import { useCallback, useMemo, useState } from 'react'
import {
  cloneProject,
  compareProjects,
  createProject,
  listProjects,
  listTags,
  deleteProject,
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

  const counts = useMemo(
    () => ({
      all: projects.filter(p => matchesFilter(p, 'all', userId)).length,
      owned: projects.filter(p => matchesFilter(p, 'owned', userId)).length,
      shared: projects.filter(p => matchesFilter(p, 'shared', userId)).length,
      archived: projects.filter(p => p.archived).length,
      trashed: projects.filter(p => p.trashed).length,
    }),
    [projects, userId]
  )

  const chosen = shown.filter(project => selected.has(project.id))
  const allChosen = shown.length > 0 && chosen.length === shown.length

  return (
    <div className="flex min-h-[calc(100dvh-3.5rem)]">
      <ProjectSidebar
        filter={filter}
        onFilter={next => {
          setFilter(next)
          setTagId(null)
          setSelected(new Set())
        }}
        counts={counts}
        tags={tags}
        tagId={tagId}
        onTag={id => {
          setTagId(id)
          setSelected(new Set())
        }}
        onChanged={refresh}
      />

      <main className="min-w-0 flex-1 p-4">
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Input
            size="sm"
            className="max-w-xs"
            placeholder="Search projects"
            value={query}
            onValueChange={setQuery}
            isClearable
            onClear={() => setQuery('')}
          />

          <Dropdown>
            <DropdownTrigger>
              <Button size="sm" variant="flat" className="h-8">
                Sort: {sort.key === 'lastUpdated' ? 'Last modified' : sort.key === 'name' ? 'Name' : 'Owner'}
              </Button>
            </DropdownTrigger>
            <DropdownMenu
              aria-label="Sort"
              onAction={key => {
                const next = String(key) as SortKey
                setSort(current =>
                  current.key === next
                    ? { key: next, ascending: !current.ascending }
                    : { key: next, ascending: next === 'name' }
                )
              }}
            >
              <DropdownItem key="lastUpdated">Last modified</DropdownItem>
              <DropdownItem key="name">Name</DropdownItem>
              <DropdownItem key="owner">Last modified by</DropdownItem>
            </DropdownMenu>
          </Dropdown>

          <div className="flex-1" />

          {chosen.length > 0 ? (
            <BulkActions
              chosen={chosen}
              filter={filter}
              tags={tags}
              busy={busy}
              onRun={run}
              onDelete={() => setAsk({ kind: 'delete', projects: chosen })}
            />
          ) : (
            <Button size="sm" color="primary" className="h-8" onPress={() => setAsk({ kind: 'new' })}>
              New project
            </Button>
          )}
        </div>

        {error ? (
          <p className="mb-2 rounded bg-danger-50 px-3 py-2 text-sm text-danger">{error}</p>
        ) : null}

        <div className="overflow-hidden rounded-lg border border-divider">
          <div className="flex items-center gap-3 border-b border-divider bg-default-50 px-3 py-2 text-xs font-medium text-default-500">
            <Checkbox
              size="sm"
              isSelected={allChosen}
              isIndeterminate={chosen.length > 0 && !allChosen}
              onValueChange={on =>
                setSelected(on ? new Set(shown.map(p => p.id)) : new Set())
              }
              aria-label="Select every project shown"
            />
            <span className="flex-1">
              {shown.length} project{shown.length === 1 ? '' : 's'}
            </span>
            <span className="hidden w-40 sm:block">Last modified</span>
            <span className="w-8" />
          </div>

          {shown.length === 0 ? (
            <p className="px-3 py-10 text-center text-sm text-default-400">
              {query
                ? 'Nothing matches that.'
                : filter === 'trashed'
                  ? 'The bin is empty.'
                  : filter === 'archived'
                    ? 'Nothing archived.'
                    : 'No projects yet.'}
            </p>
          ) : (
            <ul className="divide-y divide-divider">
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
            </ul>
          )}
        </div>
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
      <Chip size="sm" variant="flat">
        {chosen.length} selected
      </Chip>

      {tags.length > 0 ? (
        <Dropdown>
          <DropdownTrigger>
            <Button size="sm" variant="flat" className="h-8" isDisabled={busy}>
              Tag
            </Button>
          </DropdownTrigger>
          <DropdownMenu
            aria-label="Apply a tag"
            onAction={key => {
              const tag = tags.find(each => each.id === String(key))
              if (!tag) {
                return
              }
              // If every chosen project already has it, this removes it. One
              // control that does the obvious thing in both directions.
              const everyone = chosen.every(project =>
                tag.projectIds.includes(project.id)
              )
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
          >
            {tags.map(tag => (
              <DropdownItem key={tag.id}>{tag.name}</DropdownItem>
            ))}
          </DropdownMenu>
        </Dropdown>
      ) : null}

      {filter === 'trashed' ? (
        <>
          <Button
            size="sm"
            variant="flat"
            className="h-8"
            isDisabled={busy}
            onPress={() =>
              onRun(() =>
                Promise.all(chosen.map(project => setTrashed(project.id, false)))
              )
            }
          >
            Restore
          </Button>
          <Button size="sm" color="danger" variant="flat" className="h-8" onPress={onDelete}>
            Delete forever
          </Button>
        </>
      ) : filter === 'archived' ? (
        <Button
          size="sm"
          variant="flat"
          className="h-8"
          isDisabled={busy}
          onPress={() =>
            onRun(() =>
              Promise.all(chosen.map(project => setArchived(project.id, false)))
            )
          }
        >
          Unarchive
        </Button>
      ) : (
        <>
          <Button
            size="sm"
            variant="flat"
            className="h-8"
            isDisabled={busy}
            onPress={() =>
              onRun(() =>
                Promise.all(chosen.map(project => setArchived(project.id, true)))
              )
            }
          >
            Archive
          </Button>
          <Button
            size="sm"
            variant="flat"
            className="h-8"
            isDisabled={busy}
            onPress={() =>
              onRun(() =>
                Promise.all(chosen.map(project => setTrashed(project.id, true)))
              )
            }
          >
            Move to bin
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
      <Modal isOpen onClose={onCancel} size="sm">
        <ModalContent>
          <ModalHeader>
            Delete {many ? `${ask.projects.length} projects` : ask.projects[0]?.name}?
          </ModalHeader>
          <ModalBody>
            <p className="text-sm text-default-600">
              This cannot be undone. Everything in {many ? 'them' : 'it'} goes
              too, including the history.
            </p>
            {error ? <p className="text-sm text-danger">{error}</p> : null}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onCancel} isDisabled={busy}>
              Cancel
            </Button>
            <Button color="danger" isLoading={busy} onPress={() => onConfirm('')}>
              Delete
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    )
  }

  const title =
    ask.kind === 'new' ? 'New project' : ask.kind === 'copy' ? 'Copy project' : 'Rename project'

  return (
    <Modal isOpen onClose={onCancel} size="sm">
      <ModalContent>
        <form
          onSubmit={event => {
            event.preventDefault()
            if (name.trim() && !busy) {
              onConfirm(name.trim())
            }
          }}
        >
          <ModalHeader>{title}</ModalHeader>
          <ModalBody>
            <Input
              autoFocus
              label="Name"
              value={name}
              onValueChange={setName}
              placeholder="My paper"
            />
            {error ? <p className="text-sm text-danger">{error}</p> : null}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onCancel} isDisabled={busy}>
              Cancel
            </Button>
            <Button
              color="primary"
              type="submit"
              isLoading={busy}
              isDisabled={!name.trim()}
            >
              {ask.kind === 'new' ? 'Create' : ask.kind === 'copy' ? 'Copy' : 'Rename'}
            </Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  )
}
