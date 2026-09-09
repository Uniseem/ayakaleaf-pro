'use client'

/**
 * The bar across the top of the editor.
 *
 * 40px, white, one hairline underneath -- measured from the original, where
 * this bar is deliberately shorter than the 68px one on every other page: the
 * editor is a workspace and the chrome gets out of the way.
 *
 * Three parts: the way out and the menus on the left, the project's name in
 * the middle, and what you do to the project on the right.
 */

import { Tooltip } from '@heroui/react'
import Link from 'next/link'
import { useState } from 'react'
import { useProject } from '@/features/ide/contexts/project-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useConnection } from '@/features/ide/contexts/connection-context'
import { ShareModal } from '@/features/sharing/share-modal'
import { WordCountModal } from '@/features/word-count/word-count-modal'
import { MenuBar } from './menu-bar'

export function Toolbar({ userName }: { userName: string }) {
  const project = useProject()
  const layout = useLayout()

  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState(project.project.name)
  const [sharing, setSharing] = useState(false)
  const [counting, setCounting] = useState(false)

  return (
    <header className="relative flex h-10 shrink-0 items-center gap-1 border-b border-[var(--border-divider)] bg-[var(--bg-light-primary)] px-2">
      <Tooltip content="All projects" delay={400} closeDelay={0}>
        <Link
          href="/projects"
          aria-label="Back to all projects"
          className="flex h-7 w-7 shrink-0 items-center justify-center rounded-[4px] text-[var(--content-primary)] hover:bg-[var(--hover-interaction)]"
        >
          <LeafIcon />
        </Link>
      </Tooltip>

      <MenuBar
        onShare={() => setSharing(true)}
        onWordCount={() => setCounting(true)}
        onNewFile={() => window.dispatchEvent(new CustomEvent('ide:new-file'))}
        onUpload={() => window.dispatchEvent(new CustomEvent('ide:upload'))}
      />

      {/* The name sits in the middle of the bar, not after the menus, so it
          stays put as the menus change width between locales. */}
      <div className="pointer-events-none absolute left-1/2 hidden -translate-x-1/2 sm:block">
        {renaming ? (
          <form
            className="pointer-events-auto"
            onSubmit={event => {
              event.preventDefault()
              const trimmed = name.trim()
              if (trimmed && trimmed !== project.project.name) {
                void project.setName(trimmed)
              }
              setRenaming(false)
            }}
          >
            <input
              autoFocus
              value={name}
              onChange={event => setName(event.target.value)}
              onBlur={() => setRenaming(false)}
              aria-label="Project name"
              className="h-7 w-56 rounded-[4px] border border-[var(--border-active)] px-2 text-[14px] font-bold leading-5 focus:outline-none"
            />
          </form>
        ) : (
          <button
            type="button"
            className="pointer-events-auto flex h-7 max-w-[320px] items-center gap-1 truncate rounded-[4px] px-2 text-[14px] font-bold leading-5 text-[var(--content-primary)] hover:bg-[var(--hover-interaction)]"
            onClick={() => {
              if (project.canWrite) {
                setName(project.project.name)
                setRenaming(true)
              }
            }}
            title={project.canWrite ? 'Rename' : project.project.name}
          >
            {project.project.name}
          </button>
        )}
      </div>

      <div className="flex-1" />

      <ConnectionBadge />

      <Tooltip content="History" delay={400} closeDelay={0}>
        <button
          type="button"
          aria-label="Project history"
          aria-pressed={layout.view === 'history'}
          onClick={() =>
            layout.view === 'history' ? layout.restoreView() : layout.setView('history')
          }
          className={`flex h-7 w-7 items-center justify-center rounded-[4px] ${
            layout.view === 'history'
              ? 'bg-[var(--bg-accent-03)] text-[var(--link-web)]'
              : 'text-[var(--content-primary)] hover:bg-[var(--hover-interaction)]'
          }`}
        >
          <HistoryIcon />
        </button>
      </Tooltip>

      <OnlineUsers />

      <button
        type="button"
        onClick={() => setSharing(true)}
        className="ml-1 inline-flex h-6 items-center rounded-full bg-[var(--bg-accent-01)] px-3 text-[14px] font-semibold leading-5 text-white hover:bg-[var(--bg-accent-02)]"
      >
        Share
      </button>

      <ShareModal isOpen={sharing} onClose={() => setSharing(false)} />
      <WordCountModal isOpen={counting} onClose={() => setCounting(false)} />
      <span className="sr-only">{userName}</span>
    </header>
  )
}

