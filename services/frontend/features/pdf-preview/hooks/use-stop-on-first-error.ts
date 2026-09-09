import { useCallback } from 'react'
import { useCompile } from '@/features/ide/contexts/compile-context'

export function useStopOnFirstError() {
  const { setStopOnFirstError } = useCompile()

  const enableStopOnFirstError = useCallback(() => {
    setStopOnFirstError(true)
  }, [setStopOnFirstError])

  const disableStopOnFirstError = useCallback(() => {
    setStopOnFirstError(false)
  }, [setStopOnFirstError])

  return { enableStopOnFirstError, disableStopOnFirstError }
}
