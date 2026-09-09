'use client'

/**
 * The compiled document, drawn here rather than handed to the browser.
 *
 * An iframe is less code and gets text selection and printing for free, but it
 * owns the things this pane has to be able to do: the zoom control belongs to
 * the toolbar above, the page number has to be readable, and neither of those
 * is reachable inside somebody else's viewer. The deciding one is SyncTeX --
 * double-clicking a paragraph should put the cursor on the line that produced
 * it, and that needs the click coordinates in the page's own space, which an
 * iframe will not give up.
 *
 * So: pdf.js, one canvas per page. Only the pages near the viewport are
 * actually drawn, because a hundred-page document at 200% is a great deal of
 * bitmap if they all are, and a page that scrolls away is thrown out again.
 *
 * The text layer is real text, positioned over the canvas and transparent, so
 * selection and the browser's own find work on the words rather than on a
 * picture of them.
 */

import {
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type RefObject,
} from 'react'

export type Zoom = 'fit-width' | 'fit-height' | number

/** Where something is on a page, in PDF points from the top-left corner. */
export type PdfHighlight = {
  page: number
  h: number
  v: number
  width: number
  height: number
}

export type PdfViewerHandle = {
  /** Scrolls a page into view, and flashes a highlight over the place. */
  show: (highlight: PdfHighlight) => void
  goToPage: (page: number) => void
}

type Props = {
  url: string
  zoom: Zoom
  /** Told the number of pages once the file is read. */
  onPageCount: (pages: number) => void
  /** Told which page is being looked at, as it scrolls. */
  onCurrentPage: (page: number) => void
  /** Told what a fit-to-width or fit-to-height zoom worked out to. */
  onScale: (scale: number) => void
  /** A double click, in PDF points from the top-left of that page. */
  onDoubleClick: (page: number, h: number, v: number) => void
  handle: RefObject<PdfViewerHandle | null>
}

/** A page's size at scale 1, which every scale is worked out from. */
type PageSize = { width: number; height: number }

// A page is drawn before it reaches the viewport, so scrolling shows no gap.
const AHEAD = '400px'

