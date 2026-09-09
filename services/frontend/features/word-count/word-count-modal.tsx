'use client'

/**
 * How long a project is, from word-count-modal.
 *
 * The counting is done by texcount in the compiler, which is why a project
 * has to have been compiled first.
 */

import { memo, useEffect, useState } from 'react'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { Notification } from '@/components/ol/notification'
import { LoadingSpinner } from '@/components/ol/spinner'
import { api } from '@/lib/api'
import { useTranslation } from '@/lib/i18n'
import { useProject } from '@/features/ide/contexts/project-context'

export type ServerWordCountData = {
  encode?: string
  textWords: number
  headWords: number
  outside: number
  headers: number
  elements: number
  mathInline: number
  mathDisplay: number
  errors: number
  messages: string
}

export const WordCountModal = memo(function WordCountModal({ show, handleHide }: { show: boolean; handleHide: () => void }) {
  return (
    <OLModal animation show={show} onHide={handleHide} id="word-count-modal">
      {show ? <WordCountModalContent handleHide={handleHide} /> : null}
    </OLModal>
  )
})

function WordCountModalContent({ handleHide }: { handleHide: () => void }) {
  const { t } = useTranslation()
  return (
    <>
      <OLModalHeader>
        <OLModalTitle>{t('word_count_lower')}</OLModalTitle>
      </OLModalHeader>
      <OLModalBody className="ol-ui">
        <WordCountServer />
      </OLModalBody>
      <OLModalFooter>
        <Button variant="secondary" onClick={handleHide}>
          {t('close')}
        </Button>
      </OLModalFooter>
    </>
  )
}

function WordCountServer() {
  const { projectId } = useProject()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [data, setData] = useState<ServerWordCountData | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    api<{ counts?: Partial<ServerWordCountData>; texcount?: Partial<ServerWordCountData> }>(`/api/projects/${projectId}/wordcount`, {
      signal: controller.signal,
    })
      .then(answer => {
        const counts = answer.texcount ?? answer.counts ?? {}
        setData({
          textWords: counts.textWords ?? 0,
          headWords: counts.headWords ?? 0,
          outside: counts.outside ?? 0,
          headers: counts.headers ?? 0,
          elements: counts.elements ?? 0,
          mathInline: counts.mathInline ?? 0,
          mathDisplay: counts.mathDisplay ?? 0,
          errors: counts.errors ?? 0,
          messages: counts.messages ?? '',
        })
      })
      .catch(() => setError(true))
      .finally(() => setLoading(false))
    return () => controller.abort()
  }, [projectId])

  return (
    <>
      {loading && !error ? <LoadingSpinner /> : null}
      {error ? <WordCountError /> : null}
      {data ? <WordCounts data={data} /> : null}
    </>
  )
}

function WordCountError() {
  const { t } = useTranslation()
  return <Notification type="error" content={t('generic_something_went_wrong')} />
}

function WordCounts({ data }: { data: ServerWordCountData }) {
  const { t } = useTranslation()
  return (
    <div className="container-fluid">
      {data.messages ? (
        <div className="row">
          <div className="col-12">
            <Notification type="error" content={<p style={{ whiteSpace: 'pre-wrap' }}>{data.messages}</p>} />
          </div>
        </div>
      ) : null}
      <div className="row">
        <div className="col-4">
          <div className="float-end">{t('total_words')}:</div>
        </div>
        <div className="col-6">{data.textWords}</div>
      </div>
      <div className="row">
        <div className="col-4">
          <div className="float-end">{t('headers')}:</div>
        </div>
        <div className="col-6">{data.headers}</div>
      </div>
      <div className="row">
        <div className="col-4">
          <div className="float-end">{t('math_inline')}:</div>
        </div>
        <div className="col-6">{data.mathInline}</div>
      </div>
      <div className="row">
        <div className="col-4">
          <div className="float-end">{t('math_display')}:</div>
        </div>
        <div className="col-6">{data.mathDisplay}</div>
      </div>
    </div>
  )
}

export default WordCountModal
