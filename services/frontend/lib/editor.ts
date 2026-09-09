/** What the API says about one project's contents, and about compiling it. */

import { api } from './api'
import type { Access } from './projects'

/** One thing in a project's file tree. */
export type FileEntry = {
  id: string
  name: string
  path: string
  kind: 'folder' | 'doc' | 'file'
  parentId?: string
  hash?: string
  size?: number
}

export type Project = {
  id: string
  name: string
  ownerId: string
  rootDocId?: string
  compiler?: string
  imageName?: string
  spellCheckLanguage?: string
  lastUpdated?: string
}

export type ProjectView = {
  project: Project
  access: Access
  files: FileEntry[]
}

export function getProject(
  id: string,
  headers?: Record<string, string>
): Promise<ProjectView> {
  return api<ProjectView>(`/api/projects/${id}`, { headers })
}

/** A document, as the editor holds it. */
export type Doc = {
  id: string
  name: string
  path: string
  lines: string[]
  version: number
}

export async function getDocument(
  projectId: string,
  docId: string,
  signal?: AbortSignal
): Promise<Doc> {
  return api<Doc>(`/api/projects/${projectId}/documents/${docId}`, { signal })
}

export function saveDocument(
  projectId: string,
  docId: string,
  content: string
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/documents/${docId}`, {
    method: 'POST',
    body: { content },
  })
}

/** One file a compile produced. */
export type OutputFile = {
  path: string
  url: string
  type?: string
  size?: number
  build?: string
}

export type CompileResult = {
  status: string
  error?: string
  outputFiles: OutputFile[]
  stats?: Record<string, unknown>
  timings?: Record<string, unknown>
}

export type CompileOptions = {
  rootDocId?: string
  draft?: boolean
  stopOnFirstError?: boolean
  incremental?: boolean
}

export function compileProject(
  projectId: string,
  options: CompileOptions = {},
  signal?: AbortSignal
): Promise<CompileResult> {
  return api<CompileResult>(`/api/projects/${projectId}/compile`, {
    method: 'POST',
    body: options,
    signal,
  })
}

export function stopCompile(projectId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/compile/stop`, { method: 'POST' })
}

/** The PDF among what a compile produced, if it made one. */
export function pdfIn(result: CompileResult | null): OutputFile | undefined {
  return result?.outputFiles?.find(file => file.path === 'output.pdf')
}

/** The log, which is what to read when there is no PDF. */
export function logIn(result: CompileResult | null): OutputFile | undefined {
  return result?.outputFiles?.find(file => file.path === 'output.log')
}

/**
 * Whether a path is text this editor can open.
 *
 * Everything else in a project is a binary file: an image, a PDF, a font. The
 * tree still shows them, but opening one in a text editor would show a
 * screenful of nonsense, so it does not offer to.
 */
export function isEditable(entry: FileEntry): boolean {
  return entry.kind === 'doc'
}

/** Making and unmaking the things in a project. */

export function createDocument(
  projectId: string,
  input: { name: string; folderId?: string; content?: string }
): Promise<FileEntry> {
  return api<{ file: FileEntry }>(`/api/projects/${projectId}/documents`, {
    method: 'POST',
    body: input,
  }).then(answer => answer.file)
}

export function createFolder(
  projectId: string,
  input: { name: string; folderId?: string }
): Promise<FileEntry> {
  return api<{ file: FileEntry }>(`/api/projects/${projectId}/folders`, {
    method: 'POST',
    body: input,
  }).then(answer => answer.file)
}

export function renameEntry(
  projectId: string,
  entryId: string,
  name: string
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/entries/${entryId}/rename`, {
    method: 'POST',
    body: { name },
  })
}

export function deleteEntry(projectId: string, entryId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/entries/${entryId}`, {
    method: 'DELETE',
  })
}

export function setRootDoc(projectId: string, docId: string): Promise<void> {
  return api<void>(`/api/projects/${projectId}/root-doc`, {
    method: 'POST',
    body: { docId },
  })
}

/**
 * Moves an entry into a folder, or to the root when no folder is given.
 *
 * Separate from a rename because it is a different question for the server:
 * a rename cannot collide with anything outside its own folder, and a move
 * can.
 */
export function moveEntry(
  projectId: string,
  entryId: string,
  folderId?: string
): Promise<void> {
  return api<void>(`/api/projects/${projectId}/entries/${entryId}/move`, {
    method: 'POST',
    body: { folderId: folderId ?? null },
  })
}

/**
 * Uploads a file.
 *
 * Not through `api`: that sends JSON, and this is the one request in the
 * client that is a multipart body. The same rules still apply -- same origin,
 * cookies, one error shape.
 */
export async function uploadFile(
  projectId: string,
  file: File,
  folderId?: string
): Promise<FileEntry> {
  const form = new FormData()
  form.append('file', file)
  form.append('name', file.name)
  if (folderId) {
    form.append('folderId', folderId)
  }

  const response = await fetch(`/api/projects/${projectId}/uploads`, {
    method: 'POST',
    credentials: 'include',
    body: form,
  })

  const text = await response.text()
  const parsed = text ? (JSON.parse(text) as Record<string, unknown>) : {}
  if (!response.ok) {
    const failure = parsed.error as { message?: string } | undefined
    throw new Error(failure?.message ?? 'That file could not be uploaded.')
  }
  return (parsed as { file: FileEntry }).file
}
