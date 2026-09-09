'use client'

/**
 * The PDF, drawn by pdf.js, from pdf-preview/components/pdf-js-viewer.
 *
 * The viewer is pdf.js's own, wrapped in PDFJSWrapper; this component ties
 * its events to state (page, scale, page count), restores where the reader
 * was after each compile, and turns a double-click on a word into a jump
 * to the line that produced it.
 */

import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { debounce, throttle } from '@/lib/timing'
import PdfViewerControlsToolbar from './pdf-viewer-controls-toolbar'
import { useProject } from '@/features/ide/contexts/project-context'
import { usePersistedState } from '@/lib/hooks'
import { buildHighlightElement } from '../util/highlights'
import PDFJSWrapper from '../util/pdf-js-wrapper'
import { withErrorBoundary } from '@/components/ol/error-boundary'
import PdfPreviewErrorBoundaryFallback from './pdf-preview-error-boundary-fallback'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { debugConsole } from '@/lib/debug'
import { usePdfPreviewContext } from './pdf-preview-provider'
import usePresentationMode from '../hooks/use-presentation-mode'
import useMouseWheelZoom from '../hooks/use-mouse-wheel-zoom'
import { PDFJS } from '../util/pdf-js'
import type { PDFFile } from '../util/types'

type PdfJsViewerProps = {
  url: string
  pdfFile: PDFFile
}

type TextLayerRenderedEvent = {
  pageNumber: number
  source: {
    div?: HTMLDivElement
    textLayerDiv?: HTMLDivElement
    textLayer?: { div: HTMLDivElement }
  }
}

