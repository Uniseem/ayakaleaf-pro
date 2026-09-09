'use client'

/**
 * The project's files.
 *
 * A tree of rows rather than a nest of components: the nesting is drawn with
 * indentation and the rows are a flat list, so keyboard navigation is moving
 * along an array and a deep project does not build a deep React tree.
 *
 * Everything that changes the project goes through the project context, which
 * refreshes the tree afterwards. This component never edits its own copy.
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
  Tooltip,
} from '@heroui/react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { FileEntry } from '@/lib/editor'
import { moveEntry, uploadFile } from '@/lib/editor'
import { messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { buildTree, canMoveInto, checkName, namesIn, visibleRows } from './tree'
import { EntryIcon } from './entry-icon'

/** The folder a prompt is about, which is where names must be unique. */
function folderOf(prompt: Prompt): string {
  if (prompt.kind === 'rename' || prompt.kind === 'delete') {
    const slash = prompt.entry.path.lastIndexOf('/')
    return slash === -1 ? '' : prompt.entry.path.slice(0, slash)
  }
  return prompt.folderPath
}

/** Identifies a prompt, so switching between two remounts the form. */
function promptKey(prompt: Prompt): string {
  return prompt.kind === 'rename' || prompt.kind === 'delete'
    ? `${prompt.kind}:${prompt.entry.id}`
    : `${prompt.kind}:${prompt.folderId ?? ''}`
}

/** What the tree is asking the person for, when it is asking. */
type Prompt =
  | { kind: 'new-doc'; folderId?: string; folderPath: string }
  | { kind: 'new-folder'; folderId?: string; folderPath: string }
  | { kind: 'rename'; entry: FileEntry }
  | { kind: 'delete'; entry: FileEntry }

