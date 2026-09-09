'use client'

/**
 * The compiled document, and what went wrong making it.
 *
 * The PDF is shown in an iframe rather than rendered here. The browser's own
 * viewer already does the things a PDF viewer must do -- text selection,
 * search, printing, zoom that does not blur -- and reimplementing them on a
 * canvas is a large amount of code to arrive somewhere worse.
 *
 * The interesting part of this pane is the other half: what TeX said. A raw
 * log is unreadable, so it is parsed and shown as entries, each of which knows
 * which file and line it came from and can put the cursor there.
 */

import {
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  ScrollShadow,
  Spinner,
  Tab,
  Tabs,
  Tooltip,
} from '@heroui/react'
import { Button } from '@/components/ui'
import { useMemo, useState } from 'react'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import type { LogEntry } from './log-parser'

export function PdfPane() {
  const compile = useCompile()
  const layout = useLayout()

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--bg-light-secondary)]">
      <PdfToolbar />
      <div className="relative min-h-0 flex-1">
        {compile.pdfUrl ? (
          <iframe
            key={compile.pdfUrl}
            src={compile.pdfUrl}
            title="Compiled PDF"
            className="h-full w-full border-0 bg-[var(--bg-light-tertiary)]"
          />
        ) : (
          <EmptyPdf />
        )}
        {compile.compiling ? (
          <div className="absolute inset-x-0 top-0 flex items-center justify-center gap-2 bg-[var(--bg-light-secondary)]/90 py-1.5 text-[12px] leading-4 text-[var(--content-secondary)]">
            <Spinner size="sm" />
            Compiling…
          </div>
        ) : null}
      </div>

      {layout.showLogs ? <LogPane /> : null}
    </div>
  )
}

function PdfToolbar() {
  const compile = useCompile()
  const layout = useLayout()
  const [options, setOptions] = useState(false)

  const problems = compile.errors.length + compile.warnings.length

  return (
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-[var(--border-divider)] bg-[var(--bg-light-primary)] px-2">
      {/* One pill split in two: the action on the left, its settings on the
          right. The original's shape, and it keeps the common case one click
          away while the options stay reachable. */}
      <div className="flex items-stretch">
        <button
          type="button"
          onClick={compile.compiling ? compile.stop : compile.startCompile}
          className="inline-flex h-6 items-center rounded-l-full bg-[var(--bg-accent-01)] px-4 text-[14px] font-semibold leading-5 text-white hover:bg-[var(--bg-accent-02)]"
        >
          {compile.compiling ? 'Stop' : 'Recompile'}
        </button>
        <Dropdown
          placement="bottom-start"
          isOpen={options}
          onOpenChange={setOptions}
        >
          <DropdownTrigger>
            <button
              type="button"
              aria-label="Compile options"
              className="inline-flex h-6 items-center rounded-r-full border-l border-white/25 bg-[var(--bg-accent-01)] px-2 text-white hover:bg-[var(--bg-accent-02)]"
            >
              <Caret />
            </button>
          </DropdownTrigger>
          <DropdownMenu aria-label="Compile options" closeOnSelect={false}>
            <DropdownItem
              key="auto"
              onPress={() => compile.setAutoCompile(!compile.autoCompile)}
              endContent={<Tick on={compile.autoCompile} />}
            >
              Compile automatically
            </DropdownItem>
            <DropdownItem
              key="draft"
              description="Skips images, which is faster"
              onPress={() => compile.setDraft(!compile.draft)}
              endContent={<Tick on={compile.draft} />}
            >
              Draft mode
            </DropdownItem>
            <DropdownItem
              key="stop"
              onPress={() => compile.setStopOnFirstError(!compile.stopOnFirstError)}
              endContent={<Tick on={compile.stopOnFirstError} />}
            >
              Stop on first error
            </DropdownItem>
          </DropdownMenu>
        </Dropdown>
      </div>

      {compile.stale && !compile.compiling ? (
        <span className="text-[12px] leading-4 text-[var(--content-warning)]">
          Changed since last build
        </span>
      ) : null}

      <div className="flex-1" />

      {problems > 0 ? (
        <button
          type="button"
          onClick={layout.toggleLogs}
          className="flex h-7 items-center gap-1.5 rounded-[4px] px-2 text-[12px] leading-4 hover:bg-[var(--hover-interaction)]"
        >
          {compile.errors.length > 0 ? (
            <span className="text-[var(--content-danger)]">
              {compile.errors.length} error{compile.errors.length === 1 ? '' : 's'}
            </span>
          ) : null}
          {compile.warnings.length > 0 ? (
            <span className="text-[var(--content-warning)]">
              {compile.warnings.length} warning{compile.warnings.length === 1 ? '' : 's'}
            </span>
          ) : null}
        </button>
      ) : null}

      <Tooltip content={layout.showLogs ? 'Hide log' : 'Show log'} delay={400}>
        <button
          type="button"
          aria-label="Toggle the log"
          onClick={layout.toggleLogs}
          className={`flex h-7 w-7 items-center justify-center rounded-[4px] ${
            layout.showLogs
              ? 'bg-[var(--bg-accent-03)] text-[var(--link-web)]'
              : 'text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]'
          }`}
        >
          <LogIcon />
        </button>
      </Tooltip>

      {compile.pdfUrl ? (
        <Tooltip content="Download PDF" delay={400}>
          <a
            href={compile.pdfUrl}
            download
            aria-label="Download the PDF"
            className="flex h-7 w-7 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]"
          >
            <DownloadIcon />
          </a>
        </Tooltip>
      ) : null}

      {compile.pdfUrl ? (
        <Tooltip content="Open in a new tab" delay={400}>
          <a
            href={compile.pdfUrl}
            target="_blank"
            rel="noreferrer"
            aria-label="Open the PDF in a new tab"
            className="flex h-7 w-7 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]"
          >
            <ExternalIcon />
          </a>
        </Tooltip>
      ) : null}
    </div>
  )
}

