'use client'

/**
 * Bootstrap's border spinner, and the "Loading…" row built on it, from
 * shared/components/ol/ol-spinner.tsx and loading-spinner.tsx.
 */

import { useEffect, useState } from 'react'
import { useTranslation } from '@/lib/i18n'

export type SpinnerSize = 'sm' | 'lg'

export function Spinner({
  size = 'sm',
  className,
}: {
  size?: SpinnerSize
  className?: string
}) {
  return (
    <div
      className={[
        'spinner-border',
        size === 'sm' ? 'spinner-border-sm' : '',
        className ?? '',
      ]
        .filter(Boolean)
        .join(' ')}
      role="status"
      aria-hidden="true"
      data-testid="ol-spinner"
    />
  )
}

export function LoadingSpinner({
  align,
  delay = 0,
  loadingText,
  size = 'sm',
  className,
}: {
  align?: 'left' | 'center'
  delay?: 0 | 500
  loadingText?: string
  size?: SpinnerSize
  className?: string
}) {
  const { t } = useTranslation()
  const [show, setShow] = useState(delay === 0)

  useEffect(() => {
    if (delay === 0) {
      setShow(true)
      return
    }
    const timer = window.setTimeout(() => setShow(true), delay)
    return () => window.clearTimeout(timer)
  }, [delay])

  if (!show) {
    return null
  }

  return (
    <div
      role="status"
      className={[
        'loading',
        className ?? '',
        align === 'left' ? 'align-items-start' : 'align-items-center',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <Spinner size={size} />
      {loadingText || `${t('loading')}…`}
    </div>
  )
}

export function FullSizeLoadingSpinner({
  delay = 0,
  minHeight,
  loadingText,
  size = 'sm',
  className,
}: {
  delay?: 0 | 500
  minHeight?: string
  loadingText?: string
  size?: SpinnerSize
  className?: string
}) {
  return (
    <div
      className={['full-size-loading-spinner-container', className ?? '']
        .filter(Boolean)
        .join(' ')}
      style={{ minHeight }}
    >
      <LoadingSpinner size={size} loadingText={loadingText} delay={delay} />
    </div>
  )
}