export function PdfViewer({
  url,
  zoom,
  onPageCount,
  onCurrentPage,
  onScale,
  onDoubleClick,
  handle,
}: Props) {
  const scroller = useRef<HTMLDivElement>(null)
  const [file, setFile] = useState<PdfDocument | null>(null)
  const [sizes, setSizes] = useState<PageSize[]>([])
  const [error, setError] = useState<string | null>(null)
  const [available, setAvailable] = useState({ width: 0, height: 0 })
  const [highlight, setHighlight] = useState<PdfHighlight | null>(null)

  // Read the file. A new URL is a new compile, and the old document is closed
  // rather than left holding its worker and its bitmaps.
  useEffect(() => {
    let live = true
    let opened: PdfDocument | null = null
    setError(null)
    void (async () => {
      try {
        const pdfjs = await loadPdfJs()
        const task = pdfjs.getDocument({ url, withCredentials: true })
        const doc = (await task.promise) as PdfDocument
        if (!live) {
          void doc.destroy()
          return
        }
        opened = doc
        const measured = await Promise.all(
          Array.from({ length: doc.numPages }, async (_, index) => {
            const page = await doc.getPage(index + 1)
            const viewport = page.getViewport({ scale: 1 })
            return { width: viewport.width, height: viewport.height }
          })
        )
        if (!live) {
          return
        }
        setFile(doc)
        setSizes(measured)
        onPageCount(doc.numPages)
      } catch (thrown) {
        if (live) {
          setError(
            thrown instanceof Error
              ? thrown.message
              : 'The PDF could not be read.'
          )
        }
      }
    })()
    return () => {
      live = false
      void opened?.destroy()
    }
  }, [url, onPageCount])

  // The space a page has to fit into.
  useEffect(() => {
    const element = scroller.current
    if (!element) {
      return
    }
    const measure = () =>
      setAvailable({ width: element.clientWidth, height: element.clientHeight })
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  // One scale for the whole document, worked out from the largest page, so
  // that pages of different sizes stay in proportion to one another rather
  // than each filling the pane.
  const scale = useMemo(() => {
    if (typeof zoom === 'number') {
      return zoom
    }
    if (sizes.length === 0 || available.width === 0) {
      return 1
    }
    const padding = 32
    if (zoom === 'fit-width') {
      const widest = Math.max(...sizes.map(size => size.width))
      return Math.max(0.1, (available.width - padding) / widest)
    }
    const tallest = Math.max(...sizes.map(size => size.height))
    return Math.max(0.1, (available.height - padding) / tallest)
  }, [zoom, sizes, available])

  useEffect(() => {
    onScale(scale)
  }, [scale, onScale])

  // Which page is being looked at: the first whose bottom is still below the
  // top of the pane, which is the one somebody would name if asked.
  useEffect(() => {
    const element = scroller.current
    if (!element || sizes.length === 0) {
      return
    }
    let frame = 0
    const report = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const top = element.getBoundingClientRect().top
        for (const page of element.querySelectorAll<HTMLElement>('[data-page]')) {
          if (page.getBoundingClientRect().bottom > top + 8) {
            onCurrentPage(Number(page.dataset.page))
            return
          }
        }
      })
    }
    element.addEventListener('scroll', report, { passive: true })
    report()
    return () => {
      element.removeEventListener('scroll', report)
      cancelAnimationFrame(frame)
    }
  }, [sizes.length, onCurrentPage])

  useImperativeHandle(
    handle,
    () => ({
      show(target) {
        const element = scroller.current
        const page = element?.querySelector<HTMLElement>(
          `[data-page="${target.page}"]`
        )
        if (!element || !page) {
          return
        }
        // A little above the thing being pointed at: something scrolled to the
        // very top of the pane reads as the start of the page rather than as
        // the answer to a question.
        const offset = (target.v - target.height) * scale - 60
        element.scrollTo({
          top: page.offsetTop + Math.max(0, offset),
          behavior: 'smooth',
        })
        setHighlight(target)
      },
      goToPage(page) {
        const element = scroller.current
        const target = element?.querySelector<HTMLElement>(
          `[data-page="${page}"]`
        )
        if (element && target) {
          element.scrollTo({ top: target.offsetTop, behavior: 'smooth' })
        }
      },
    }),
    [scale]
  )

  // The highlight is a hint rather than a state: it says "here", and is done.
  useEffect(() => {
    if (!highlight) {
      return
    }
    const timer = setTimeout(() => setHighlight(null), 2000)
    return () => clearTimeout(timer)
  }, [highlight])

  if (error) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <p className="max-w-sm text-center text-[14px] leading-5 text-[var(--content-secondary)]">
          {error}
        </p>
      </div>
    )
  }

  return (
    <div
      ref={scroller}
      data-pdf-scroller
      className="h-full w-full overflow-auto bg-[var(--bg-light-tertiary)]"
    >
      <div className="flex flex-col items-center gap-2 py-4">
        {sizes.map((size, index) => (
          <PdfPage
            key={index}
            file={file}
            number={index + 1}
            size={size}
            scale={scale}
            highlight={highlight?.page === index + 1 ? highlight : null}
            onDoubleClick={onDoubleClick}
          />
        ))}
      </div>
    </div>
  )
}

/**
 * One page.
 *
 * The wrapper is sized from the page's own dimensions before anything is
 * drawn, so the document has its full height immediately and the scrollbar
 * does not jump about as pages render.
 */