function Tick({ on }: { on: boolean }) {
  return on ? (
    <svg viewBox="0 0 16 16" className="h-4 w-4 text-[var(--content-positive)]" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <path d="m3.5 8.5 3 3 6-7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  ) : (
    <span className="h-4 w-4" />
  )
}

function Caret() {
  return (
    <svg viewBox="0 0 16 16" className="h-3 w-3" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <path d="m4 6 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function DownloadIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden>
      <path d="M8 2.5v8M5 7.5l3 3 3-3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M2.5 12v.5A1.5 1.5 0 0 0 4 14h8a1.5 1.5 0 0 0 1.5-1.5V12" strokeLinecap="round" />
    </svg>
  )
}

function EmptyPdf() {
  const compile = useCompile()

  if (compile.error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
        <p className="text-sm text-danger">{compile.error}</p>
        <Button size="sm" kind="secondary" onClick={compile.startCompile}>
          Try again
        </Button>
      </div>
    )
  }

  if (compile.hasCompiled && compile.errors.length > 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 px-6 text-center">
        <p className="text-sm font-medium">This did not compile.</p>
        <p className="max-w-sm text-xs text-default-500">
          The log below says why. The first error is usually the only real one:
          everything after it is TeX trying to carry on.
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
      <p className="text-sm text-default-500">No PDF yet.</p>
      <Button size="sm" onClick={compile.startCompile}>
        Compile
      </Button>
    </div>
  )
}

