/**
 * What the editor knows about LaTeX.
 *
 * Enough of the language to be useful while typing: the commands worth
 * suggesting, which ones take an argument, which environments exist, and which
 * of those want a matching \end. It is not a model of LaTeX -- nothing short
 * of running TeX is -- it is the shortlist that makes completion feel like it
 * understands the document.
 */

/** A command worth offering, and what typing it should produce. */
export type Command = {
  name: string
  /** What is inserted, with # marking where the cursor lands. */
  snippet?: string
  detail?: string
  /** Ranks a suggestion; the common ones come first. */
  boost?: number
}

/** The commands that make up most of what anybody types. */
export const COMMANDS: Command[] = [
  // structure
  { name: 'documentclass', snippet: 'documentclass{#}', detail: 'document class', boost: 9 },
  { name: 'usepackage', snippet: 'usepackage{#}', detail: 'load a package', boost: 9 },
  { name: 'begin', snippet: 'begin{#}', detail: 'open an environment', boost: 9 },
  { name: 'end', snippet: 'end{#}', detail: 'close an environment' },
  { name: 'part', snippet: 'part{#}' },
  { name: 'chapter', snippet: 'chapter{#}', boost: 6 },
  { name: 'section', snippet: 'section{#}', boost: 8 },
  { name: 'subsection', snippet: 'subsection{#}', boost: 7 },
  { name: 'subsubsection', snippet: 'subsubsection{#}', boost: 5 },
  { name: 'paragraph', snippet: 'paragraph{#}' },
  { name: 'subparagraph', snippet: 'subparagraph{#}' },
  { name: 'title', snippet: 'title{#}', boost: 6 },
  { name: 'author', snippet: 'author{#}', boost: 6 },
  { name: 'date', snippet: 'date{#}' },
  { name: 'maketitle', detail: 'print the title block', boost: 5 },
  { name: 'tableofcontents', boost: 4 },
  { name: 'listoffigures' },
  { name: 'listoftables' },
  { name: 'appendix' },
  { name: 'bibliography', snippet: 'bibliography{#}' },
  { name: 'bibliographystyle', snippet: 'bibliographystyle{#}' },
  { name: 'printbibliography' },
  { name: 'addbibresource', snippet: 'addbibresource{#}' },
  { name: 'input', snippet: 'input{#}', detail: 'include a file' },
  { name: 'include', snippet: 'include{#}' },
  { name: 'includeonly', snippet: 'includeonly{#}' },

  // references
  { name: 'label', snippet: 'label{#}', detail: 'name this place', boost: 8 },
  { name: 'ref', snippet: 'ref{#}', detail: 'refer to a label', boost: 8 },
  { name: 'eqref', snippet: 'eqref{#}', detail: 'refer to an equation' },
  { name: 'pageref', snippet: 'pageref{#}' },
  { name: 'autoref', snippet: 'autoref{#}' },
  { name: 'nameref', snippet: 'nameref{#}' },
  { name: 'cref', snippet: 'cref{#}', detail: 'cleveref' },
  { name: 'Cref', snippet: 'Cref{#}' },
  { name: 'cite', snippet: 'cite{#}', detail: 'cite a work', boost: 8 },
  { name: 'citep', snippet: 'citep{#}' },
  { name: 'citet', snippet: 'citet{#}' },
  { name: 'footnote', snippet: 'footnote{#}', boost: 5 },
  { name: 'url', snippet: 'url{#}' },
  { name: 'href', snippet: 'href{#}{}' },

  // text
  { name: 'textbf', snippet: 'textbf{#}', detail: 'bold', boost: 8 },
  { name: 'textit', snippet: 'textit{#}', detail: 'italic', boost: 8 },
  { name: 'texttt', snippet: 'texttt{#}', detail: 'monospace', boost: 6 },
  { name: 'textsc', snippet: 'textsc{#}', detail: 'small caps' },
  { name: 'textsf', snippet: 'textsf{#}' },
  { name: 'textrm', snippet: 'textrm{#}' },
  { name: 'underline', snippet: 'underline{#}' },
  { name: 'emph', snippet: 'emph{#}', detail: 'emphasis', boost: 7 },
  { name: 'texorpdfstring', snippet: 'texorpdfstring{#}{}' },
  { name: 'item', detail: 'a list item', boost: 7 },
  { name: 'caption', snippet: 'caption{#}', boost: 6 },
  { name: 'centering' },
  { name: 'noindent' },
  { name: 'newline' },
  { name: 'newpage' },
  { name: 'clearpage' },
  { name: 'pagebreak' },
  { name: 'linebreak' },
  { name: 'hspace', snippet: 'hspace{#}' },
  { name: 'vspace', snippet: 'vspace{#}' },
  { name: 'hfill' },
  { name: 'vfill' },

  // figures and tables
  { name: 'includegraphics', snippet: 'includegraphics[width=#]{}', detail: 'an image', boost: 7 },
  { name: 'graphicspath', snippet: 'graphicspath{#}' },
  { name: 'hline' },
  { name: 'toprule' },
  { name: 'midrule' },
  { name: 'bottomrule' },
  { name: 'multicolumn', snippet: 'multicolumn{#}{}{}' },
  { name: 'multirow', snippet: 'multirow{#}{}{}' },

  // maths
  { name: 'frac', snippet: 'frac{#}{}', detail: 'a fraction', boost: 8 },
  { name: 'dfrac', snippet: 'dfrac{#}{}' },
  { name: 'sqrt', snippet: 'sqrt{#}', boost: 7 },
  { name: 'sum', snippet: 'sum_{#}^{}', boost: 7 },
  { name: 'prod', snippet: 'prod_{#}^{}' },
  { name: 'int', snippet: 'int_{#}^{}', boost: 6 },
  { name: 'iint', snippet: 'iint_{#}^{}' },
  { name: 'oint', snippet: 'oint_{#}^{}' },
  { name: 'lim', snippet: 'lim_{#}' },
  { name: 'infty', boost: 6 },
  { name: 'partial' },
  { name: 'nabla' },
  { name: 'cdot' },
  { name: 'cdots' },
  { name: 'ldots' },
  { name: 'times' },
  { name: 'div' },
  { name: 'pm' },
  { name: 'mp' },
  { name: 'leq' },
  { name: 'geq' },
  { name: 'neq' },
  { name: 'approx' },
  { name: 'equiv' },
  { name: 'sim' },
  { name: 'propto' },
  { name: 'in' },
  { name: 'notin' },
  { name: 'subset' },
  { name: 'subseteq' },
  { name: 'cup' },
  { name: 'cap' },
  { name: 'forall' },
  { name: 'exists' },
  { name: 'rightarrow' },
  { name: 'leftarrow' },
  { name: 'Rightarrow' },
  { name: 'Leftarrow' },
  { name: 'leftrightarrow' },
  { name: 'mapsto' },
  { name: 'mathbb', snippet: 'mathbb{#}', detail: 'blackboard bold' },
  { name: 'mathcal', snippet: 'mathcal{#}' },
  { name: 'mathbf', snippet: 'mathbf{#}' },
  { name: 'mathrm', snippet: 'mathrm{#}' },
  { name: 'mathit', snippet: 'mathit{#}' },
  { name: 'text', snippet: 'text{#}', detail: 'prose inside maths', boost: 6 },
  { name: 'left', snippet: 'left(' },
  { name: 'right', snippet: 'right)' },
  { name: 'overline', snippet: 'overline{#}' },
  { name: 'hat', snippet: 'hat{#}' },
  { name: 'bar', snippet: 'bar{#}' },
  { name: 'vec', snippet: 'vec{#}' },
  { name: 'dot', snippet: 'dot{#}' },
  { name: 'tilde', snippet: 'tilde{#}' },

  // definitions
  { name: 'newcommand', snippet: 'newcommand{#}{}', boost: 5 },
  { name: 'renewcommand', snippet: 'renewcommand{#}{}' },
  { name: 'newenvironment', snippet: 'newenvironment{#}{}{}' },
  { name: 'newtheorem', snippet: 'newtheorem{#}{}' },
  { name: 'DeclareMathOperator', snippet: 'DeclareMathOperator{#}{}' },
  { name: 'setlength', snippet: 'setlength{#}{}' },
  { name: 'def' },
]

