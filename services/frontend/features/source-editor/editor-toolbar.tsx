'use client'

/**
 * The row of formatting buttons above the text.
 *
 * 32px, white, no border -- measured from the original, whose button set this
 * matches: undo and redo, the heading level, bold and italic, then the things
 * that are awkward to type from memory. That last group is the reason a LaTeX
 * editor has a toolbar at all: nobody needs a button for \textbf, but a
 * correctly-formed figure environment with a caption and a label is six lines
 * that are easy to get subtly wrong.
 *
 * Every button dispatches the same events the menu bar does, so there is one
 * place that knows how to put text into the document.
 */

import {
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Tooltip,
} from '@heroui/react'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useSettings } from '@/features/ide/contexts/settings-context'

const insert = (snippet: string) =>
  window.dispatchEvent(new CustomEvent('ide:insert', { detail: { snippet } }))

const wrap = (snippet: string) =>
  window.dispatchEvent(new CustomEvent('ide:wrap', { detail: { snippet } }))

const command = (name: string) =>
  window.dispatchEvent(new CustomEvent(`ide:${name}`))

const FIGURE =
  '\\begin{figure}[h]\n  \\centering\n  \\includegraphics[width=0.8\\textwidth]{}\n  \\caption{}\n  \\label{fig:}\n\\end{figure}\n'
const TABLE =
  '\\begin{table}[h]\n  \\centering\n  \\begin{tabular}{ll}\n    \\hline\n     &  \\\\\n    \\hline\n  \\end{tabular}\n  \\caption{}\n  \\label{tab:}\n\\end{table}\n'
const LIST = '\\begin{itemize}\n  \\item \n\\end{itemize}\n'

export function EditorToolbar() {
  const { editable } = useEditor()
  const settings = useSettings()

  return (
    <div className="flex h-8 shrink-0 items-center gap-0.5 bg-[var(--bg-light-primary)] px-2">
      <ToolButton label="Undo" onClick={() => command('undo')} disabled={!editable}>
        <UndoIcon />
      </ToolButton>
      <ToolButton label="Redo" onClick={() => command('redo')} disabled={!editable}>
        <RedoIcon />
      </ToolButton>

      <Divider />

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button
            type="button"
            aria-label="Section heading level"
            disabled={!editable}
            className="flex h-6 items-center gap-1 rounded-[4px] px-1.5 text-[14px] leading-5 text-[var(--content-primary)] hover:bg-[var(--hover-interaction)] disabled:opacity-40"
          >
            <HeadingIcon />
            <Caret />
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Section heading level"
          onAction={key => wrap(`\\${String(key)}{}`)}
        >
          <DropdownItem key="section">Section</DropdownItem>
          <DropdownItem key="subsection">Subsection</DropdownItem>
          <DropdownItem key="subsubsection">Subsubsection</DropdownItem>
          <DropdownItem key="paragraph">Paragraph</DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <ToolButton label="Bold" onClick={() => wrap('\\textbf{}')} disabled={!editable}>
        <span className="text-[14px] font-bold leading-5">B</span>
      </ToolButton>
      <ToolButton label="Italic" onClick={() => wrap('\\textit{}')} disabled={!editable}>
        <span className="text-[14px] italic leading-5">I</span>
      </ToolButton>

      <Divider />

      <ToolButton label="Insert math" onClick={() => wrap('$$')} disabled={!editable}>
        <span className="text-[14px] leading-5">∑</span>
      </ToolButton>

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button
            type="button"
            aria-label="Insert symbol"
            disabled={!editable}
            className="flex h-6 w-6 items-center justify-center rounded-[4px] text-[14px] leading-5 text-[var(--content-primary)] hover:bg-[var(--hover-interaction)] disabled:opacity-40"
          >
            Ω
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Insert symbol"
          onAction={key => insert(`\\${String(key)}`)}
          className="max-h-64 overflow-auto"
        >
          {SYMBOLS.map(symbol => (
            <DropdownItem key={symbol.name} startContent={<span className="w-5 text-center">{symbol.glyph}</span>}>
              {'\\' + symbol.name}
            </DropdownItem>
          ))}
        </DropdownMenu>
      </Dropdown>

      <ToolButton label="Insert link" onClick={() => insert('\\href{}{}')} disabled={!editable}>
        <LinkIcon />
      </ToolButton>

      <Divider />

      <ToolButton
        label="Insert cross reference"
        onClick={() => insert('\\ref{}')}
        disabled={!editable}
      >
        <RefIcon />
      </ToolButton>
      <ToolButton
        label="Insert citation"
        onClick={() => insert('\\cite{}')}
        disabled={!editable}
      >
        <CiteIcon />
      </ToolButton>
      <ToolButton label="Insert figure" onClick={() => insert(FIGURE)} disabled={!editable}>
        <FigureIcon />
      </ToolButton>
      <ToolButton label="Insert table" onClick={() => insert(TABLE)} disabled={!editable}>
        <TableIcon />
      </ToolButton>
      <ToolButton label="Insert list" onClick={() => insert(LIST)} disabled={!editable}>
        <ListIcon />
      </ToolButton>

      <div className="flex-1" />

      {/* Source or rendered. A pair of segments rather than a menu, because it
          is a switch between two things and both should be readable without
          opening anything. */}
      <div
        role="group"
        aria-label="How the document is shown"
        className="mr-1 flex items-center rounded-full bg-[var(--bg-light-secondary)] p-0.5"
      >
        {(['code', 'visual'] as const).map(mode => (
          <button
            key={mode}
            type="button"
            aria-pressed={settings.mode === mode}
            onClick={() => settings.set('mode', mode)}
            className={`h-5 rounded-full px-2.5 text-[12px] leading-4 ${
              settings.mode === mode
                ? 'bg-[var(--bg-light-primary)] font-semibold text-[var(--content-primary)] shadow-sm'
                : 'text-[var(--content-secondary)]'
            }`}
          >
            {mode === 'code' ? 'Code' : 'Visual'}
          </button>
        ))}
      </div>

      <ToolButton label="Search file" onClick={() => command('find')}>
        <SearchIcon />
      </ToolButton>
    </div>
  )
}