function LogPane() {
  const compile = useCompile()
  const layout = useLayout()
  const [tab, setTab] = useState<string>('problems')

  const counts = {
    errors: compile.errors.length,
    warnings: compile.warnings.length,
    typesetting: compile.typesetting.length,
  }

  const shown = useMemo(() => {
    switch (tab) {
      case 'errors':
        return compile.errors
      case 'warnings':
        return compile.warnings
      case 'typesetting':
        return compile.typesetting
      default:
        return [...compile.errors, ...compile.warnings]
    }
  }, [tab, compile.errors, compile.warnings, compile.typesetting])

  return (
    <section className="flex h-2/5 min-h-[140px] flex-col border-t border-divider bg-background">
      <div className="flex items-center gap-2 border-b border-divider px-2">
        <Tabs
          aria-label="Log"
          size="sm"
          variant="underlined"
          selectedKey={tab}
          onSelectionChange={key => setTab(String(key))}
          classNames={{ tabList: 'gap-3' }}
        >
          <Tab key="problems" title={`Problems (${counts.errors + counts.warnings})`} />
          <Tab key="errors" title={`Errors (${counts.errors})`} />
          <Tab key="warnings" title={`Warnings (${counts.warnings})`} />
          <Tab key="typesetting" title={`Typesetting (${counts.typesetting})`} />
          <Tab key="raw" title="Raw log" />
        </Tabs>
        <div className="flex-1" />
        <button
          type="button"
          aria-label="Hide the log"
          onClick={() => layout.setShowLogs(false)}
          className="flex h-6 w-6 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]"
        >
          <CloseIcon />
        </button>
      </div>

      <ScrollShadow className="min-h-0 flex-1">
        {tab === 'raw' ? (
          <pre className="whitespace-pre-wrap break-words p-3 font-mono text-[11px] leading-relaxed text-default-600">
            {compile.rawLog || 'Nothing yet.'}
          </pre>
        ) : shown.length === 0 ? (
          <p className="p-4 text-sm text-default-400">
            {compile.hasCompiled ? 'Nothing to report.' : 'Compile to see the log.'}
          </p>
        ) : (
          <ul className="divide-y divide-divider">
            {shown.map((entry, index) => (
              <LogRow key={`${entry.raw}-${index}`} entry={entry} />
            ))}
          </ul>
        )}
      </ScrollShadow>
    </section>
  )
}

function LogRow({ entry }: { entry: LogEntry }) {
  const [open, setOpen] = useState(false)
  const project = useProject()
  const editor = useEditor()

  const colour =
    entry.level === 'error'
      ? 'text-danger'
      : entry.level === 'warning'
        ? 'text-warning-600'
        : 'text-default-500'

  // TeX names files by the path it compiled them at, which starts with ./ or
  // an absolute compile directory. Only the tail is meaningful here.
  const where = useMemo(() => {
    if (!entry.file) {
      return null
    }
    const cleaned = entry.file.replace(/^\.\//, '')
    const found =
      project.entryByPath(cleaned) ??
      project.files.find(file => cleaned.endsWith(file.path))
    return found ?? null
  }, [entry.file, project])

  return (
    <li className="px-3 py-2 text-xs">
      <div className="flex items-start gap-2">
        <span className={`mt-px shrink-0 font-medium uppercase ${colour}`}>
          {entry.level === 'typesetting' ? 'box' : entry.level}
        </span>
        <div className="min-w-0 flex-1">
          <p className="break-words text-default-700">{entry.message}</p>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-default-500">
            {where ? (
              <button
                type="button"
                className="underline underline-offset-2 hover:text-primary"
                onClick={() => editor.open(where)}
              >
                {where.path}
                {entry.line ? `:${entry.line}` : ''}
              </button>
            ) : entry.file ? (
              <span>
                {entry.file}
                {entry.line ? `:${entry.line}` : ''}
              </span>
            ) : null}
            {entry.content ? (
              <button
                type="button"
                className="underline underline-offset-2 hover:text-primary"
                onClick={() => setOpen(value => !value)}
              >
                {open ? 'Hide detail' : 'Show detail'}
              </button>
            ) : null}
          </div>
          {open && entry.content ? (
            <pre className="mt-2 overflow-x-auto rounded bg-default-100 p-2 font-mono text-[11px] leading-relaxed">
              {entry.content.trim()}
            </pre>
          ) : null}
        </div>
      </div>
    </li>
  )
}

function LogIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4">
      <path d="M3 3h10v10H3z" />
      <path d="M5.5 6h5M5.5 8.5h5M5.5 11h3" strokeLinecap="round" />
    </svg>
  )
}

function ExternalIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4">
      <path d="M9 3h4v4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M13 3 7.5 8.5" strokeLinecap="round" />
      <path d="M12 9.5v3A1.5 1.5 0 0 1 10.5 14h-7A1.5 1.5 0 0 1 2 12.5v-7A1.5 1.5 0 0 1 3.5 4h3" strokeLinecap="round" />
    </svg>
  )
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.4">
      <path d="M4 4l8 8M12 4l-8 8" strokeLinecap="round" />
    </svg>
  )
}
