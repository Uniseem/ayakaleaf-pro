import { parseLatexLog, type ParsedLog, type LogEntry as ParsedEntry } from '../log-parser'
import ruleset from './rules'
import type { LogEntry } from '../util/types'

type ParserOptions = Parameters<typeof parseLatexLog>[1]

export type HumanReadableLogResult = {
  all: LogEntry[]
  errors: LogEntry[]
  warnings: LogEntry[]
  typesetting: LogEntry[]
}

/**
 * The compiler's log, read and then explained: each entry is matched against
 * the rules, which give it a stable id for its hint, a clearer title where
 * one is known, and the command to highlight in the editor. Errors known to
 * cascade from an earlier one are dropped.
 */
export default {
  parse(rawLog: string | ParsedLog, options: ParserOptions): HumanReadableLogResult {
    const parsedLogEntries = typeof rawLog === 'string' ? parseLatexLog(rawLog, options) : rawLog

    const seenErrorTypes: Record<string, boolean> = {} // keep track of types of errors seen

    // The parsed entries are extended in place: the lists share their objects.
    const entries = parsedLogEntries.all as (ParsedEntry & Partial<LogEntry>)[]

    for (const entry of entries) {
      const ruleDetails = ruleset.find(rule => rule.regexToMatch.test(entry.message))

      if (ruleDetails) {
        if (ruleDetails.ruleId) {
          entry.ruleId = ruleDetails.ruleId
        }

        if (ruleDetails.newMessage) {
          entry.message = entry.message.replace(ruleDetails.regexToMatch, ruleDetails.newMessage)
        }

        if (ruleDetails.contentRegex) {
          if (entry.content != null) {
            const match = entry.content.match(ruleDetails.contentRegex)
            if (match) {
              entry.contentDetails = match.slice(1)
            }
          }
        }

        if (entry.contentDetails && ruleDetails.improvedTitle) {
          const message = ruleDetails.improvedTitle(entry.message, entry.contentDetails)

          if (Array.isArray(message)) {
            entry.message = message[0]
          } else {
            entry.message = message
          }
        }

        if (entry.contentDetails && ruleDetails.highlightCommand) {
          entry.command = ruleDetails.highlightCommand(entry.contentDetails)
        }

        // suppress any entries that are known to cascade from previous error types
        if (ruleDetails.cascadesFrom) {
          for (const type of ruleDetails.cascadesFrom) {
            if (seenErrorTypes[type]) {
              entry.suppressed = true
            }
          }
        }

        // record the types of errors seen
        if (ruleDetails.types) {
          for (const type of ruleDetails.types) {
            seenErrorTypes[type] = true
          }
        }
      }
    }

    const keep = (list: ParsedEntry[]) => (list as (ParsedEntry & Partial<LogEntry>)[]).filter(entry => !entry.suppressed)

    return {
      all: keep(parsedLogEntries.all) as LogEntry[],
      errors: keep(parsedLogEntries.errors) as LogEntry[],
      warnings: keep(parsedLogEntries.warnings) as LogEntry[],
      typesetting: keep(parsedLogEntries.typesetting) as LogEntry[],
    }
  },
}
