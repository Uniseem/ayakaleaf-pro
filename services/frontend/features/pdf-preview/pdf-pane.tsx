'use client'

/**
 * The compiled document, and what went wrong making it.
 *
 * The PDF is drawn here rather than handed to the browser, because the things
 * this pane has to do reach inside it: the zoom control is in the toolbar, the
 * page number has to be readable, and double-clicking a paragraph has to put
 * the cursor on the line that produced it. See pdf-viewer.tsx.
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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { syncFromCode, syncFromPdf } from '@/lib/editor'
import {
  PdfViewer,
  nextZoom,
  type PdfViewerHandle,
  type Zoom,
} from './pdf-viewer'
import type { LogEntry } from './log-parser'

export function PdfPane() {
  const compile = useCompile()
  const layout = useLayout()
  const project = useProject()
  const editor = useEditor()

  const viewer = useRef<PdfViewerHandle | null>(null)
  const [zoom, setZoom] = useState<Zoom>('fit-width')
  const [scale, setScale] = useState(1)
  const [page, setPage] = useState(1)
  const [pages, setPages] = useState(0)
  const [syncError, setSyncError] = useState<string | null>(null)

  // A jump that is waiting for its file to finish opening.
  const pending = useRef<{ docId: string; line: number } | null>(null)

  /**
   * A double click in the PDF puts the cursor on the line that made it.
   *
   * The file it names is often not the one that is open -- a thesis is a
   * dozen chapters -- so the file is opened first and the jump waits for it.
   * Jumping straight away moves the cursor in the document being left, and
   * the new one then opens at its own beginning: the file is right, the line
   * is wrong, and it looks close enough to working to be missed.
   */
  const jumpToSource = useCallback(
    async (clickedPage: number, h: number, v: number) => {
      setSyncError(null)
      try {
        const { code } = await syncFromPdf(project.projectId, clickedPage, h, v)
        const first = code?.[0]
        if (!first) {
          setSyncError('Nothing on this page came from the source.')
          return
        }
        const entry = project.files.find(file => file.path === first.file)
        if (entry && entry.id !== editor.current?.id) {
          pending.current = { docId: entry.id, line: first.line }
          editor.open(entry)
          return
        }
        window.dispatchEvent(
          new CustomEvent('ide:goto-line', { detail: { line: first.line - 1 } })
        )
      } catch {
        setSyncError('The document has not been compiled yet.')
      }
    },
    [project.projectId, project.files, editor]
  )

  // The file asked for has arrived. `loading` is the signal rather than the
  // id, which changes the moment the file is chosen and says nothing about
  // whether its text is here yet.
  const openId = editor.current?.id
  const stillLoading = editor.loading
  useEffect(() => {
    const wanted = pending.current
    if (!wanted || stillLoading || openId !== wanted.docId) {
      return
    }
    pending.current = null
    window.dispatchEvent(
      new CustomEvent('ide:goto-line', { detail: { line: wanted.line - 1 } })
    )
  }, [openId, stillLoading])

  /** And the other direction, asked for by the editor. */
  useEffect(() => {
    const show = async (event: Event) => {
      const detail = (event as CustomEvent<{ file: string; line: number }>).detail
      if (!detail) {
        return
      }
      setSyncError(null)
      try {
        const { pdf } = await syncFromCode(
          project.projectId,
          detail.file,
          detail.line
        )
        const first = pdf?.[0]
        if (!first) {
          setSyncError('That line does not appear in the PDF.')
          return
        }
        viewer.current?.show(first)
      } catch {
        setSyncError('The document has not been compiled yet.')
      }
    }
    window.addEventListener('ide:show-in-pdf', show)
    return () => window.removeEventListener('ide:show-in-pdf', show)
  }, [project.projectId])

  // A message about a jump is about that jump, and stops being true the
  // moment anything else happens.
  useEffect(() => {
    if (!syncError) {
      return
    }
    const timer = setTimeout(() => setSyncError(null), 4000)
    return () => clearTimeout(timer)
  }, [syncError])

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--bg-light-secondary)]">
      <PdfToolbar
        zoom={zoom}
        scale={scale}
        setZoom={setZoom}
        page={page}
        pages={pages}
        goToPage={target => viewer.current?.goToPage(target)}
      />
      <div className="relative min-h-0 flex-1">
        {compile.pdfUrl ? (
          <PdfViewer
            key={compile.pdfUrl}
            url={compile.pdfUrl}
            zoom={zoom}
            onPageCount={setPages}
            onCurrentPage={setPage}
            onScale={setScale}
            onDoubleClick={(clickedPage, h, v) =>
              void jumpToSource(clickedPage, h, v)
            }
            handle={viewer}
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
        {syncError ? (
          <p className="absolute inset-x-0 bottom-0 bg-[var(--bg-dark-secondary)]/90 px-3 py-1.5 text-center text-[12px] leading-4 text-white">
            {syncError}
          </p>
        ) : null}
      </div>

      {layout.showLogs ? <LogPane /> : null}
    </div>
  )
}

