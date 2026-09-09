import type { OutputFile } from '@/lib/editor'
import type { PdfFileData, PdfFileDataList } from './types'

const topFileTypes = ['bbl', 'gls', 'ind']
// The compiler's bookkeeping files, of no use to anybody downloading output.
const ignoreFiles = ['output.fls', 'output.fdb_latexmk']

const typeOf = (file: OutputFile) => file.type ?? file.path.split('.').pop() ?? ''

/** The output files as the logs pane offers them: the notable ones first. */
export function buildFileList(outputFiles: Map<string, OutputFile>): PdfFileDataList {
  const files: PdfFileDataList = { top: [], other: [] }

  const allFiles: PdfFileData[] = []

  // filter out ignored files and set some properties
  for (const file of outputFiles.values()) {
    if (!ignoreFiles.includes(file.path)) {
      allFiles.push({
        ...file,
        type: typeOf(file),
        main: file.path.startsWith('output.'),
        downloadURL: file.url,
      })
    }
  }

  // sort main files first, then alphabetical
  allFiles.sort((a, b) => {
    if (a.main && !b.main) {
      return -1
    }

    if (b.main && !a.main) {
      return 1
    }

    return a.path.localeCompare(b.path, undefined, { numeric: true })
  })

  // group files into "top" and "other"
  for (const file of allFiles) {
    if (topFileTypes.includes(file.type ?? '')) {
      files.top.push(file)
    } else if (!(file.type === 'pdf' && file.main === true)) {
      files.other.push(file)
    }
  }

  return files
}