/** The symbols worth a menu, which is the ones that are hard to remember. */
const SYMBOLS = [
  { name: 'alpha', glyph: 'α' },
  { name: 'beta', glyph: 'β' },
  { name: 'gamma', glyph: 'γ' },
  { name: 'delta', glyph: 'δ' },
  { name: 'epsilon', glyph: 'ε' },
  { name: 'theta', glyph: 'θ' },
  { name: 'lambda', glyph: 'λ' },
  { name: 'mu', glyph: 'μ' },
  { name: 'pi', glyph: 'π' },
  { name: 'sigma', glyph: 'σ' },
  { name: 'phi', glyph: 'φ' },
  { name: 'omega', glyph: 'ω' },
  { name: 'Delta', glyph: 'Δ' },
  { name: 'Sigma', glyph: 'Σ' },
  { name: 'Omega', glyph: 'Ω' },
  { name: 'infty', glyph: '∞' },
  { name: 'partial', glyph: '∂' },
  { name: 'nabla', glyph: '∇' },
  { name: 'times', glyph: '×' },
  { name: 'leq', glyph: '≤' },
  { name: 'geq', glyph: '≥' },
  { name: 'neq', glyph: '≠' },
  { name: 'approx', glyph: '≈' },
  { name: 'rightarrow', glyph: '→' },
  { name: 'in', glyph: '∈' },
  { name: 'forall', glyph: '∀' },
  { name: 'exists', glyph: '∃' },
]

function ToolButton({
  label,
  onClick,
  disabled,
  children,
}: {
  label: string
  onClick: () => void
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <Tooltip content={label} delay={500} closeDelay={0}>
      <button
        type="button"
        aria-label={label}
        onClick={onClick}
        disabled={disabled}
        className="flex h-6 w-6 items-center justify-center rounded-[4px] text-[var(--content-primary)] hover:bg-[var(--hover-interaction)] disabled:opacity-40 disabled:hover:bg-transparent"
      >
        {children}
      </button>
    </Tooltip>
  )
}

function Divider() {
  return <span className="mx-1 h-4 w-px bg-[var(--border-divider)]" aria-hidden />
}

const line = {
  viewBox: '0 0 16 16',
  className: 'h-4 w-4',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.4,
  'aria-hidden': true,
} as const

function UndoIcon() {
  return (
    <svg {...line}>
      <path d="M3 7h7a3 3 0 1 1 0 6H7" strokeLinecap="round" />
      <path d="M5.5 4.5 3 7l2.5 2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function RedoIcon() {
  return (
    <svg {...line}>
      <path d="M13 7H6a3 3 0 1 0 0 6h3" strokeLinecap="round" />
      <path d="M10.5 4.5 13 7l-2.5 2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function HeadingIcon() {
  return (
    <svg {...line}>
      <path d="M3.5 3v10M10.5 3v10M3.5 8h7" strokeLinecap="round" />
    </svg>
  )
}

function LinkIcon() {
  return (
    <svg {...line}>
      <path d="M6.5 9.5a2.5 2.5 0 0 0 3.5 0l2-2a2.5 2.5 0 0 0-3.5-3.5l-.8.8" strokeLinecap="round" />
      <path d="M9.5 6.5a2.5 2.5 0 0 0-3.5 0l-2 2a2.5 2.5 0 0 0 3.5 3.5l.8-.8" strokeLinecap="round" />
    </svg>
  )
}

function RefIcon() {
  return (
    <svg {...line}>
      <path d="M3 4.5h10M3 8h6M3 11.5h4" strokeLinecap="round" />
      <path d="M11 9.5l2 2 2-3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function CiteIcon() {
  return (
    <svg {...line}>
      <path d="M6 4.5c-1.5 0-2.5 1-2.5 2.5S4.5 9.5 6 9.5c0 1.2-.8 2-2 2.3" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M12.5 4.5c-1.5 0-2.5 1-2.5 2.5s1 2.5 2.5 2.5c0 1.2-.8 2-2 2.3" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function FigureIcon() {
  return (
    <svg {...line}>
      <rect x="2" y="3" width="12" height="10" rx="1.5" />
      <circle cx="5.75" cy="6.5" r="1" />
      <path d="M2.5 11.5 6 8.5l2.5 2 2-1.5 3 2.5" strokeLinejoin="round" />
    </svg>
  )
}

function TableIcon() {
  return (
    <svg {...line}>
      <rect x="2" y="3" width="12" height="10" rx="1.5" />
      <path d="M2 6.5h12M2 10h12M6.5 3v10" />
    </svg>
  )
}

function ListIcon() {
  return (
    <svg {...line}>
      <path d="M6 4.5h8M6 8h8M6 11.5h8" strokeLinecap="round" />
      <circle cx="3.2" cy="4.5" r="0.9" fill="currentColor" stroke="none" />
      <circle cx="3.2" cy="8" r="0.9" fill="currentColor" stroke="none" />
      <circle cx="3.2" cy="11.5" r="0.9" fill="currentColor" stroke="none" />
    </svg>
  )
}

function SearchIcon() {
  return (
    <svg {...line}>
      <circle cx="7" cy="7" r="4.2" />
      <path d="m10.2 10.2 3.3 3.3" strokeLinecap="round" />
    </svg>
  )
}

function Caret() {
  return (
    <svg viewBox="0 0 16 16" className="h-3 w-3" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <path d="m4 6 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