function PdfToolbar({
  zoom,
  scale,
  setZoom,
  page,
  pages,
  goToPage,
}: {
  zoom: Zoom
  scale: number
  setZoom: (zoom: Zoom) => void
  page: number
  pages: number
  goToPage: (page: number) => void
}) {
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

      {pages > 0 ? (
        <PageControl page={page} pages={pages} goToPage={goToPage} />
      ) : null}

      {pages > 0 ? <ZoomControl zoom={zoom} scale={scale} setZoom={setZoom} /> : null}

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

/**
 * Which page is on screen, and a way to type another one.
 *
 * The number is an input rather than a label because a reader of a long
 * document knows the page they want, and scrolling to it is the slow way.
 */
function PageControl({
  page,
  pages,
  goToPage,
}: {
  page: number
  pages: number
  goToPage: (page: number) => void
}) {
  const [typed, setTyped] = useState<string | null>(null)

  return (
    <form
      className="flex items-center gap-1 text-[12px] leading-4 text-[var(--content-secondary)]"
      onSubmit={event => {
        event.preventDefault()
        const wanted = Number(typed)
        if (Number.isFinite(wanted) && wanted >= 1 && wanted <= pages) {
          goToPage(Math.floor(wanted))
        }
        setTyped(null)
      }}
    >
      <input
        aria-label="Page"
        inputMode="numeric"
        value={typed ?? String(page)}
        onChange={event => setTyped(event.target.value)}
        onFocus={event => event.target.select()}
        onBlur={() => setTyped(null)}
        className="h-6 w-9 rounded-[4px] border border-[var(--border-divider)] bg-[var(--bg-light-primary)] text-center tabular-nums focus:border-[var(--border-active)] focus:outline-none"
      />
      <span>of {pages}</span>
    </form>
  )
}

/**
 * How large the page is drawn.
 *
 * Fit-to-width is the default and the first entry, because it is what almost
 * everybody wants almost always: a page of A4 at 100% is wider than this pane
 * ever is, and reading it means scrolling sideways for every line.
 */
function ZoomControl({
  zoom,
  scale,
  setZoom,
}: {
  zoom: Zoom
  scale: number
  setZoom: (zoom: Zoom) => void
}) {
  const percent = Math.round((typeof zoom === 'number' ? zoom : scale) * 100)

  return (
    <div className="flex items-center">
      <Tooltip content="Zoom out" delay={400}>
        <button
          type="button"
          aria-label="Zoom out"
          onClick={() => setZoom(nextZoom(scale, -1))}
          className="flex h-7 w-7 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]"
        >
          <MinusIcon />
        </button>
      </Tooltip>

      <Dropdown placement="bottom-end">
        <DropdownTrigger>
          <button
            type="button"
            aria-label="Zoom"
            className="flex h-7 min-w-14 items-center justify-center gap-0.5 rounded-[4px] px-1 text-[12px] leading-4 tabular-nums text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]"
          >
            {percent}%
            <Caret />
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Zoom"
          onAction={key => {
            const chosen = String(key)
            setZoom(
              chosen === 'fit-width' || chosen === 'fit-height'
                ? chosen
                : Number(chosen)
            )
          }}
        >
          <DropdownItem key="fit-width">Fit to width</DropdownItem>
          <DropdownItem key="fit-height">Fit to height</DropdownItem>
          <DropdownItem key="0.5">50%</DropdownItem>
          <DropdownItem key="0.75">75%</DropdownItem>
          <DropdownItem key="1">100%</DropdownItem>
          <DropdownItem key="1.5">150%</DropdownItem>
          <DropdownItem key="2">200%</DropdownItem>
          <DropdownItem key="4">400%</DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Tooltip content="Zoom in" delay={400}>
        <button
          type="button"
          aria-label="Zoom in"
          onClick={() => setZoom(nextZoom(scale, 1))}
          className="flex h-7 w-7 items-center justify-center rounded-[4px] text-[var(--content-secondary)] hover:bg-[var(--hover-interaction)]"
        >
          <PlusIcon />
        </button>
      </Tooltip>
    </div>
  )
}

function MinusIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <path d="M3.5 8h9" strokeLinecap="round" />
    </svg>
  )
}

function PlusIcon() {
  return (
    <svg viewBox="0 0 16 16" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <path d="M8 3.5v9M3.5 8h9" strokeLinecap="round" />
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