/** Greek, which is most of what gets typed in maths. */
const GREEK = [
  'alpha', 'beta', 'gamma', 'delta', 'epsilon', 'varepsilon', 'zeta', 'eta',
  'theta', 'vartheta', 'iota', 'kappa', 'lambda', 'mu', 'nu', 'xi', 'pi',
  'varpi', 'rho', 'varrho', 'sigma', 'varsigma', 'tau', 'upsilon', 'phi',
  'varphi', 'chi', 'psi', 'omega', 'Gamma', 'Delta', 'Theta', 'Lambda', 'Xi',
  'Pi', 'Sigma', 'Upsilon', 'Phi', 'Psi', 'Omega',
]

for (const letter of GREEK) {
  COMMANDS.push({ name: letter, detail: 'greek', boost: 3 })
}

/** An environment worth offering, and what it needs inside it. */
export type Environment = {
  name: string
  /** Lines put between \begin and \end, with # marking the cursor. */
  body?: string
  detail?: string
  boost?: number
}

export const ENVIRONMENTS: Environment[] = [
  { name: 'document', detail: 'the document body', boost: 9 },
  { name: 'abstract', boost: 5 },
  { name: 'itemize', body: '  \\item #', detail: 'a bulleted list', boost: 9 },
  { name: 'enumerate', body: '  \\item #', detail: 'a numbered list', boost: 9 },
  { name: 'description', body: '  \\item[#] ' },
  { name: 'figure', body: '  \\centering\n  \\includegraphics[width=0.8\\textwidth]{#}\n  \\caption{}\n  \\label{fig:}', detail: 'a floating figure', boost: 8 },
  { name: 'table', body: '  \\centering\n  \\caption{#}\n  \\label{tab:}', boost: 8 },
  { name: 'tabular', body: '  # & \\\\', detail: 'a table body', boost: 7 },
  { name: 'tabularx', body: '  # & \\\\' },
  { name: 'equation', body: '  #', detail: 'a numbered equation', boost: 9 },
  { name: 'equation*', body: '  #' },
  { name: 'align', body: '  # &= \\\\', detail: 'aligned equations', boost: 8 },
  { name: 'align*', body: '  # &= \\\\' },
  { name: 'gather', body: '  #' },
  { name: 'multline', body: '  #' },
  { name: 'split', body: '  #' },
  { name: 'cases', body: '  # & \\text{if } \\\\' },
  { name: 'matrix', body: '  # & \\\\' },
  { name: 'pmatrix', body: '  # & \\\\' },
  { name: 'bmatrix', body: '  # & \\\\' },
  { name: 'vmatrix', body: '  # & \\\\' },
  { name: 'verbatim', body: '#', detail: 'text exactly as typed' },
  { name: 'lstlisting', body: '#', detail: 'source code' },
  { name: 'minted', body: '#' },
  { name: 'quote', body: '  #' },
  { name: 'quotation', body: '  #' },
  { name: 'center', body: '  #' },
  { name: 'flushleft', body: '  #' },
  { name: 'flushright', body: '  #' },
  { name: 'thebibliography', body: '  \\bibitem{#}' },
  { name: 'theorem', body: '  #' },
  { name: 'lemma', body: '  #' },
  { name: 'proof', body: '  #' },
  { name: 'definition', body: '  #' },
  { name: 'corollary', body: '  #' },
  { name: 'remark', body: '  #' },
  { name: 'example', body: '  #' },
  { name: 'frame', body: '  \\frametitle{#}', detail: 'a beamer slide' },
  { name: 'columns', body: '  #' },
  { name: 'subfigure', body: '  #' },
  { name: 'wrapfigure', body: '  #' },
  { name: 'algorithm', body: '  #' },
  { name: 'algorithmic', body: '  #' },
]

