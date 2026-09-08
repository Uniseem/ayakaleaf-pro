'use client'

import { Button, Spinner, Tab, Tabs } from '@heroui/react'
import { useEffect, useState } from 'react'
import { logIn, pdfIn, type CompileResult } from '@/lib/editor'

/**
 * What came out of the last compile.
 *
 * The PDF is shown by the browser's own viewer. It is fetched from the same
 * site as this page -- nginx routes the compiler's output paths -- so nothing
 * downloads it into this process first, and a hundred-page document costs the
 * server nothing to display.
 */
export function PdfPane({
  result,
  compiling,
  error,
}: {
  result: CompileResult | null
  compiling: boolean
  error: string
}) {
  const pdf = pdfIn(result)
  const log = logIn(result)
  const failed = Boolean(result && result.status !== 'success')
  const [tab, setTab] = useState<string>('pdf')
  const [logText, setLogText] = useState<string>('')

  // Whatever came back decides what is worth looking at: a compile that made
  // no PDF is one where the log is the answer.
  useEffect(() => {
    if (result) {
      setTab(pdf ? (failed ? 'log' : 'pdf') : 'log')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [result])

  useEffect(() => {
    if (tab !== 'log' || !log) {
      return
    }
    let live = true
    fetch(log.url, { credentials: 'include' })
      .then(response => (response.ok ? response.text() : ''))
      .then(text => {
        if (live) {
          setLogText(text)
        }
      })
      .catch(() => {
        if (live) {
          setLogText('')
        }
      })
    return () => {
      live = false
    }
  }, [tab, log])

  return (
    <div className="flex h-full flex-col bg-default-50">
      <div className="flex items-center justify-between gap-2 border-b border-divider px-3 py-1.5">
        <Tabs
          size="sm"
          variant="light"
          selectedKey={tab}
          onSelectionChange={key => setTab(String(key))}
          aria-label="Compiler output"
        >
          <Tab key="pdf" title="PDF" />
          <Tab key="log" title={failed ? 'Log ●' : 'Log'} />
        </Tabs>
        {pdf ? (
          <Button
            as="a"
            href={pdf.url}
            target="_blank"
            rel="noreferrer"
            size="sm"
            variant="light"
          >
            Open
          </Button>
        ) : null}
      </div>

      <div className="relative min-h-0 flex-1">
        {compiling ? (
          <div className="absolute inset-0 z-10 flex items-center justify-center gap-2 bg-background/70 text-sm text-default-500">
            <Spinner size="sm" /> Compiling…
          </div>
        ) : null}

        {error ? (
          <div className="p-4 text-sm text-danger">{error}</div>
        ) : !result ? (
          <div className="flex h-full items-center justify-center p-6 text-center text-sm text-default-400">
            Press Compile to see the document.
          </div>
        ) : tab === 'pdf' && pdf ? (
          // The build id is part of the address, so a new compile is a new
          // src and the viewer reloads by itself.
          <iframe key={pdf.url} src={pdf.url} title="PDF" className="h-full w-full border-0" />
        ) : (
          <pre className="h-full overflow-auto whitespace-pre-wrap p-3 font-mono text-xs leading-relaxed">
            {logText ||
              result.error ||
              (failed
                ? 'The compile failed and left no log.'
                : 'This compile produced no log.')}
          </pre>
        )}
      </div>
    </div>
  )
}
