/**
 * Turning the flat list of entries into a tree.
 *
 * The API sends every entry with its full path and nothing about nesting,
 * which is the right thing to send: one representation crosses the wire and
 * how it is drawn stays this feature's business. The nesting is rebuilt from
 * the paths here.
 */

import type { FileEntry } from '@/lib/editor'

export type TreeNode = {
  entry: FileEntry
  children: TreeNode[]
  depth: number
}

/** Folders before files, then by name, the way a file manager sorts. */
function byKindThenName(a: TreeNode, b: TreeNode): number {
  const aFolder = a.entry.kind === 'folder'
  const bFolder = b.entry.kind === 'folder'
  if (aFolder !== bFolder) {
    return aFolder ? -1 : 1
  }
  return a.entry.name.localeCompare(b.entry.name, undefined, {
    numeric: true,
    sensitivity: 'base',
  })
}

export function buildTree(entries: FileEntry[]): TreeNode[] {
  const byPath = new Map<string, TreeNode>()
  const roots: TreeNode[] = []

  // A parent's path is a prefix of its children's, so sorting by path puts
  // every folder before what is inside it and one pass is enough.
  const sorted = [...entries].sort((a, b) => a.path.localeCompare(b.path))

  for (const entry of sorted) {
    const node: TreeNode = { entry, children: [], depth: 0 }
    byPath.set(entry.path, node)
    const slash = entry.path.lastIndexOf('/')
    const parent = slash < 0 ? undefined : byPath.get(entry.path.slice(0, slash))
    if (parent) {
      node.depth = parent.depth + 1
      parent.children.push(node)
    } else {
      roots.push(node)
    }
  }

  const order = (nodes: TreeNode[]) => {
    nodes.sort(byKindThenName)
    nodes.forEach(node => order(node.children))
  }
  order(roots)
  return roots
}

/** The tree flattened back to rows, skipping what is collapsed. */
export function visibleRows(
  nodes: TreeNode[],
  collapsed: ReadonlySet<string>
): TreeNode[] {
  const rows: TreeNode[] = []
  const walk = (list: TreeNode[]) => {
    for (const node of list) {
      rows.push(node)
      if (node.entry.kind === 'folder' && !collapsed.has(node.entry.path)) {
        walk(node.children)
      }
    }
  }
  walk(nodes)
  return rows
}

/** Whether moving an entry into a folder is a move that makes sense. */
export function canMoveInto(entry: FileEntry, folder: FileEntry | null): boolean {
  if (folder === null) {
    // To the root, which is fine unless it is already there.
    return entry.path.includes('/')
  }
  if (folder.kind !== 'folder' || folder.id === entry.id) {
    return false
  }
  // A folder cannot be moved inside itself or anything it contains.
  if (entry.kind === 'folder' && folder.path.startsWith(entry.path + '/')) {
    return false
  }
  // Already there.
  return folder.path !== entry.path.slice(0, entry.path.lastIndexOf('/'))
}

/** The names already taken in the folder something is about to go into. */
export function namesIn(entries: FileEntry[], folderPath: string): Set<string> {
  const prefix = folderPath ? folderPath + '/' : ''
  const names = new Set<string>()
  for (const entry of entries) {
    if (!entry.path.startsWith(prefix)) {
      continue
    }
    const rest = entry.path.slice(prefix.length)
    if (rest && !rest.includes('/')) {
      names.add(rest)
    }
  }
  return names
}

/** The extensions the editor will open as text. */
const TEXT_EXTENSIONS = new Set([
  'tex', 'bib', 'bbl', 'cls', 'sty', 'txt', 'md', 'markdown', 'rmd',
  'latex', 'ltx', 'bst', 'def', 'clo', 'ins', 'dtx', 'aux', 'toc',
  'lof', 'lot', 'gls', 'ist', 'nlo', 'py', 'r', 'json', 'yml', 'yaml',
  'csv', 'tsv', 'svg', 'asy', 'lua', 'sh',
])

export function extensionOf(name: string): string {
  const dot = name.lastIndexOf('.')
  return dot === -1 ? '' : name.slice(dot + 1).toLowerCase()
}

export function looksLikeText(name: string): boolean {
  return TEXT_EXTENSIONS.has(extensionOf(name))
}

/** A name that cannot be used, and why -- or null when it can. */
export function checkName(
  name: string,
  taken: ReadonlySet<string>
): string | null {
  const trimmed = name.trim()
  if (!trimmed) {
    return 'A name is required.'
  }
  if (trimmed.includes('/')) {
    return 'A name cannot contain a slash.'
  }
  if (trimmed === '.' || trimmed === '..') {
    return 'That name is reserved.'
  }
  if (trimmed.length > 150) {
    return 'That name is too long.'
  }
  // Nothing that would make a path a machine cannot store or serve. Written
  // as codes and a list rather than a regex, so that no escape in this
  // source has to survive being edited.
  const backslash = String.fromCharCode(92)
  const forbidden = new Set(['<', '>', ':', '"', '|', '?', '*', backslash])
  const unusable = [...trimmed].some(character => {
    const code = character.charCodeAt(0)
    return code < 32 || code === 127 || forbidden.has(character)
  })
  if (unusable) {
    return 'A name cannot contain that character.'
  }
  if (taken.has(trimmed)) {
    return 'There is already something here with that name.'
  }
  return null
}