function PdfJsViewer({ url, pdfFile }: PdfJsViewerProps) {
  const { projectId } = useProject()

  const { setError, highlights, position, setPosition } = useCompile()

  const { setLoadingError } = usePdfPreviewContext()

  // state values persisted in localStorage to restore on load
  const [scale, setScale] = usePersistedState(`pdf-viewer-scale:${projectId}`, 'page-width')

  // rawScale is different from scale as it is always a number.
  // This is relevant when scale is e.g. 'page-width'.
  const [rawScale, setRawScale] = useState<number | null>(null)
  const [page, setPage] = useState<number | null>(null)
  const [totalPages, setTotalPages] = useState<number | null>(null)

  // local state values
  const [pdfJsWrapper, setPdfJsWrapper] = useState<PDFJSWrapper | null>()
  const [initialised, setInitialised] = useState(false)

  const handlePageChange = useCallback(
    (newPage: number) => {
      if (!totalPages || newPage < 1 || newPage > totalPages) {
        return
      }

      setPage(newPage)
      if (pdfJsWrapper?.viewer) {
        pdfJsWrapper.viewer.currentPageNumber = newPage
      }
    },
    [pdfJsWrapper, setPage, totalPages]
  )

  // create the viewer when the container is mounted
  const handleContainer = useCallback(
    (parent: HTMLDivElement | null) => {
      if (parent) {
        try {
          setPdfJsWrapper(new PDFJSWrapper(parent.firstChild as HTMLDivElement))
        } catch (error) {
          setLoadingError(true)
          debugConsole.error(error)
        }
      }
    },
    [setLoadingError]
  )

  useEffect(() => {
    return () => {
      setPdfJsWrapper(null)
    }
  }, [])

  // listen for events and trigger rendering.
  // Do everything in one effect to mitigate de-sync between events.
  useEffect(() => {
    if (!pdfJsWrapper) return

    const handlePagesinit = () => {
      // Set scale immediately to avoid a one-frame flash at PDF.js's default
      // 1.333 scale (96/72 DPI) before the React restore effect can correct it.
      pdfJsWrapper.viewer.currentScaleValue = scaleRef.current
      setInitialised(true)
    }

    const handleRenderedInitialPageNumber = () => {
      setPage(pdfJsWrapper.viewer.currentPageNumber)
      // Only need to set the initial page number once.
      pdfJsWrapper.eventBus.off('pagerendered', handleRenderedInitialPageNumber)
    }

    const handleScaleChanged = (scale: { scale: number }) => {
      setRawScale(scale.scale)
    }

    const handlePageChanging = (event: { pageNumber: number }) => {
      setPage(event.pageNumber)
    }

    // `pagesinit` fires when the data for rendering the first page is ready.
    pdfJsWrapper.eventBus.on('pagesinit', handlePagesinit)
    // Once a page has been rendered we can set the initial current page number.
    pdfJsWrapper.eventBus.on('pagerendered', handleRenderedInitialPageNumber)
    pdfJsWrapper.eventBus.on('scalechanging', handleScaleChanged)
    // `pagechanging` fires when the page number changes.
    pdfJsWrapper.eventBus.on('pagechanging', handlePageChanging)

    return () => {
      pdfJsWrapper.eventBus.off('pagesinit', handlePagesinit)
      pdfJsWrapper.eventBus.off('pagerendered', handleRenderedInitialPageNumber)
      pdfJsWrapper.eventBus.off('scalechanging', handleScaleChanged)
      pdfJsWrapper.eventBus.off('pagechanging', handlePageChanging)
    }
  }, [pdfJsWrapper])

  // load the PDF document from the URL
  useEffect(() => {
    if (pdfJsWrapper && url) {
      setInitialised(false)
      setError(undefined)

      const abortController = new AbortController()
      const handleFetchError = (err: unknown) => {
        if (abortController.signal.aborted) return
        // The error is already logged at the call-site with additional context.
        if (err instanceof PDFJS.ResponseException && err.missing) {
          setError('rendering-error-expected')
        } else {
          setError('rendering-error')
        }
      }
      pdfJsWrapper
        .loadDocument({ url, pdfFile, abortController, handleFetchError })
        .then(doc => {
          if (doc) {
            setTotalPages(doc.numPages)
          }
        })
        .catch(error => {
          if (abortController.signal.aborted) return
          debugConsole.error(error)
          setError('rendering-error')
        })
      return () => {
        abortController.abort()
      }
    }
  }, [pdfJsWrapper, url, pdfFile, setError])

  // listen for scroll events
  useEffect(() => {
    let storePositionTimer: number

    if (initialised && pdfJsWrapper) {
      if (!pdfJsWrapper.isVisible()) {
        return
      }

      // store the scroll position in localStorage, for the synctex button
      const storePosition = debounce((pdfViewer: PDFJSWrapper) => {
        // set position for "sync to code" button
        try {
          setPosition(pdfViewer.currentPosition)
        } catch {
          // the viewer has no pages yet
        }
      }, 500)

      storePositionTimer = window.setTimeout(() => {
        storePosition(pdfJsWrapper)
      }, 100)

      const scrollListener = () => {
        storePosition(pdfJsWrapper)
        setPage(pdfJsWrapper.viewer.currentPageNumber)
      }

      pdfJsWrapper.container.addEventListener('scroll', scrollListener)

      return () => {
        pdfJsWrapper.container.removeEventListener('scroll', scrollListener)
        if (storePositionTimer) {
          window.clearTimeout(storePositionTimer)
        }
        storePosition.cancel()
        try {
          setPosition(pdfJsWrapper.currentPosition)
        } catch {
          // the viewer is already gone
        }
      }
    }
  }, [setPosition, pdfJsWrapper, initialised])

  // listen for double-click events
  useEffect(() => {
    if (pdfJsWrapper) {
      const handleTextlayerrendered = (textLayer: TextLayerRenderedEvent) => {
        // handle every version of the event's shape
        const textLayerDiv = textLayer.source.div ?? textLayer.source.textLayerDiv ?? textLayer.source.textLayer?.div

        if (textLayerDiv && !textLayerDiv.dataset.listeningForDoubleClick) {
          textLayerDiv.dataset.listeningForDoubleClick = 'true'

          const doubleClickListener = (event: MouseEvent) => {
            const clickPosition = pdfJsWrapper.clickPosition(
              event,
              textLayerDiv.closest('.page')?.querySelector('canvas') ?? null,
              textLayer.pageNumber - 1
            )

            if (clickPosition) {
              window.dispatchEvent(
                new CustomEvent('synctex:sync-to-position', {
                  detail: {
                    position: clickPosition,
                    selectText: window.getSelection()?.toString(),
                  },
                })
              )
            }
          }

          textLayerDiv.addEventListener('dblclick', doubleClickListener)
        }
      }

      pdfJsWrapper.eventBus.on('textlayerrendered', handleTextlayerrendered)
      return () => pdfJsWrapper.eventBus.off('textlayerrendered', handleTextlayerrendered)
    }
  }, [pdfJsWrapper])

  const positionRef = useRef(position)
  useEffect(() => {
    positionRef.current = position
  }, [position])

  const scaleRef = useRef(scale)
  useEffect(() => {
    scaleRef.current = scale
  }, [scale])

  // restore the saved scale and scroll position
  useEffect(() => {
    if (initialised && pdfJsWrapper) {
      if (!pdfJsWrapper.isVisible()) {
        return
      }
      if (positionRef.current) {
        pdfJsWrapper.scrollToPosition(positionRef.current, scaleRef.current)
      } else {
        pdfJsWrapper.viewer.currentScaleValue = scaleRef.current
      }
    }
  }, [initialised, pdfJsWrapper, scaleRef, positionRef])

  // transmit scale value to the viewer when it changes
  useEffect(() => {
    if (pdfJsWrapper) {
      pdfJsWrapper.viewer.currentScaleValue = scale
    }
  }, [scale, pdfJsWrapper])

  // when highlights are created, build the highlight elements
  useEffect(() => {
    const timers: number[] = []
    let intersectionObserver: IntersectionObserver

    if (pdfJsWrapper && highlights?.length) {
      // watch for the highlight elements to scroll into view
      intersectionObserver = new IntersectionObserver(
        entries => {
          for (const entry of entries) {
            if (entry.isIntersecting) {
              intersectionObserver.unobserve(entry.target)

              const element = entry.target as HTMLElement

              // fade the element in and out
              element.style.opacity = '0.5'

              timers.push(
                window.setTimeout(() => {
                  element.style.opacity = '0'
                }, 1100)
              )
            }
          }
        },
        {
          threshold: 1.0, // the whole element must be visible
        }
      )

      const elements: HTMLDivElement[] = []

      for (const highlight of highlights) {
        try {
          const element = buildHighlightElement(highlight, pdfJsWrapper.viewer)
          elements.push(element)
          intersectionObserver.observe(element)
        } catch {
          // ignore invalid highlights
        }
      }

      const [firstElement] = elements

      if (firstElement) {
        // scroll to the first highlighted element
        // Briefly delay the scrolling after adding the element to the DOM.
        timers.push(
          window.setTimeout(() => {
            firstElement.scrollIntoView({
              block: 'center',
              inline: 'start',
              behavior: 'smooth',
            })
          }, 100)
        )
      }

      return () => {
        for (const timer of timers) {
          window.clearTimeout(timer)
        }
        for (const element of elements) {
          element.remove()
        }
        intersectionObserver?.disconnect()
      }
    }
  }, [highlights, pdfJsWrapper])

  // set the scale in response to zoom option changes
  const setZoom = useCallback(
    (zoom: string) => {
      switch (zoom) {
        case 'zoom-in':
          if (pdfJsWrapper) {
            setScale(`${Math.min(pdfJsWrapper.viewer.currentScale * 1.25, 9.99)}`)
          }
          break

        case 'zoom-out':
          if (pdfJsWrapper) {
            setScale(`${Math.max(pdfJsWrapper.viewer.currentScale / 1.25, 0.1)}`)
          }
          break

        default:
          setScale(zoom)
      }
    },
    [pdfJsWrapper, setScale]
  )

  // adjust the scale when the container is resized
  useEffect(() => {
    if (pdfJsWrapper && 'ResizeObserver' in window) {
      const resizeListener = throttle(() => {
        pdfJsWrapper.updateOnResize()
      }, 250)

      const resizeObserver = new ResizeObserver(resizeListener)
      resizeObserver.observe(pdfJsWrapper.container)

      window.addEventListener('resize', resizeListener)

      return () => {
        resizeObserver.disconnect()
        window.removeEventListener('resize', resizeListener)
      }
    }
  }, [pdfJsWrapper])

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (!initialised || !pdfJsWrapper) {
        return
      }
      if (event.metaKey || event.ctrlKey) {
        switch (event.key) {
          case '+':
          case '=':
            event.preventDefault()
            setZoom('zoom-in')
            pdfJsWrapper.container.focus()
            break

          case '-':
            event.preventDefault()
            setZoom('zoom-out')
            pdfJsWrapper.container.focus()
            break

          case '0':
            event.preventDefault()
            setZoom('page-width')
            pdfJsWrapper.container.focus()
            break

          case '9':
            event.preventDefault()
            setZoom('page-height')
            pdfJsWrapper.container.focus()
            break
        }
      }
    },
    [initialised, setZoom, pdfJsWrapper]
  )

  useMouseWheelZoom(pdfJsWrapper, setScale)

  const requestPresentationMode = usePresentationMode(pdfJsWrapper, page, handlePageChange, scale, setScale)

  // Don't render the toolbar until we have the necessary information
  const toolbarInfoLoaded = rawScale !== null && page !== null && totalPages !== null

  // Remove the 'region' role from each PDF page container.
  // This prevents polluting the landmark navigation menu for every page,
  // which creates a poor screen reader experience. Page navigation should be handled
  // by the toolbar controls.
  useEffect(() => {
    if (!initialised || !pdfJsWrapper) return

    const pageElements = pdfJsWrapper.container.querySelectorAll('div[data-page-number][role="region"]')
    pageElements.forEach(element => {
      element.removeAttribute('role')
    })
  }, [initialised, pdfJsWrapper])

  return (
    <div className="pdfjs-viewer pdfjs-viewer-outer" ref={handleContainer} onKeyDown={handleKeyDown} tabIndex={-1}>
      <div className="pdfjs-viewer-inner" tabIndex={0} role="tabpanel" data-testid="pdfjs-viewer-inner">
        <div className="pdfViewer" />
      </div>
      {toolbarInfoLoaded && (
        <PdfViewerControlsToolbar
          requestPresentationMode={requestPresentationMode}
          setZoom={setZoom}
          rawScale={rawScale}
          setPage={handlePageChange}
          page={page}
          totalPages={totalPages}
          pdfContainer={pdfJsWrapper?.container}
        />
      )}
    </div>
  )
}

export default withErrorBoundary(memo(PdfJsViewer), () => <PdfPreviewErrorBoundaryFallback type="pdf" />)
