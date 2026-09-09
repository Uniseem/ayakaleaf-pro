'use client'

/**
 * What the whole project defines, for autocomplete: the labels it declares,
 * the packages it loads, the commands it defines, and the citation keys in
 * its bibliographies.
 *
 * The original had the server scan every document and hand the result over.
 * Here the documents are read once each and scanned in the browser, with the
 * open document read from the editor instead so its labels are current as
 * they are typed. A document is rescanned when the editor says a line that
 * matters changed.
 */

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { getDocument, type FileEntry } from '@/lib/editor'
import { useProject } from './project-context'
import { useEditor } from './editor-context'
import type { MetadataCommand } from '@/features/source-editor/extensions/language'

export type MetadataValue = {
  labels: Set<string>
  packageNames: Set<string>
  commands: MetadataCommand[]
  referenceKeys: Set<string>
}

type DocMetadata = {
  labels: string[]
  packages: string[]
  commands: MetadataCommand[]
  referenceKeys: string[]
}

const MetadataContext = createContext<MetadataValue | undefined>(undefined)

const EMPTY: DocMetadata = { labels: [], packages: [], commands: [], referenceKeys: [] }

const isTex = (name: string) => /\.(tex|ltx|rtex|rnw|latex)$/i.test(name)
const isBib = (name: string) => /\.bib$/i.test(name)

/** What one document contributes. */
export function scanDocument(name: string, text: string): DocMetadata {
  if (isBib(name)) {
    const referenceKeys: string[] = []
    for (const match of text.matchAll(/^\s*@(?!comment|preamble|string)\w+\s*[{(]\s*([^,\s]+)\s*,/gim)) {
      referenceKeys.push(match[1]!)
    }
    return { ...EMPTY, referenceKeys }
  }

  if (!isTex(name)) {
    return EMPTY
  }

  const labels: string[] = []
  const packages: string[] = []
  const commands: MetadataCommand[] = []

  for (const rawLine of text.split('\n')) {
    // strip comments
    const line = rawLine.replace(/(^|[^\\])%.*$/, '$1')

    for (const match of line.matchAll(/\\label\{([^}]+)\}/g)) {
      labels.push(match[1]!)
    }
    for (const match of line.matchAll(/\\(?:usepackage|RequirePackage)(?:\[[^\]]*\])?\{([^}]+)\}/g)) {
      for (const name of match[1]!.split(',')) {
        const trimmed = name.trim()
        if (trimmed) {
          packages.push(trimmed)
        }
      }
    }
    for (const match of line.matchAll(/\\(?:re)?newcommand\*?\s*\{?\\(\w+)\}?\s*(?:\[(\d+)\])?/g)) {
      const name = match[1]!
      const argCount = match[2] ? parseInt(match[2], 10) : 0
      const caption = `\\${name}${'{}'.repeat(argCount)}`
      const snippet = `\\${name}${Array.from({ length: argCount }, (_, i) => `{$${i + 1}}`).join('')}`
      commands.push({ caption, snippet, meta: 'cmd', score: 0 })
    }
  }

  return { labels, packages, commands, referenceKeys: [] }
}

export function MetadataProvider({ children }: { children: ReactNode }) {
  const { projectId, files } = useProject()
  const editor = useEditor()

  // What each document contributed, by id, from the last time it was read.
  const [scanned, setScanned] = useState<Map<string, DocMetadata>>(() => new Map())
  const fetched = useRef<Set<string>>(new Set())

  const relevant = useMemo(
    () => files.filter((entry): entry is FileEntry => entry.kind === 'doc' && (isTex(entry.name) || isBib(entry.name))),
    [files]
  )

  // Read every relevant document once.
  useEffect(() => {
    let live = true
    const missing = relevant.filter(entry => !fetched.current.has(entry.id))
    for (const entry of missing) {
      fetched.current.add(entry.id)
    }
    const read = async () => {
      for (const entry of missing) {
        try {
          const doc = await getDocument(projectId, entry.id)
          if (!live) {
            return
          }
          const metadata = scanDocument(entry.name, doc.lines.join('\n'))
          setScanned(previous => new Map(previous).set(entry.id, metadata))
        } catch {
          // a document that could not be read contributes nothing
        }
      }
    }
    void read()
    return () => {
      live = false
    }
  }, [projectId, relevant])

  // Forget documents that were deleted.
  useEffect(() => {
    const ids = new Set(relevant.map(entry => entry.id))
    setScanned(previous => {
      let changed = false
      const next = new Map(previous)
      for (const id of next.keys()) {
        if (!ids.has(id)) {
          next.delete(id)
          changed = true
        }
      }
      return changed ? next : previous
    })
  }, [relevant])

  // The open document is read from the editor, and rescanned when the editor
  // reports a change to a line that matters.
  const currentId = editor.current?.id
  const currentName = editor.current?.name
  const currentText = editor.current?.content
  const latestText = useRef(currentText)
  latestText.current = currentText

  const rescanCurrent = useCallback(() => {
    if (currentId && currentName && latestText.current !== undefined) {
      const metadata = scanDocument(currentName, latestText.current)
      setScanned(previous => new Map(previous).set(currentId, metadata))
    }
  }, [currentId, currentName])

  useEffect(() => {
    rescanCurrent()
  }, [rescanCurrent, editor.revision])

  useEffect(() => {
    let timer: number | null = null
    const listener = () => {
      if (timer !== null) {
        window.clearTimeout(timer)
      }
      timer = window.setTimeout(rescanCurrent, 1000)
    }
    window.addEventListener('editor:metadata-outdated', listener)
    return () => {
      window.removeEventListener('editor:metadata-outdated', listener)
      if (timer !== null) {
        window.clearTimeout(timer)
      }
    }
  }, [rescanCurrent])

  const value = useMemo<MetadataValue>(() => {
    const labels = new Set<string>()
    const packageNames = new Set<string>()
    const referenceKeys = new Set<string>()
    const commands: MetadataCommand[] = []
    const seenCommands = new Set<string>()
    for (const metadata of scanned.values()) {
      for (const label of metadata.labels) labels.add(label)
      for (const name of metadata.packages) packageNames.add(name)
      for (const key of metadata.referenceKeys) referenceKeys.add(key)
      for (const command of metadata.commands) {
        if (!seenCommands.has(command.caption)) {
          seenCommands.add(command.caption)
          commands.push(command)
        }
      }
    }
    return { labels, packageNames, commands, referenceKeys }
  }, [scanned])

  return <MetadataContext.Provider value={value}>{children}</MetadataContext.Provider>
}

export function useMetadataContext(): MetadataValue {
  const context = useContext(MetadataContext)
  if (!context) {
    throw new Error('useMetadataContext is only available inside MetadataProvider')
  }
  return context
}