export function FileTree() {
  const project = useProject()
  const editor = useEditor()

  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [prompt, setPrompt] = useState<Prompt | null>(null)
  const [dragging, setDragging] = useState<FileEntry | null>(null)
  const [dropTarget, setDropTarget] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const uploadInput = useRef<HTMLInputElement>(null)
  const uploadInto = useRef<string | undefined>(undefined)

  const rows = useMemo(
    () => visibleRows(buildTree(project.files), collapsed),
    [project.files, collapsed]
  )

  const toggle = useCallback((path: string) => {
    setCollapsed(previous => {
      const next = new Set(previous)
      if (next.has(path)) {
        next.delete(path)
      } else {
        next.add(path)
      }
      return next
    })
  }, [])

  const run = useCallback(async (work: () => Promise<unknown>) => {
    setBusy(true)
    setError(null)
    try {
      await work()
      setPrompt(null)
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setBusy(false)
    }
  }, [])

  const onDrop = useCallback(
    async (target: FileEntry | null) => {
      const moving = dragging
      setDragging(null)
      setDropTarget(null)
      if (!moving || !canMoveInto(moving, target)) {
        return
      }
      await run(async () => {
        await moveEntry(project.projectId, moving.id, target?.id)
        await project.refresh()
      })
    },
    [dragging, project, run]
  )

  const onUpload = useCallback(
    async (files: FileList | null) => {
      if (!files || files.length === 0) {
        return
      }
      const folderId = uploadInto.current
      await run(async () => {
        for (const file of Array.from(files)) {
          await uploadFile(project.projectId, file, folderId)
        }
        await project.refresh()
      })
      uploadInto.current = undefined
      if (uploadInput.current) {
        uploadInput.current.value = ''
      }
    },
    [project, run]
  )

  const askUpload = useCallback((folderId?: string) => {
    uploadInto.current = folderId
    uploadInput.current?.click()
  }, [])

  // The toolbar above the tree and the File menu ask for these by event,
  // because neither holds a reference to the tree.
  useEffect(() => {
    const newFile = () => setPrompt({ kind: 'new-doc', folderPath: '' })
    const newFolder = () => setPrompt({ kind: 'new-folder', folderPath: '' })
    const upload = () => askUpload(undefined)
    window.addEventListener('ide:new-file', newFile)
    window.addEventListener('ide:new-folder', newFolder)
    window.addEventListener('ide:upload', upload)
    return () => {
      window.removeEventListener('ide:new-file', newFile)
      window.removeEventListener('ide:new-folder', newFolder)
      window.removeEventListener('ide:upload', upload)
    }
  }, [askUpload])

  return (
    <div className="flex min-h-0 flex-1 flex-col">

      {error ? (
        <p className="border-b border-divider bg-danger-50 px-3 py-2 text-xs text-danger">
          {error}
        </p>
      ) : null}

      <ul
        className="flex-1 overflow-auto p-1"
        onDragOver={event => {
          if (dragging) {
            event.preventDefault()
            setDropTarget('')
          }
        }}
        onDrop={event => {
          event.preventDefault()
          void onDrop(null)
        }}
      >
        {rows.length === 0 ? (
          <li className="px-3 py-6 text-sm text-default-400">
            Nothing here yet.
          </li>
        ) : (
          rows.map(node => (
            <TreeRow
              key={node.entry.id}
              entry={node.entry}
              depth={node.depth}
              collapsed={collapsed.has(node.entry.path)}
              isOpen={editor.current?.id === node.entry.id}
              isRoot={project.project.rootDocId === node.entry.id}
              canWrite={project.canWrite}
              isDropTarget={dropTarget === node.entry.path}
              onToggle={() => toggle(node.entry.path)}
              onOpen={() => editor.open(node.entry)}
              onDragStart={() => setDragging(node.entry)}
              onDragEnd={() => {
                setDragging(null)
                setDropTarget(null)
              }}
              onDragOver={() => {
                if (dragging && canMoveInto(dragging, node.entry)) {
                  setDropTarget(node.entry.path)
                }
              }}
              onDrop={() => void onDrop(node.entry)}
              onAction={key => {
                const folder =
                  node.entry.kind === 'folder' ? node.entry : undefined
                switch (key) {
                  case 'rename':
                    setPrompt({ kind: 'rename', entry: node.entry })
                    break
                  case 'delete':
                    setPrompt({ kind: 'delete', entry: node.entry })
                    break
                  case 'root':
                    void run(() => project.chooseRootDoc(node.entry.id))
                    break
                  case 'new-doc':
                    setPrompt({
                      kind: 'new-doc',
                      folderId: folder?.id,
                      folderPath: folder?.path ?? '',
                    })
                    break
                  case 'new-folder':
                    setPrompt({
                      kind: 'new-folder',
                      folderId: folder?.id,
                      folderPath: folder?.path ?? '',
                    })
                    break
                  case 'upload':
                    askUpload(folder?.id)
                    break
                }
              }}
            />
          ))
        )}
      </ul>

      <input
        ref={uploadInput}
        type="file"
        multiple
        hidden
        onChange={event => void onUpload(event.target.files)}
      />

      {prompt ? (
        <TreePrompt
          key={promptKey(prompt)}
          prompt={prompt}
          busy={busy}
          error={error}
          taken={namesIn(project.files, folderOf(prompt))}
          onCancel={() => {
            setPrompt(null)
            setError(null)
          }}
          onConfirm={name => {
            switch (prompt.kind) {
              case 'new-doc':
                void run(async () => {
                  const created = await project.addDoc(name, prompt.folderId)
                  if (created) {
                    editor.open(created)
                  }
                })
                break
              case 'new-folder':
                void run(() => project.addFolder(name, prompt.folderId))
                break
              case 'rename':
                void run(() => project.rename(prompt.entry.id, name))
                break
              case 'delete':
                void run(() => project.remove(prompt.entry.id))
                break
            }
          }}
        />
      ) : null}
    </div>
  )
}

function TreeAction({
  label,
  onPress,
  children,
}: {
  label: string
  onPress: () => void
  children: React.ReactNode
}) {
  return (
    <Tooltip content={label} delay={400} closeDelay={0}>
      <Button
        isIconOnly
        size="sm"
        variant="light"
        className="h-6 w-6 min-w-6"
        aria-label={label}
        onPress={onPress}
      >
        {children}
      </Button>
    </Tooltip>
  )
}

