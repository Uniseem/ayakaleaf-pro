'use client'

/**
 * The bar across the top of the editor.
 *
 * Project identity on the left, what is being done to it in the middle, and
 * the person on the right. The compile button lives here rather than only in
 * the PDF pane so that it is reachable when the PDF pane is closed.
 */

import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownSection,
  DropdownTrigger,
  Input,
  Switch,
  Tooltip,
} from '@heroui/react'
import Link from 'next/link'
import { useState } from 'react'
import { useProject } from '@/features/ide/contexts/project-context'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useSettings } from '@/features/ide/contexts/settings-context'
import { useConnection } from '@/features/ide/contexts/connection-context'
import { ShareModal } from '@/features/sharing/share-modal'

export function Toolbar({ userName }: { userName: string }) {
  const project = useProject()
  const compile = useCompile()
  const layout = useLayout()
  const editor = useEditor()
  const settings = useSettings()

  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState(project.project.name)
  const [sharing, setSharing] = useState(false)

  return (
    <header className="flex h-11 shrink-0 items-center gap-2 border-b border-divider bg-background px-2">
      <Tooltip content="All projects" delay={400}>
        <Button
          as={Link}
          href="/projects"
          size="sm"
          variant="light"
          isIconOnly
          className="h-8 w-8 min-w-8"
          aria-label="Back to all projects"
        >
          <BackIcon />
        </Button>
      </Tooltip>

      {renaming ? (
        <form
          onSubmit={event => {
            event.preventDefault()
            const trimmed = name.trim()
            if (trimmed && trimmed !== project.project.name) {
              void project.setName(trimmed)
            }
            setRenaming(false)
          }}
        >
          <Input
            autoFocus
            size="sm"
            value={name}
            onValueChange={setName}
            onBlur={() => setRenaming(false)}
            className="w-56"
            aria-label="Project name"
          />
        </form>
      ) : (
        <button
          type="button"
          className="truncate rounded px-1.5 py-1 text-sm font-medium hover:bg-default-100"
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

      <ConnectionBadge />

      <div className="flex-1" />

      <Button
        size="sm"
        variant="flat"
        className="h-8"
        onPress={() => setSharing(true)}
      >
        Share
      </Button>

      <Button
        size="sm"
        color="primary"
        className="h-8"
        onPress={compile.compiling ? compile.stop : compile.startCompile}
      >
        {compile.compiling ? 'Stop' : 'Compile'}
      </Button>

      <Dropdown placement="bottom-end">
        <DropdownTrigger>
          <Button
            size="sm"
            variant="light"
            isIconOnly
            className="h-8 w-8 min-w-8"
            aria-label="Compile options"
          >
            <ChevronIcon />
          </Button>
        </DropdownTrigger>
        <DropdownMenu aria-label="Compile options" closeOnSelect={false}>
          <DropdownItem
            key="auto"
            endContent={
              <Switch
                size="sm"
                isSelected={compile.autoCompile}
                onValueChange={compile.setAutoCompile}
                aria-label="Compile automatically"
              />
            }
          >
            Compile automatically
          </DropdownItem>
          <DropdownItem
            key="draft"
            endContent={
              <Switch
                size="sm"
                isSelected={compile.draft}
                onValueChange={compile.setDraft}
                aria-label="Draft mode"
              />
            }
            description="Skips images, which is faster"
          >
            Draft mode
          </DropdownItem>
          <DropdownItem
            key="stop"
            endContent={
              <Switch
                size="sm"
                isSelected={compile.stopOnFirstError}
                onValueChange={compile.setStopOnFirstError}
                aria-label="Stop on first error"
              />
            }
          >
            Stop on first error
          </DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Tooltip content={layout.pdfLayout === 'sideBySide' ? 'One pane' : 'Side by side'} delay={400}>
        <Button
          size="sm"
          variant="light"
          isIconOnly
          className="h-8 w-8 min-w-8"
          aria-label="Change the layout"
          onPress={() =>
            layout.changeLayout(
              layout.pdfLayout === 'sideBySide' ? 'flat' : 'sideBySide'
            )
          }
        >
          <LayoutIcon />
        </Button>
      </Tooltip>

      <Dropdown placement="bottom-end">
        <DropdownTrigger>
          <Button
            size="sm"
            variant="light"
            isIconOnly
            className="h-8 w-8 min-w-8"
            aria-label="Menu"
          >
            <MenuIcon />
          </Button>
        </DropdownTrigger>
        <DropdownMenu aria-label="Menu" closeOnSelect={false}>
          <DropdownSection title={userName} showDivider>
            <DropdownItem
              key="theme"
              endContent={
                <Switch
                  size="sm"
                  isSelected={settings.overallTheme === 'dark'}
                  onValueChange={on => settings.set('overallTheme', on ? 'dark' : 'light')}
                  aria-label="Dark theme"
                />
              }
            >
              Dark theme
            </DropdownItem>
            <DropdownItem
              key="outline"
              endContent={
                <Switch
                  size="sm"
                  isSelected={settings.showOutline}
                  onValueChange={on => settings.set('showOutline', on)}
                  aria-label="Show the outline"
                />
              }
            >
              Show outline
            </DropdownItem>
            <DropdownItem
              key="wrap"
              endContent={
                <Switch
                  size="sm"
                  isSelected={settings.autoComplete}
                  onValueChange={on => settings.set('autoComplete', on)}
                  aria-label="Autocomplete"
                />
              }
            >
              Autocomplete
            </DropdownItem>
            <DropdownItem
              key="lint"
              endContent={
                <Switch
                  size="sm"
                  isSelected={settings.syntaxValidation}
                  onValueChange={on => settings.set('syntaxValidation', on)}
                  aria-label="Check syntax"
                />
              }
            >
              Check syntax
            </DropdownItem>
          </DropdownSection>
          <DropdownSection>
            <DropdownItem key="settings" href="/account" closeOnSelect>
              Account settings
            </DropdownItem>
            <DropdownItem key="projects" href="/projects" closeOnSelect>
              All projects
            </DropdownItem>
          </DropdownSection>
        </DropdownMenu>
      </Dropdown>

      <ShareModal isOpen={sharing} onClose={() => setSharing(false)} />
    </header>
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
  const { state, others } = useConnection()
  const editor = useEditor()

  if (state === 'connected') {
    return (
      <div className="flex items-center gap-2">
        {editor.unsaved ? (
          <span className="text-[11px] text-default-400">Saving…</span>
        ) : null}
        {others.length > 0 ? (
          <Tooltip content={others.map(each => each.name).join(', ')} delay={200}>
            <span className="flex items-center gap-1 text-[11px] text-default-500">
              <span className="h-1.5 w-1.5 rounded-full bg-success" />
              {others.length} other{others.length === 1 ? '' : 's'}
            </span>
          </Tooltip>
        ) : null}
      </div>
    )
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
        className={`flex items-center gap-1 text-[11px] ${
          state === 'failed' ? 'text-danger' : 'text-warning-600'
        }`}
      >
        <span
          className={`h-1.5 w-1.5 rounded-full ${
            state === 'failed' ? 'bg-danger' : 'bg-warning'
          }`}
        />
        {label}
      </span>
    </Tooltip>
  )
}

function BackIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M10 3 5 8l5 5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function ChevronIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-3.5 w-3.5" fill="none" stroke="currentColor" strokeWidth="1.6">
      <path d="m4 6 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function LayoutIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4">
      <rect x="2" y="3" width="12" height="10" rx="1.5" />
      <path d="M8 3v10" />
    </svg>
  )
}

function MenuIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M2.5 4.5h11M2.5 8h11M2.5 11.5h11" strokeLinecap="round" />
    </svg>
  )
}
