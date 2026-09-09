'use client'

/**
 * The File / Edit / View / Insert / Format / Help menus.
 *
 * A menu bar rather than a settings drawer, because that is what the original
 * grew into and because it is where somebody looks for "download this as a
 * PDF" or "how do I make a table". Every entry here does something; there are
 * no placeholders, which is why the lists are shorter than a word processor's.
 */

import {
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownSection,
  DropdownTrigger,
} from '@heroui/react'
import { useCompile } from '@/features/ide/contexts/compile-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useLayout } from '@/features/ide/contexts/layout-context'
import { useProject } from '@/features/ide/contexts/project-context'
import { useSettings } from '@/features/ide/contexts/settings-context'

export type MenuBarActions = {
  onShare: () => void
  onWordCount: () => void
  onNewFile: () => void
  onUpload: () => void
}

export function MenuBar(actions: MenuBarActions) {
  const project = useProject()
  const compile = useCompile()
  const editor = useEditor()
  const layout = useLayout()
  const settings = useSettings()

  const item =
    'flex h-7 items-center rounded-[4px] px-2 text-[14px] leading-5 text-[var(--content-primary)] hover:bg-[var(--hover-interaction)]'

  /** Puts text into the document at the cursor, for the Insert menu. */
  const insert = (snippet: string) => {
    window.dispatchEvent(
      new CustomEvent('ide:insert', { detail: { snippet } })
    )
  }

  return (
    <div className="flex items-center gap-0.5">
      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button type="button" className={item}>
            File
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="File"
          disabledKeys={project.canWrite ? [] : ['new', 'upload']}
          onAction={key => {
            switch (key) {
              case 'new':
                actions.onNewFile()
                break
              case 'upload':
                actions.onUpload()
                break
              case 'pdf':
                if (compile.pdfUrl) {
                  window.open(compile.pdfUrl, '_blank', 'noopener')
                }
                break
              case 'projects':
                window.location.assign('/projects')
                break
            }
          }}
        >
          <DropdownSection showDivider>
            <DropdownItem key="new">New file</DropdownItem>
            <DropdownItem key="upload">Upload file</DropdownItem>
          </DropdownSection>
          <DropdownSection showDivider>
            <DropdownItem key="pdf" description={compile.pdfUrl ? undefined : 'Compile first'}>
              Download PDF
            </DropdownItem>
          </DropdownSection>
          <DropdownItem key="projects">Back to projects</DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button type="button" className={item}>
            Edit
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Edit"
          onAction={key => window.dispatchEvent(new CustomEvent(`ide:${String(key)}`))}
        >
          <DropdownItem key="undo" shortcut="Ctrl+Z">
            Undo
          </DropdownItem>
          <DropdownItem key="redo" shortcut="Ctrl+Y">
            Redo
          </DropdownItem>
          <DropdownItem key="find" shortcut="Ctrl+F">
            Find
          </DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button type="button" className={item}>
            View
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="View"
          closeOnSelect={false}
          onAction={key => {
            switch (key) {
              case 'layout':
                layout.changeLayout(
                  layout.pdfLayout === 'sideBySide' ? 'flat' : 'sideBySide'
                )
                break
              case 'outline':
                settings.set('showOutline', !settings.showOutline)
                break
              case 'theme':
                settings.set('overallTheme', settings.overallTheme === 'dark' ? 'light' : 'dark')
                break
              case 'history':
                layout.setView('history')
                break
              case 'logs':
                layout.toggleLogs()
                break
            }
          }}
        >
          <DropdownItem key="layout">
            {layout.pdfLayout === 'sideBySide' ? 'Show one pane' : 'Show side by side'}
          </DropdownItem>
          <DropdownItem key="outline">
            {settings.showOutline ? 'Hide outline' : 'Show outline'}
          </DropdownItem>
          <DropdownItem key="logs">
            {layout.showLogs ? 'Hide log' : 'Show log'}
          </DropdownItem>
          <DropdownItem key="theme">
            {settings.overallTheme === 'dark' ? 'Light theme' : 'Dark theme'}
          </DropdownItem>
          <DropdownItem key="history">Project history</DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button type="button" className={item}>
            Insert
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Insert"
          disabledKeys={editor.editable ? [] : ['figure', 'table', 'equation', 'list', 'cite', 'ref']}
          onAction={key => {
            const snippets: Record<string, string> = {
              figure:
                '\\begin{figure}[h]\n  \\centering\n  \\includegraphics[width=0.8\\textwidth]{}\n  \\caption{}\n  \\label{fig:}\n\\end{figure}\n',
              table:
                '\\begin{table}[h]\n  \\centering\n  \\begin{tabular}{ll}\n    \\hline\n     &  \\\\\n    \\hline\n  \\end{tabular}\n  \\caption{}\n  \\label{tab:}\n\\end{table}\n',
              equation: '\\begin{equation}\n  \n\\end{equation}\n',
              list: '\\begin{itemize}\n  \\item \n\\end{itemize}\n',
              cite: '\\cite{}',
              ref: '\\ref{}',
            }
            const snippet = snippets[String(key)]
            if (snippet) {
              insert(snippet)
            }
          }}
        >
          <DropdownItem key="figure">Figure</DropdownItem>
          <DropdownItem key="table">Table</DropdownItem>
          <DropdownItem key="equation">Equation</DropdownItem>
          <DropdownItem key="list">List</DropdownItem>
          <DropdownItem key="cite">Citation</DropdownItem>
          <DropdownItem key="ref">Cross-reference</DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button type="button" className={item}>
            Format
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Format"
          disabledKeys={editor.editable ? [] : ['bold', 'italic', 'code', 'section', 'subsection']}
          onAction={key => {
            const wraps: Record<string, string> = {
              bold: '\\textbf{}',
              italic: '\\textit{}',
              code: '\\texttt{}',
              section: '\\section{}',
              subsection: '\\subsection{}',
            }
            const snippet = wraps[String(key)]
            if (snippet) {
              window.dispatchEvent(
                new CustomEvent('ide:wrap', { detail: { snippet } })
              )
            }
          }}
        >
          <DropdownItem key="bold" shortcut="Ctrl+B">
            Bold
          </DropdownItem>
          <DropdownItem key="italic" shortcut="Ctrl+I">
            Italic
          </DropdownItem>
          <DropdownItem key="code">Monospace</DropdownItem>
          <DropdownItem key="section">Section</DropdownItem>
          <DropdownItem key="subsection">Subsection</DropdownItem>
        </DropdownMenu>
      </Dropdown>

      <Dropdown placement="bottom-start">
        <DropdownTrigger>
          <button type="button" className={item}>
            Help
          </button>
        </DropdownTrigger>
        <DropdownMenu
          aria-label="Help"
          onAction={key => {
            if (key === 'wordcount') {
              actions.onWordCount()
            } else if (key === 'share') {
              actions.onShare()
            } else if (key === 'account') {
              window.location.assign('/account')
            }
          }}
        >
          <DropdownItem key="wordcount">Word count</DropdownItem>
          <DropdownItem key="share">Share this project</DropdownItem>
          <DropdownItem key="account">Account settings</DropdownItem>
        </DropdownMenu>
      </Dropdown>
    </div>
  )
}
