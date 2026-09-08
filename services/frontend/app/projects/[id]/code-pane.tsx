'use client'

import { StreamLanguage } from '@codemirror/language'
import { stex } from '@codemirror/legacy-modes/mode/stex'
import { EditorView } from '@codemirror/view'
import CodeMirror from '@uiw/react-codemirror'
import { useMemo } from 'react'

/**
 * The text.
 *
 * CodeMirror is loaded here and nowhere else, so the rest of the editor holds
 * a string and knows nothing about it. Swapping the surface later is a change
 * to this file.
 */
export function CodePane({
  value,
  onChange,
  readOnly,
  busy,
}: {
  value: string
  onChange: (next: string) => void
  readOnly: boolean
  busy: boolean
}) {
  const extensions = useMemo(
    () => [
      // LaTeX is a stream mode rather than a full grammar. It is what the
      // editor this replaces used too, and it is enough for the thing that
      // matters at this size: commands, maths and comments told apart.
      StreamLanguage.define(stex),
      EditorView.lineWrapping,
    ],
    []
  )

  return (
    <div className="relative h-full">
      {busy ? (
        <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 text-sm text-default-500">
          Loading…
        </div>
      ) : null}
      <CodeMirror
        value={value}
        height="100%"
        style={{ height: '100%', fontSize: '13px' }}
        extensions={extensions}
        editable={!readOnly}
        readOnly={readOnly}
        onChange={onChange}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: true,
          bracketMatching: true,
          closeBrackets: true,
          autocompletion: false,
          foldGutter: false,
        }}
      />
    </div>
  )
}