/** The packages worth suggesting inside \usepackage. */
export const PACKAGES = [
  'amsmath', 'amssymb', 'amsthm', 'amsfonts', 'graphicx', 'hyperref',
  'geometry', 'babel', 'inputenc', 'fontenc', 'xcolor', 'color', 'booktabs',
  'tabularx', 'longtable', 'multirow', 'array', 'caption', 'subcaption',
  'float', 'wrapfig', 'listings', 'minted', 'algorithm', 'algorithmic',
  'algpseudocode', 'natbib', 'biblatex', 'cleveref', 'enumitem', 'setspace',
  'titlesec', 'fancyhdr', 'lipsum', 'tikz', 'pgfplots', 'siunitx', 'physics',
  'mathtools', 'bm', 'microtype', 'csquotes', 'url', 'cite', 'appendix',
  'todonotes', 'soul', 'ulem', 'parskip', 'indentfirst', 'lmodern',
]

/** The document classes worth suggesting. */
export const CLASSES = [
  'article', 'report', 'book', 'letter', 'beamer', 'memoir', 'scrartcl',
  'scrreprt', 'scrbook', 'standalone', 'exam', 'moderncv', 'IEEEtran',
  'acmart', 'elsarticle', 'revtex4-2',
]

/** Environments whose contents are maths, so maths completion applies. */
export const MATH_ENVIRONMENTS = new Set([
  'equation', 'equation*', 'align', 'align*', 'gather', 'gather*',
  'multline', 'multline*', 'split', 'cases', 'matrix', 'pmatrix', 'bmatrix',
  'vmatrix', 'Vmatrix', 'smallmatrix', 'array', 'eqnarray', 'eqnarray*',
  'displaymath', 'math', 'alignat', 'alignat*', 'flalign', 'flalign*',
])

/** Environments that should not be re-indented or completed inside. */
export const VERBATIM_ENVIRONMENTS = new Set([
  'verbatim', 'Verbatim', 'lstlisting', 'minted', 'alltt', 'comment',
])

/** The commands that begin a section, deepest last. */
export const SECTION_COMMANDS = [
  'part',
  'chapter',
  'section',
  'subsection',
  'subsubsection',
  'paragraph',
  'subparagraph',
]
