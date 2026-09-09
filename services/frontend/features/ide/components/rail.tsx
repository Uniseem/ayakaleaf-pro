'use client'

/**
 * The strip of panels down the left.
 *
 * One button per panel, and one panel showing at a time. The tab bar stays
 * whether or not a panel is open, so closing one does not move everything
 * else, and clicking the open panel's own button closes it -- which is how
 * every editor with this shape behaves and is what people try first.
 */

import { Tooltip } from '@heroui/react'
import type { ReactNode } from 'react'
import { useLayout, type RailTab } from '@/features/ide/contexts/layout-context'
import { useSettings } from '@/features/ide/contexts/settings-context'
import { FileTree } from '@/features/file-tree/file-tree'
import { Outline } from './outline'
import { ProjectSearch } from './project-search'
import { ChatPane } from '@/features/chat/chat-pane'
import { ReviewPanel } from '@/features/review-panel/review-panel'

type RailEntry = {
  key: RailTab
  title: string
  icon: ReactNode
  panel: ReactNode
  /** A number shown on the button, for a panel with something waiting. */
  badge?: number
}

/**
 * The panels, and which button opens each.
 *
 * A hook rather than a constant because two components need the same list --
 * the strip of buttons and the panel beside it -- and they are separate so
 * that the strip can sit outside the resizable group and keep its width.
 */
function useRailEntries(): RailEntry[] {
  const settings = useSettings()

  return [
    {
      key: 'file-tree',
      title: 'Files',
      icon: <FilesIcon />,
      panel: (
        <div className="flex h-full flex-col">
          <div className={settings.showOutline ? 'min-h-0 flex-1' : 'h-full'}>
            <FileTree />
          </div>
          {settings.showOutline ? (
            <div className="flex h-2/5 min-h-[120px] flex-col border-t border-[var(--border-divider)]">
              <header className="flex h-7 shrink-0 items-center px-3 text-[12px] font-semibold uppercase leading-4 tracking-wide text-[var(--content-secondary)]">
                File outline
              </header>
              <div className="min-h-0 flex-1">
                <Outline />
              </div>
            </div>
          ) : null}
        </div>
      ),
    },
    {
      key: 'full-project-search',
      title: 'Search',
      icon: <SearchIcon />,
      panel: <ProjectSearch />,
    },
    {
      key: 'review-panel',
      title: 'Review',
      icon: <ReviewIcon />,
      panel: <ReviewPanel />,

    },
    {
      key: 'chat',
      title: 'Chat',
      icon: <ChatIcon />,
      panel: <ChatPane />,
    },
  ]
}

/** The strip of buttons. Always visible, whether a panel is open or not. */
export function RailTabs() {
  const layout = useLayout()
  const entries = useRailEntries()

  return (
      <nav
        className="flex w-10 shrink-0 flex-col items-center gap-1 border-r border-[var(--border-divider)] bg-[var(--bg-light-primary)] py-2"
        aria-label="Panels"
      >
        {entries.map(entry => (
          <Tooltip
            key={entry.key}
            content={entry.title}
            placement="right"
            delay={400}
            closeDelay={0}
          >
            <button
              type="button"
              aria-label={entry.title}
              aria-pressed={layout.railTab === entry.key}
              onClick={() => layout.toggleRailTab(entry.key)}
              className={[
                'relative flex h-8 w-8 items-center justify-center rounded-[4px]',
                layout.railTab === entry.key
                  ? 'bg-[var(--bg-accent-03)] text-[var(--link-web-hover)]'
                  : 'text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)] hover:text-[var(--content-primary)]',
              ].join(' ')}
            >
              {entry.icon}
              {entry.badge ? (
                <span className="absolute -right-0.5 -top-0.5 flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-[var(--bg-danger-01)] px-1 text-[9px] font-medium text-white">
                  {entry.badge > 9 ? '9+' : entry.badge}
                </span>
              ) : null}
            </button>
          </Tooltip>
        ))}

        <div className="flex-1" />

        <Tooltip content="Keyboard shortcuts" placement="right" delay={400} closeDelay={0}>
          <button
            type="button"
            aria-label="Keyboard shortcuts"
            onClick={() => window.dispatchEvent(new CustomEvent('ide:shortcuts'))}
            className="flex h-8 w-8 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)] hover:text-[var(--content-primary)]"
          >
            <HelpIcon />
          </button>
        </Tooltip>

        <Tooltip content="Settings" placement="right" delay={400} closeDelay={0}>
          <a
            href="/account"
            aria-label="Settings"
            className="flex h-8 w-8 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)] hover:text-[var(--content-primary)]"
          >
            <SettingsIcon />
          </a>
        </Tooltip>
      </nav>
  )
}

function HelpIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden>
      <circle cx="8" cy="8" r="5.6" />
      <path d="M6.4 6.3a1.7 1.7 0 1 1 2.2 1.9c-.4.2-.6.5-.6.9v.3" strokeLinecap="round" />
      <circle cx="8" cy="11.4" r="0.7" fill="currentColor" stroke="none" />
    </svg>
  )
}

function SettingsIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden>
      <circle cx="8" cy="8" r="2.2" />
      <path d="M8 1.8v1.6M8 12.6v1.6M2.6 8H1M15 8h-1.6M4.2 4.2 3 3M13 13l-1.2-1.2M11.8 4.2 13 3M3 13l1.2-1.2" strokeLinecap="round" />
    </svg>
  )
}

/** Whichever panel the strip has open. */
export function RailPanel() {
  const layout = useLayout()
  const entries = useRailEntries()
  const open = entries.find(entry => entry.key === layout.railTab)

  if (!open) {
    return null
  }
  return (
    <div className="flex h-full min-h-0 w-full flex-col overflow-hidden border-r border-[var(--border-divider)] bg-[var(--bg-light-primary)]">
      {open.panel}
    </div>
  )
}

function FilesIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="1.4">
      <path d="M4 2.5h5L12.5 6v7.5A1 1 0 0 1 11.5 14h-7a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1z" />
      <path d="M9 2.5V6h3.5" strokeLinejoin="round" />
    </svg>
  )
}

function SearchIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="1.4">
      <circle cx="7" cy="7" r="4.2" />
      <path d="m10.2 10.2 3.3 3.3" strokeLinecap="round" />
    </svg>
  )
}

function ReviewIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="1.4">
      <path d="M2.5 4A1.5 1.5 0 0 1 4 2.5h8A1.5 1.5 0 0 1 13.5 4v5A1.5 1.5 0 0 1 12 10.5H6.5L3.5 13V10.5A1.5 1.5 0 0 1 2.5 9z" strokeLinejoin="round" />
    </svg>
  )
}

function ChatIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4.5 w-4.5" fill="none" stroke="currentColor" strokeWidth="1.4">
      <path d="M8 2.5c3.3 0 5.5 1.9 5.5 4.4S11.3 11.3 8 11.3a7 7 0 0 1-1.6-.2L3.5 12.5l.7-2.2A4.6 4.6 0 0 1 2.5 6.9C2.5 4.4 4.7 2.5 8 2.5Z" strokeLinejoin="round" />
    </svg>
  )
}
