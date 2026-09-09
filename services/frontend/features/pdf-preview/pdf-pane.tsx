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

import { Button, Chip, ScrollShadow, Spinner, Tab, Tabs, Tooltip } from '@heroui/react'
import { useMemo, useState } from 'react'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import type { LogEntry } from './log-parser'

export function PdfPane() {
  const compile = useCompile()
  const layout = useLayout()

  const problems = compile.errors.length + compile.warnings.length

  return (
    <div className="flex h-full min-h-0 flex-col bg-default-50">
      <PdfToolbar />
      <div className="relative min-h-0 flex-1">
        {compile.pdfUrl ? (
          <iframe
            key={compile.pdfUrl}
            src={compile.pdfUrl}
            title="Compiled PDF"
            className="h-full w-full border-0 bg-default-100"
          />
        ) : (
          <EmptyPdf />
        )}
        {compile.compiling ? (
          <div className="absolute inset-x-0 top-0 flex items-center justify-center gap-2 bg-default-100/90 py-1.5 text-xs text-default-600">
            <Spinner size="sm" />
            Compiling…
          </div>
        ) : null}
      </div>

      {layout.showLogs ? (
        <LogPane />
      ) : problems > 0 ? (
        <button
          type="button"
          onClick={layout.toggleLogs}
          className="flex items-center justify-between border-t border-divider px-3 py-2 text-left text-xs hover:bg-default-100"
        >
          <span className="flex items-center gap-2">
            {compile.errors.length > 0 ? (
              <Chip size="sm" color="danger" variant="flat">
                {compile.errors.length} error{compile.errors.length === 1 ? '' : 's'}
              </Chip>
            ) : null}
            {compile.warnings.length > 0 ? (
              <Chip size="sm" color="warning" variant="flat">
                {compile.warnings.length} warning{compile.warnings.length === 1 ? '' : 's'}
              </Chip>
            ) : null}
          </span>
          <span className="text-default-500">Show log</span>
        </button>
      ) : null}
    </div>
  )
}

function PdfToolbar() {
  const compile = useCompile()
  const layout = useLayout()

  return (
    <div className="flex items-center gap-1 border-b border-divider px-2 py-1.5">
      <Button
        size="sm"
        color="primary"
        onPress={compile.compiling ? compile.stop : compile.startCompile}
        className="h-7"
      >
        {compile.compiling ? 'Stop' : 'Recompile'}
      </Button>

      {compile.stale && !compile.compiling ? (
        <span className="text-xs text-warning-600">Changed since last build</span>
      ) : null}

      <div className="flex-1" />

      {compile.errors.length > 0 ? (
        <Chip size="sm" color="danger" variant="flat">
          {compile.errors.length}
        </Chip>
      ) : null}
      {compile.warnings.length > 0 ? (
        <Chip size="sm" color="warning" variant="flat">
          {compile.warnings.length}
        </Chip>
      ) : null}

      <Tooltip content={layout.showLogs ? 'Hide log' : 'Show log'} delay={400}>
        <Button
          size="sm"
          variant="light"
          isIconOnly
          className="h-7 w-7 min-w-7"
          aria-label="Toggle the log"
          onPress={layout.toggleLogs}
        >
          <LogIcon />
        </Button>
      </Tooltip>

      {compile.pdfUrl ? (
        <Tooltip content="Open in a new tab" delay={400}>
          <Button
            as="a"
            href={compile.pdfUrl}
            target="_blank"
            rel="noreferrer"
            size="sm"
            variant="light"
            isIconOnly
            className="h-7 w-7 min-w-7"
            aria-label="Open the PDF in a new tab"
          >
            <ExternalIcon />
          </Button>
        </Tooltip>
      ) : null}
    </div>
  )
}

function EmptyPdf() {
  const compile = useCompile()

  if (compile.error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
        <p className="text-sm text-danger">{compile.error}</p>
        <Button size="sm" variant="flat" onPress={compile.startCompile}>
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
      <Button size="sm" color="primary" onPress={compile.startCompile}>
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
        <Button
          size="sm"
          variant="light"
          isIconOnly
          className="h-6 w-6 min-w-6"
          aria-label="Hide the log"
          onPress={() => layout.setShowLogs(false)}
        >
          <CloseIcon />
        </Button>
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
