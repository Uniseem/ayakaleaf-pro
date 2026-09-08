'use client'

import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Snippet,
  Spinner,
} from '@heroui/react'
import Link from 'next/link'
import dynamic from 'next/dynamic'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { messageFor } from '@/lib/api'
import { displayName, type PublicUser } from '@/lib/auth'
import {
  compileProject,
  createDocument,
  createFolder,
  deleteEntry,
  getDocument,
  renameEntry,
  saveDocument,
  setRootDoc,
  type CompileResult,
  type FileEntry,
  type ProjectView,
} from '@/lib/editor'
import { canWrite } from '@/lib/projects'
import { FileTree } from './file-tree'
import { PdfPane } from './pdf-pane'

// CodeMirror measures the DOM as it mounts, so it is loaded in the browser
// only. The rest of this page still renders on the server.
const CodePane = dynamic(() => import('./code-pane').then(module => module.CodePane), {
  ssr: false,
  loading: () => (
    <div className="flex h-full items-center justify-center text-sm text-default-400">
      Loading the editor…
    </div>
  ),
})

/** How long after the last keystroke the text is written back. */
const SAVE_AFTER_MS = 1200

type Prompt =
  | { kind: 'new-doc' | 'new-folder'; folderId?: string; value: string }
  | { kind: 'rename'; entry: FileEntry; value: string }
  | { kind: 'delete'; entry: FileEntry }

