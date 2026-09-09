'use client'

/**
 * How long a project is.
 *
 * The counting is done by texcount in the compiler, not here. It reads the
 * files the way TeX does -- following \input, skipping the preamble, not
 * counting a command name as a word -- and doing that in the browser would
 * mean reimplementing it against the same edge cases, worse.
 *
 * The consequence is that a project has to have been compiled at least once,
 * because texcount reads the compile directory. The empty answer says so
 * rather than showing zero, which would look like a result.
 */

import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Spinner,
} from '@heroui/react'
import { useCallback, useEffect, useState } from 'react'
import { api, messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'

type Counts = Record<string, unknown>

/** The rows worth showing, in the order they make sense in. */
const ROWS: Array<{ key: string; label: string }> = [
  { key: 'textWords', label: 'Words in the text' },
  { key: 'headWords', label: 'Words in headings' },
  { key: 'outsideWords', label: 'Words outside the text' },
  { key: 'headers', label: 'Headings' },
  { key: 'mathInline', label: 'Inline formulae' },
  { key: 'mathDisplay', label: 'Displayed formulae' },
  { key: 'elements', label: 'Figures and tables' },
]

export function WordCountModal({
  isOpen,
  onClose,
}: {
  isOpen: boolean
  onClose: () => void
}) {
  const { projectId } = useProject()
  const [counts, setCounts] = useState<Counts | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const answer = await api<{ counts: Counts }>(
        `/api/projects/${projectId}/wordcount`
      )
      setCounts(answer.counts)
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    if (isOpen) {
      void load()
    }
  }, [isOpen, load])

  const total = numberOf(counts?.textWords) + numberOf(counts?.headWords)

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="sm">
      <ModalContent>
        <ModalHeader>Word count</ModalHeader>
        <ModalBody>
          {loading ? (
            <div className="flex justify-center py-6">
              <Spinner size="sm" />
            </div>
          ) : error ? (
            <p className="text-sm text-danger">{error}</p>
          ) : !counts || Object.keys(counts).length === 0 ? (
            <p className="text-sm text-default-500">
              Nothing to count yet. This is read from the last compile, so
              compile the project first.
            </p>
          ) : (
            <>
              <p className="text-2xl font-semibold">
                {total.toLocaleString()}{' '}
                <span className="text-sm font-normal text-default-500">
                  words
                </span>
              </p>
              <dl className="mt-2 divide-y divide-divider text-sm">
                {ROWS.filter(row => counts[row.key] !== undefined).map(row => (
                  <div key={row.key} className="flex justify-between py-1.5">
                    <dt className="text-default-500">{row.label}</dt>
                    <dd>{numberOf(counts[row.key]).toLocaleString()}</dd>
                  </div>
                ))}
              </dl>
            </>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            Close
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}

function numberOf(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0
}