function PdfPage({
  file,
  number,
  size,
  scale,
  highlight,
  onDoubleClick,
}: {
  file: PdfDocument | null
  number: number
  size: PageSize
  scale: number
  highlight: PdfHighlight | null
  onDoubleClick: (page: number, h: number, v: number) => void
}) {
  const wrapper = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const text = useRef<HTMLDivElement>(null)
  const [near, setNear] = useState(false)

  const width = Math.floor(size.width * scale)
  const height = Math.floor(size.height * scale)

  useEffect(() => {
    const element = wrapper.current
    if (!element) {
      return
    }
    const observer = new IntersectionObserver(
      entries => setNear(entries.some(entry => entry.isIntersecting)),
      { root: element.closest('[data-pdf-scroller]'), rootMargin: AHEAD }
    )
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!file || !near) {
      return
    }
    let live = true
    let drawing: { cancel: () => void } | null = null
    void (async () => {
      const page = await file.getPage(number)
      const element = canvas.current
      if (!live || !element) {
        return
      }
      const viewport = page.getViewport({ scale })
      // Drawn at the screen's own pixel density and scaled down by CSS, which
      // is the difference between sharp text and blurred text on any display
      // made in the last decade.
      const density = Math.min(window.devicePixelRatio || 1, 2)
      element.width = Math.floor(viewport.width * density)
      element.height = Math.floor(viewport.height * density)
      const context = element.getContext('2d')
      if (!context) {
        return
      }
      const render = page.render({
        canvas: element,
        canvasContext: context,
        viewport,
        transform: density === 1 ? undefined : [density, 0, 0, density, 0, 0],
      })
      drawing = render
      try {
        await render.promise
      } catch {
        // A render is cancelled whenever the scale changes while it is
        // running, which happens every time somebody drags the zoom. That is
        // the mechanism working, not a failure.
        return
      }
      if (!live || !text.current) {
        return
      }
      const pdfjs = await loadPdfJs()
      const content = await page.getTextContent()
      if (!live || !text.current) {
        return
      }
      text.current.replaceChildren()
      const layer = new pdfjs.TextLayer({
        textContentSource: content,
        container: text.current,
        viewport,
      })
      await layer.render()
    })()
    return () => {
      live = false
      drawing?.cancel()
    }
  }, [file, near, number, scale])

  return (
    <div
      ref={wrapper}
      data-page={number}
      className="relative bg-white shadow-[0_1px_4px_rgba(0,0,0,0.25)]"
      style={
        {
          width,
          height,
          '--scale-factor': scale,
          '--total-scale-factor': scale,
          '--user-unit': 1,
          '--scale-round-x': '1px',
          '--scale-round-y': '1px',
        } as CSSProperties
      }
      onDoubleClick={event => {
        const box = event.currentTarget.getBoundingClientRect()
        onDoubleClick(
          number,
          (event.clientX - box.left) / scale,
          (event.clientY - box.top) / scale
        )
      }}
    >
      <canvas
        ref={canvas}
        style={{ width, height }}
        className="block"
        aria-label={`Page ${number}`}
      />
      <div ref={text} className="pdf-text-layer textLayer" />
      {highlight ? (
        <span
          className="pointer-events-none absolute rounded-[2px] bg-[var(--bg-accent-01)]/25 ring-1 ring-[var(--bg-accent-01)]"
          style={{
            left: highlight.h * scale,
            top: (highlight.v - highlight.height) * scale,
            width: Math.max(highlight.width * scale, 8),
            height: Math.max(highlight.height * scale, 12),
          }}
        />
      ) : null}
    </div>
  )
}

/* pdf.js, typed only where this file touches it. Its own declarations are a
   separate build that pulls in a great deal this project does not use. */

type PdfViewport = { width: number; height: number }

type PdfPageProxy = {
  getViewport: (options: { scale: number }) => PdfViewport
  render: (options: Record<string, unknown>) => {
    promise: Promise<void>
    cancel: () => void
  }
  getTextContent: () => Promise<unknown>
}

type PdfDocument = {
  numPages: number
  getPage: (page: number) => Promise<PdfPageProxy>
  destroy: () => Promise<void>
}

type PdfJs = {
  getDocument: (options: Record<string, unknown>) => { promise: Promise<unknown> }
  GlobalWorkerOptions: { workerSrc: string }
  TextLayer: new (options: Record<string, unknown>) => {
    render: () => Promise<void>
  }
}

let loading: Promise<PdfJs> | null = null

/**
 * pdf.js, fetched the first time a PDF is shown.
 *
 * Imported rather than bundled into the page: it is over a megabyte, and
 * somebody who opens a project to type in it should not have waited for it.
 * The worker is served from /public because parsing a large PDF on the main
 * thread stops the editor from responding while it happens.
 */
async function loadPdfJs(): Promise<PdfJs> {
  if (!loading) {
    loading = import('pdfjs-dist').then(module => {
      const pdfjs = module as unknown as PdfJs
      pdfjs.GlobalWorkerOptions.workerSrc = '/pdf.worker.min.mjs'
      return pdfjs
    })
  }
  return loading
}

/**
 * The zoom steps the buttons move between.
 *
 * Not a multiplier applied to whatever the scale happens to be: fit-to-width
 * gives an arbitrary number, and zooming out from 87% should arrive at 75%
 * rather than at 69.6%.
 */
export const ZOOM_STEPS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4]

export function nextZoom(from: number, direction: 1 | -1): number {
  if (direction === 1) {
    return ZOOM_STEPS.find(step => step > from + 0.001) ?? from
  }
  return [...ZOOM_STEPS].reverse().find(step => step < from - 0.001) ?? from
}

/** Reads the zoom back as a percentage for the control that shows it. */
export function useZoomLabel(zoom: Zoom, scale: number): string {
  return useCallback(
    () =>
      typeof zoom === 'number'
        ? `${Math.round(zoom * 100)}%`
        : `${Math.round(scale * 100)}%`,
    [zoom, scale]
  )()
}