export function Editor({
  user,
  view,
  git,
}: {
  user: PublicUser
  view: ProjectView
  git: boolean
}) {
  const projectId = view.project.id
  const writable = canWrite(view.access)

  const [files, setFiles] = useState<FileEntry[]>(view.files)
  const [rootDocId, setRootDocIdState] = useState<string | undefined>(view.project.rootDocId)
  const [openId, setOpenId] = useState<string | null>(null)
  const [content, setContent] = useState('')
  const [loadingDoc, setLoadingDoc] = useState(false)

  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [compiling, setCompiling] = useState(false)
  const [result, setResult] = useState<CompileResult | null>(null)
  const [error, setError] = useState('')
  const [showPdf, setShowPdf] = useState(true)
  const [prompt, setPrompt] = useState<Prompt | null>(null)
  const [working, setWorking] = useState(false)
  const [showClone, setShowClone] = useState(false)

  // What has been typed but not written back yet. A ref rather than state
  // because saving must see the latest text, not the text as it was when a
  // timer was set.
  const pending = useRef<{ docId: string; content: string } | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const flush = useCallback(async () => {
    if (timer.current) {
      clearTimeout(timer.current)
      timer.current = null
    }
    const outstanding = pending.current
    if (!outstanding) {
      return
    }
    pending.current = null
    setSaving(true)
    try {
      await saveDocument(projectId, outstanding.docId, outstanding.content)
      setDirty(false)
    } catch (problem) {
      // Put it back: the text is still only in this browser, and the next
      // attempt should send it rather than pretend it was written.
      pending.current = outstanding
      setError(messageFor(problem))
    } finally {
      setSaving(false)
    }
  }, [projectId])

  const open = useCallback(
    async (entry: FileEntry) => {
      if (entry.kind !== 'doc' || entry.id === openId) {
        return
      }
      await flush()
      setOpenId(entry.id)
      setLoadingDoc(true)
      setError('')
      try {
        const doc = await getDocument(projectId, entry.id)
        setContent(doc.lines.join('\n'))
        setDirty(false)
      } catch (problem) {
        setError(messageFor(problem))
        setContent('')
      } finally {
        setLoadingDoc(false)
      }
    },
    [flush, openId, projectId]
  )

  // The file to start on: the one that gets compiled, or the first document
  // there is. Opening a project should show something.
  useEffect(() => {
    if (openId) {
      return
    }
    const docs = files.filter(file => file.kind === 'doc')
    const first = docs.find(file => file.id === rootDocId) ?? docs[0]
    if (first) {
      void open(first)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [files])

  function edit(next: string) {
    setContent(next)
    if (!openId || !writable) {
      return
    }
    setDirty(true)
    pending.current = { docId: openId, content: next }
    if (timer.current) {
      clearTimeout(timer.current)
    }
    timer.current = setTimeout(() => void flush(), SAVE_AFTER_MS)
  }

  const compile = useCallback(async () => {
    setError('')
    setCompiling(true)
    try {
      // Whatever is on screen is what should be compiled, so anything not
      // written back yet goes first.
      await flush()
      setResult(await compileProject(projectId, {}))
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setCompiling(false)
    }
  }, [flush, projectId])

  // Ctrl-S writes back, Ctrl-Enter compiles. Both are what the keys already
  // mean elsewhere, and the browser's own meaning for Ctrl-S is not useful on
  // a page like this.
  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      const meta = event.ctrlKey || event.metaKey
      if (!meta) {
        return
      }
      if (event.key === 's') {
        event.preventDefault()
        void flush()
      }
      if (event.key === 'Enter') {
        event.preventDefault()
        void compile()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [compile, flush])

  // Leaving with unwritten text loses it, so say so. The browser decides the
  // wording; all a page can do is ask.
  useEffect(() => {
    function onLeave(event: BeforeUnloadEvent) {
      if (pending.current) {
        event.preventDefault()
        event.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', onLeave)
    return () => window.removeEventListener('beforeunload', onLeave)
  }, [])

  async function confirmPrompt() {
    if (!prompt) {
      return
    }
    setWorking(true)
    setError('')
    try {
      if (prompt.kind === 'new-doc') {
        const created = await createDocument(projectId, {
          name: withTeX(prompt.value),
          folderId: prompt.folderId,
        })
        setFiles(previous => [...previous, created])
        setPrompt(null)
        await open(created)
        return
      }
      if (prompt.kind === 'new-folder') {
        const created = await createFolder(projectId, {
          name: prompt.value.trim(),
          folderId: prompt.folderId,
        })
        setFiles(previous => [...previous, created])
      }
      if (prompt.kind === 'rename') {
        const name = prompt.value.trim()
        await renameEntry(projectId, prompt.entry.id, name)
        setFiles(previous => renamed(previous, prompt.entry, name))
      }
      if (prompt.kind === 'delete') {
        await deleteEntry(projectId, prompt.entry.id)
        setFiles(previous => removed(previous, prompt.entry))
        if (prompt.entry.id === openId) {
          pending.current = null
          setOpenId(null)
          setContent('')
        }
      }
      setPrompt(null)
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setWorking(false)
    }
  }

  async function chooseRoot(entry: FileEntry) {
    setError('')
    try {
      await setRootDoc(projectId, entry.id)
      setRootDocIdState(entry.id)
    } catch (problem) {
      setError(messageFor(problem))
    }
  }

  const openEntry = useMemo(
    () => files.find(file => file.id === openId) ?? null,
    [files, openId]
  )

  return (
    <div className="flex h-screen flex-col overflow-hidden">
      <header className="flex h-12 shrink-0 items-center gap-3 border-b border-divider px-3">
        <Link href="/projects" className="text-sm text-default-500 hover:text-foreground">
          ← Projects
        </Link>
        <span className="truncate font-medium">{view.project.name}</span>

        <span className="ml-2 text-xs text-default-400">
          {!writable
            ? 'Read only'
            : saving
              ? 'Saving…'
              : dirty
                ? 'Unsaved'
                : openEntry
                  ? 'Saved'
                  : ''}
        </span>

        <div className="ml-auto flex items-center gap-2">
          {git ? (
            <Button size="sm" variant="light" onPress={() => setShowClone(true)}>
              Git
            </Button>
          ) : null}
          <Button size="sm" variant="flat" onPress={() => setShowPdf(value => !value)}>
            {showPdf ? 'Hide PDF' : 'Show PDF'}
          </Button>
          <Button size="sm" color="primary" isLoading={compiling} onPress={() => void compile()}>
            Compile
          </Button>
          <span className="hidden text-xs text-default-400 sm:inline">
            {displayName(user)}
          </span>
        </div>
      </header>

      {error ? (
        <div className="flex items-center justify-between gap-3 border-b border-divider bg-danger-50 px-3 py-1.5 text-sm text-danger">
          <span className="truncate">{error}</span>
          <button type="button" className="shrink-0 text-xs underline" onClick={() => setError('')}>
            Dismiss
          </button>
        </div>
      ) : null}

      <div className="flex min-h-0 flex-1">
        <aside className="w-60 shrink-0 border-r border-divider">
          <FileTree
            files={files}
            openId={openId}
            rootDocId={rootDocId}
            canWrite={writable}
            onOpen={entry => void open(entry)}
            onRename={entry => setPrompt({ kind: 'rename', entry, value: entry.name })}
            onDelete={entry => setPrompt({ kind: 'delete', entry })}
            onSetRoot={entry => void chooseRoot(entry)}
            onCreate={(folderId, kind) =>
              setPrompt({
                kind: kind === 'doc' ? 'new-doc' : 'new-folder',
                folderId,
                value: '',
              })
            }
          />
        </aside>

        <main className="min-w-0 flex-1 border-r border-divider">
          {openEntry ? (
            <CodePane
              value={content}
              onChange={edit}
              readOnly={!writable}
              busy={loadingDoc}
            />
          ) : (
            <div className="flex h-full items-center justify-center p-6 text-center text-sm text-default-400">
              {files.some(file => file.kind === 'doc')
                ? 'Choose a file to edit.'
                : 'This project has no files yet.'}
            </div>
          )}
        </main>

        {showPdf ? (
          <section className="min-w-0 flex-1">
            <PdfPane result={result} compiling={compiling} error="" />
          </section>
        ) : null}
      </div>

      <Modal isOpen={showClone} onClose={() => setShowClone(false)} size="lg">
        <ModalContent>
          <ModalHeader className="text-base">Clone this project</ModalHeader>
          <ModalBody className="gap-3 pb-6">
            <Snippet size="sm" symbol="" variant="bordered" className="w-full">
              {`git clone ${cloneURL(projectId)}`}
            </Snippet>
            <p className="text-small text-default-500">
              Sign in as <code>git</code>, with a token from{' '}
              <Link href="/account" className="underline">
                your account page
              </Link>{' '}
              as the password.
            </p>
          </ModalBody>
        </ModalContent>
      </Modal>

      <Modal isOpen={prompt !== null} onClose={() => setPrompt(null)} size="sm">
        <ModalContent>
          <ModalHeader className="text-base">{titleFor(prompt)}</ModalHeader>
          <ModalBody>
            {prompt && prompt.kind === 'delete' ? (
              <p className="text-sm">
                Delete <span className="font-medium">{prompt.entry.name}</span>?
                {prompt.entry.kind === 'folder'
                  ? ' Everything inside it goes too.'
                  : ''}
              </p>
            ) : prompt ? (
              <Input
                autoFocus
                size="sm"
                label="Name"
                value={prompt.value}
                onValueChange={value => setPrompt({ ...prompt, value })}
                onKeyDown={event => {
                  if (event.key === 'Enter') {
                    void confirmPrompt()
                  }
                }}
              />
            ) : null}
          </ModalBody>
          <ModalFooter>
            <Button size="sm" variant="light" onPress={() => setPrompt(null)}>
              Cancel
            </Button>
            <Button
              size="sm"
              color={prompt?.kind === 'delete' ? 'danger' : 'primary'}
              isLoading={working}
              onPress={() => void confirmPrompt()}
            >
              {prompt?.kind === 'delete' ? 'Delete' : 'Save'}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {loadingDoc && !openEntry ? (
        <div className="pointer-events-none fixed inset-0 flex items-center justify-center">
          <Spinner />
        </div>
      ) : null}
    </div>
  )
}

/**
 * Where git finds this project.
 *
 * Built from the address in the browser rather than from a setting, so it is
 * the address this person actually reached the site at -- which is the one
 * that will work when they paste it into a terminal.
 */
function cloneURL(projectId: string): string {
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  return `${origin}/git/${projectId}`
}

/** A LaTeX file with no extension almost always meant to have one. */
function withTeX(name: string): string {
  const trimmed = name.trim()
  return trimmed.includes('.') ? trimmed : `${trimmed}.tex`
}

function titleFor(prompt: Prompt | null): string {
  switch (prompt?.kind) {
    case 'new-doc':
      return 'New file'
    case 'new-folder':
      return 'New folder'
    case 'rename':
      return 'Rename'
    case 'delete':
      return 'Delete'
    default:
      return ''
  }
}

/**
 * The tree after a rename.
 *
 * A folder's name is part of the path of everything inside it, so renaming one
 * moves all of them. Doing it here keeps the tree right without asking the
 * server for it again.
 */
function renamed(files: FileEntry[], entry: FileEntry, name: string): FileEntry[] {
  const slash = entry.path.lastIndexOf('/')
  const nextPath = slash < 0 ? name : `${entry.path.slice(0, slash)}/${name}`
  const inside = `${entry.path}/`
  return files.map(file => {
    if (file.id === entry.id) {
      return { ...file, name, path: nextPath }
    }
    if (file.path.startsWith(inside)) {
      return { ...file, path: nextPath + file.path.slice(entry.path.length) }
    }
    return file
  })
}

/** The tree after a delete, which takes a folder's contents with it. */
function removed(files: FileEntry[], entry: FileEntry): FileEntry[] {
  const inside = `${entry.path}/`
  return files.filter(file => file.id !== entry.id && !file.path.startsWith(inside))
}
