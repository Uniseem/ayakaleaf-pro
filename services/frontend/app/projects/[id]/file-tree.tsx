'use client'

import { Button, Dropdown, DropdownItem, DropdownMenu, DropdownTrigger } from '@heroui/react'
import { useMemo, useState, type ReactElement } from 'react'
import type { FileEntry } from '@/lib/editor'

/**
 * The list of files in a project.
 *
 * The API sends the tree flat, each entry with its path, so the nesting is
 * rebuilt here from the paths rather than from a shape the API had to agree
 * to. That means one representation crosses the wire and the way it is drawn
 * is this component's business alone.
 */

type Node = {
  entry: FileEntry
  children: Node[]
}

function buildTree(entries: FileEntry[]): Node[] {
  const byPath = new Map<string, Node>()
  const roots: Node[] = []

  // A parent's path is a prefix of its children's, so sorting by path puts
  // every folder before what is inside it and one pass is enough.
  for (const entry of [...entries].sort((a, b) => a.path.localeCompare(b.path))) {
    const node: Node = { entry, children: [] }
    byPath.set(entry.path, node)
    const slash = entry.path.lastIndexOf('/')
    const parent = slash < 0 ? undefined : byPath.get(entry.path.slice(0, slash))
    ;(parent ? parent.children : roots).push(node)
  }

  const order = (nodes: Node[]) => {
    nodes.sort((a, b) => {
      if ((a.entry.kind === 'folder') !== (b.entry.kind === 'folder')) {
        return a.entry.kind === 'folder' ? -1 : 1
      }
      return a.entry.name.localeCompare(b.entry.name)
    })
    nodes.forEach(node => order(node.children))
  }
  order(roots)
  return roots
}

function Icon({ kind }: { kind: FileEntry['kind'] }) {
  const glyph = kind === 'folder' ? '▸' : kind === 'doc' ? '≡' : '▪'
  return (
    <span aria-hidden className="w-3 shrink-0 text-center text-default-400">
      {glyph}
    </span>
  )
}

export type FileTreeProps = {
  files: FileEntry[]
  openId: string | null
  rootDocId?: string
  canWrite: boolean
  onOpen: (entry: FileEntry) => void
  onRename: (entry: FileEntry) => void
  onDelete: (entry: FileEntry) => void
  onSetRoot: (entry: FileEntry) => void
  onCreate: (folderId: string | undefined, kind: 'doc' | 'folder') => void
}

export function FileTree(props: FileTreeProps) {
  const tree = useMemo(() => buildTree(props.files), [props.files])
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())

  function toggle(path: string) {
    setCollapsed(previous => {
      const next = new Set(previous)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return next
    })
  }

  function rows(nodes: Node[], depth: number): ReactElement[] {
    return nodes.flatMap(node => {
      const { entry } = node
      const isOpen = entry.id === props.openId
      const isFolder = entry.kind === 'folder'
      const shut = collapsed.has(entry.path)

      const row = (
        <li key={entry.id}>
          <div
            className={`group flex items-center gap-1 rounded-md pr-1 ${
              isOpen ? 'bg-default-200' : 'hover:bg-default-100'
            }`}
            style={{ paddingLeft: `${depth * 12 + 4}px` }}
          >
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-1.5 py-1 text-left text-sm"
              onClick={() => (isFolder ? toggle(entry.path) : props.onOpen(entry))}
              title={entry.path}
            >
              <Icon kind={entry.kind} />
              <span className="truncate">{entry.name}</span>
              {entry.id === props.rootDocId ? (
                <span
                  className="ml-1 shrink-0 rounded bg-primary/15 px-1 text-[10px] font-medium text-primary"
                  title="This is the file that gets compiled"
                >
                  main
                </span>
              ) : null}
            </button>

            {props.canWrite ? (
              <Dropdown placement="bottom-end">
                <DropdownTrigger>
                  <Button
                    isIconOnly
                    size="sm"
                    variant="light"
                    className="h-6 w-6 min-w-6 opacity-0 group-hover:opacity-100 data-[focus-visible=true]:opacity-100"
                    aria-label={`Actions for ${entry.name}`}
                  >
                    ⋯
                  </Button>
                </DropdownTrigger>
                <DropdownMenu aria-label={`Actions for ${entry.name}`}>
                  <DropdownItem key="rename" onPress={() => props.onRename(entry)}>
                    Rename
                  </DropdownItem>
                  {entry.kind === 'doc' ? (
                    <DropdownItem key="root" onPress={() => props.onSetRoot(entry)}>
                      Compile this file
                    </DropdownItem>
                  ) : null}
                  {isFolder ? (
                    <DropdownItem key="new-file" onPress={() => props.onCreate(entry.id, 'doc')}>
                      New file here
                    </DropdownItem>
                  ) : null}
                  <DropdownItem
                    key="delete"
                    color="danger"
                    className="text-danger"
                    onPress={() => props.onDelete(entry)}
                  >
                    Delete
                  </DropdownItem>
                </DropdownMenu>
              </Dropdown>
            ) : null}
          </div>
        </li>
      )

      if (isFolder && !shut && node.children.length > 0) {
        return [row, ...rows(node.children, depth + 1)]
      }
      return [row]
    })
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-divider px-3 py-2">
        <span className="text-xs font-semibold uppercase tracking-wide text-default-500">
          Files
        </span>
        {props.canWrite ? (
          <div className="flex gap-1">
            <Button
              isIconOnly
              size="sm"
              variant="light"
              className="h-6 w-6 min-w-6"
              aria-label="New file"
              title="New file"
              onPress={() => props.onCreate(undefined, 'doc')}
            >
              +
            </Button>
            <Button
              isIconOnly
              size="sm"
              variant="light"
              className="h-6 w-6 min-w-6"
              aria-label="New folder"
              title="New folder"
              onPress={() => props.onCreate(undefined, 'folder')}
            >
              ⊞
            </Button>
          </div>
        ) : null}
      </div>

      <ul className="flex-1 overflow-auto p-1">
        {tree.length === 0 ? (
          <li className="px-3 py-6 text-sm text-default-400">
            Nothing here yet.
          </li>
        ) : (
          rows(tree, 0)
        )}
      </ul>
    </div>
  )
}
