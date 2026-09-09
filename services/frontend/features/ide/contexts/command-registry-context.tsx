'use client'

/**
 * The commands the editor offers, from ide-react/context/command-registry-context.
 *
 * A menu does not know how to insert a figure; the source editor does, and
 * it registers a command for it while it is on screen. The menu bar then
 * reads the registry and shows what is there. That is what lets the same
 * File menu list "New file" only while there is a file tree to make one in,
 * and "Download PDF" only after a compile.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type DependencyList,
  type ReactNode,
} from 'react'
import { isMac } from '@/lib/os'

type CommandInvocationContext = {
  location?: string
}

export type Command = {
  label: string
  /** The label in a menu, when it differs from the command's own. */
  menuLabel?: string
  id: string
  handler?: (context: CommandInvocationContext) => void
  href?: string
  disabled?: boolean
  leadingIcon?: ReactNode
}

export type Shortcut = { key: string }

export type Shortcuts = Record<string, Shortcut[]>

type CommandRegistry = {
  registry: Map<string, Command>
  register: (...elements: Command[]) => void
  unregister: (...ids: string[]) => void
  shortcuts: Shortcuts
}

const CommandRegistryContext = createContext<CommandRegistry | undefined>(undefined)

export function CommandRegistryProvider({ children }: { children: ReactNode }) {
  const [registry, setRegistry] = useState(new Map<string, Command>())

  const register = useCallback((...elements: Command[]) => {
    setRegistry(
      previous => new Map([...previous, ...elements.map(element => [element.id, element] as const)])
    )
  }, [])

  const unregister = useCallback((...ids: string[]) => {
    setRegistry(previous => new Map([...previous].filter(([key]) => !ids.includes(key))))
  }, [])

  const shortcuts: Shortcuts = useMemo(
    () => ({
      cut: [{ key: 'Mod-x' }],
      copy: [{ key: 'Mod-c' }],
      paste: [{ key: 'Mod-v' }],
      'paste-special': [{ key: 'Mod-Shift-V' }],
      'toggle-track-changes': [{ key: 'Mod-Shift-A' }],
      undo: [{ key: 'Mod-z' }],
      redo: [{ key: 'Mod-y' }, { key: 'Mod-Shift-Z' }],
      find: [{ key: 'Mod-f' }],
      'select-all': [{ key: 'Mod-a' }],
      'insert-comment': [{ key: 'Mod-Shift-C' }],
      'format-bold': [{ key: 'Mod-b' }],
      'format-italics': [{ key: 'Mod-i' }],
    }),
    []
  )

  const value = useMemo(() => ({ registry, register, unregister, shortcuts }), [registry, register, unregister, shortcuts])

  return <CommandRegistryContext.Provider value={value}>{children}</CommandRegistryContext.Provider>
}

export function useCommandRegistry(): CommandRegistry {
  const context = useContext(CommandRegistryContext)
  if (!context) {
    throw new Error('useCommandRegistry must be used within a CommandRegistryProvider')
  }
  return context
}

/** Registers commands for as long as the component is mounted. */
export function useCommandProvider(generateElements: () => Command[] | undefined, dependencies: DependencyList) {
  const { register, unregister } = useCommandRegistry()
  useEffect(() => {
    const elements = generateElements()
    if (!elements) {
      return
    }
    register(...elements)
    return () => {
      unregister(...elements.map(element => element.id))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, dependencies)
}

function parseShortcut(shortcut: Shortcut) {
  let alt = false
  let ctrl = false
  let shift = false
  let meta = false
  let character: string | null = null
  const shortcutString = shortcut.key ?? ''
  const keys = shortcutString.split(/-(?!$)/) ?? []

  for (let i = 0; i < keys.length; i++) {
    const isLast = i === keys.length - 1
    const key = keys[i]
    if (!key) {
      throw new Error('Empty key in shortcut: ' + shortcutString)
    }
    if (key === 'Alt' || (!isLast && key === 'a')) {
      alt = true
    } else if (key === 'Ctrl' || key === 'Control' || (!isLast && key === 'c')) {
      ctrl = true
    } else if (key === 'Shift' || (!isLast && key === 's')) {
      shift = true
    } else if (key === 'Meta' || key === 'Cmd' || (!isLast && key === 'm')) {
      meta = true
    } else if (key === 'Mod') {
      if (isMac) {
        meta = true
      } else {
        ctrl = true
      }
    } else {
      if (key === 'Space') {
        character = ' '
      }
      if (!isLast) {
        throw new Error('Character key must be last in shortcut: ' + shortcutString)
      }
      if (key.length !== 1) {
        throw new Error(`Invalid key '${key}' in shortcut: ${shortcutString}`)
      }
      if (character) {
        throw new Error('Multiple characters in shortcut: ' + shortcutString)
      }
      character = key
    }
  }
  if (!character) {
    throw new Error('No character in shortcut: ' + shortcutString)
  }

  return { alt, ctrl, shift, meta, character }
}

export const formatShortcut = (shortcut: Shortcut): string => {
  const { alt, ctrl, shift, meta, character } = parseShortcut(shortcut)

  if (isMac) {
    return [ctrl ? '⌃' : '', alt ? '⌥' : '', shift ? '⇧' : '', meta ? '⌘' : '', character.toUpperCase()].join('')
  }

  return [ctrl ? 'Ctrl' : '', shift ? 'Shift' : '', meta ? 'Meta' : '', alt ? 'Alt' : '', character.toUpperCase()].join(' ')
}