/** Faces of everybody else with the project open. */
function OnlineUsers() {
  const { others } = useConnection()
  if (others.length === 0) {
    return null
  }
  return (
    <Tooltip content={others.map(each => each.name).join(', ')} delay={200}>
      <div className="flex items-center -space-x-1.5 pr-1">
        {others.slice(0, 3).map(person => (
          <span
            key={person.clientId}
            className="flex h-6 w-6 items-center justify-center rounded-full border-2 border-[var(--bg-light-primary)] bg-[var(--bg-info-01)] text-[10px] font-semibold text-white"
          >
            {person.name.slice(0, 1).toUpperCase()}
          </span>
        ))}
        {others.length > 3 ? (
          <span className="flex h-6 w-6 items-center justify-center rounded-full border-2 border-[var(--bg-light-primary)] bg-[var(--bg-light-tertiary)] text-[10px] font-semibold text-[var(--content-secondary)]">
            +{others.length - 3}
          </span>
        ) : null}
      </div>
    </Tooltip>
  )
}

/**
 * Whether this editor is actually connected.
 *
 * Shown rather than hidden on purpose. An editor that looks the same online
 * and offline is one that quietly stops saving, and the person finds out when
 * they close the tab.
 */
function ConnectionBadge() {
  const { state } = useConnection()
  const editor = useEditor()

  if (state === 'connected') {
    return editor.unsaved ? (
      <span className="px-2 text-[12px] leading-4 text-[var(--content-secondary)]">
        Saving…
      </span>
    ) : null
  }

  const label =
    state === 'failed'
      ? 'Not connected'
      : state === 'reconnecting'
        ? 'Reconnecting…'
        : state === 'disconnected'
          ? 'Disconnected'
          : 'Connecting…'

  return (
    <Tooltip
      content={
        state === 'failed'
          ? 'Edits cannot be saved. Refresh the page to try again.'
          : 'Edits cannot be saved until this reconnects.'
      }
      delay={200}
    >
      <span
        className={`flex items-center gap-1 px-2 text-[12px] leading-4 ${
          state === 'failed'
            ? 'text-[var(--content-danger)]'
            : 'text-[var(--content-warning)]'
        }`}
      >
        <span
          className={`h-1.5 w-1.5 rounded-full ${
            state === 'failed'
              ? 'bg-[var(--bg-danger-01)]'
              : 'bg-[var(--bg-warning-01)]'
          }`}
        />
        {label}
      </span>
    </Tooltip>
  )
}

function LeafIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-5 w-5 text-[var(--bg-accent-01)]" fill="currentColor" aria-hidden>
      <path d="M13.4 2.2c.3 4.6-1 7.6-3.1 9.2-1.6 1.2-3.4 1.4-4.8 1L4 13.8a.7.7 0 0 1-1-1l1.4-1.4c-.5-1.5-.3-3.4 1-5C7 4.2 9.6 2.6 13.4 2.2Zm-2 2C8.9 4.9 7.2 6 6.3 7.2c-.8 1-1 2-.9 2.9l5-5a.7.7 0 0 1 1 1l-5 5c.9.1 2-.1 3-.9 1.3-1 2.3-2.9 2-6Z" />
    </svg>
  )
}

function HistoryIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden>
      <path d="M2.6 8a5.4 5.4 0 1 0 1.6-3.8" strokeLinecap="round" />
      <path d="M2.5 3v2.6h2.6" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M8 5.2V8l2 1.4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