function TreeRow(props: {
  entry: FileEntry
  depth: number
  collapsed: boolean
  isOpen: boolean
  isRoot: boolean
  canWrite: boolean
  isDropTarget: boolean
  onToggle: () => void
  onOpen: () => void
  onDragStart: () => void
  onDragEnd: () => void
  onDragOver: () => void
  onDrop: () => void
  onAction: (key: string) => void
}) {
  const { entry } = props
  const isFolder = entry.kind === 'folder'

  return (
    <li>
      <div
        draggable={props.canWrite}
        onDragStart={props.onDragStart}
        onDragEnd={props.onDragEnd}
        onDragOver={event => {
          event.preventDefault()
          event.stopPropagation()
          props.onDragOver()
        }}
        onDrop={event => {
          event.preventDefault()
          event.stopPropagation()
          props.onDrop()
        }}
        className={[
          'group flex items-center gap-1 rounded-md pr-1',
          props.isOpen ? 'bg-default-200' : 'hover:bg-default-100',
          props.isDropTarget ? 'ring-1 ring-primary' : '',
        ].join(' ')}
        style={{ paddingLeft: `${props.depth * 12 + 4}px` }}
      >
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-1.5 py-1 text-left text-sm"
          onClick={isFolder ? props.onToggle : props.onOpen}
          title={entry.path}
        >
          <EntryIcon
            kind={entry.kind}
            name={entry.name}
            collapsed={props.collapsed}
          />
          <span className="truncate">{entry.name}</span>
          {props.isRoot ? (
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
                <MoreIcon />
              </Button>
            </DropdownTrigger>
            <DropdownMenu
              aria-label={`Actions for ${entry.name}`}
              onAction={key => props.onAction(String(key))}
            >
              <DropdownItem key="rename">Rename</DropdownItem>
              {entry.kind === 'doc' ? (
                <DropdownItem key="root">Compile this file</DropdownItem>
              ) : null}
              {isFolder ? <DropdownItem key="new-doc">New file here</DropdownItem> : null}
              {isFolder ? (
                <DropdownItem key="new-folder">New folder here</DropdownItem>
              ) : null}
              {isFolder ? <DropdownItem key="upload">Upload here</DropdownItem> : null}
              <DropdownItem key="delete" color="danger" className="text-danger">
                Delete
              </DropdownItem>
            </DropdownMenu>
          </Dropdown>
        ) : null}
      </div>
    </li>
  )
}

function TreePrompt({
  prompt,
  busy,
  error,
  taken,
  onCancel,
  onConfirm,
}: {
  prompt: Prompt
  busy: boolean
  error: string | null
  taken: ReadonlySet<string>
  onCancel: () => void
  onConfirm: (name: string) => void
}) {
  const existing = prompt.kind === 'rename' ? prompt.entry.name : ''
  const [name, setName] = useState(existing)

  if (prompt.kind === 'delete') {
    return (
      <Modal isOpen onClose={onCancel} size="sm">
        <ModalContent>
          <ModalHeader>Delete {prompt.entry.name}?</ModalHeader>
          <ModalBody>
            <p className="text-sm text-default-600">
              {prompt.entry.kind === 'folder'
                ? 'Everything inside it goes too. This cannot be undone.'
                : 'This cannot be undone.'}
            </p>
            {error ? <p className="text-sm text-danger">{error}</p> : null}
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={onCancel} isDisabled={busy}>
              Cancel
            </Button>
            <Button
              color="danger"
              onPress={() => onConfirm(prompt.entry.name)}
              isLoading={busy}
            >
              Delete
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    )
  }

  // A rename keeps its own name available; everything else is taken.
  const unavailable =
    prompt.kind === 'rename'
      ? new Set([...taken].filter(each => each !== existing))
      : taken
  const problem = name === existing && prompt.kind === 'rename'
    ? null
    : checkName(name, unavailable)

  const title =
    prompt.kind === 'new-doc'
      ? 'New file'
      : prompt.kind === 'new-folder'
        ? 'New folder'
        : `Rename ${existing}`

  return (
    <Modal isOpen onClose={onCancel} size="sm">
      <ModalContent>
        <form
          onSubmit={event => {
            event.preventDefault()
            if (!problem && !busy) {
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
              isInvalid={Boolean(name && problem)}
              errorMessage={name ? problem : null}
              placeholder={prompt.kind === 'new-folder' ? 'chapters' : 'section.tex'}
            />
            {folderOf(prompt) ? (
              <p className="text-xs text-default-500">in {folderOf(prompt)}</p>
            ) : null}
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
              isDisabled={Boolean(problem)}
            >
              {prompt.kind === 'rename' ? 'Rename' : 'Create'}
            </Button>
          </ModalFooter>
        </form>
      </ModalContent>
    </Modal>
  )
}

function PlusIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M8 3v10M3 8h10" strokeLinecap="round" />
    </svg>
  )
}

function FolderPlusIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h2.6l1.2 1.5h5.2A1.5 1.5 0 0 1 14 6v5.5A1.5 1.5 0 0 1 12.5 13h-9A1.5 1.5 0 0 1 2 11.5z" />
      <path d="M8 7v4M6 9h4" strokeLinecap="round" />
    </svg>
  )
}

function UploadIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M8 11V3M5 6l3-3 3 3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.5 10.5v1A1.5 1.5 0 0 0 4 13h8a1.5 1.5 0 0 0 1.5-1.5v-1" strokeLinecap="round" />
    </svg>
  )
}

function MoreIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="currentColor">
      <circle cx="8" cy="3.5" r="1.2" />
      <circle cx="8" cy="8" r="1.2" />
      <circle cx="8" cy="12.5" r="1.2" />
    </svg>
  )
}
