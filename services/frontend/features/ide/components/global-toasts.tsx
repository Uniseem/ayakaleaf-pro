'use client'

/**
 * The toasts anything in the editor can raise by name, from
 * ide-react/components/global-toasts: an `ide:show-toast` event names a
 * generator, which makes the toast; a handle lets the sender take it back.
 */

import { Fragment, memo, useCallback, useState, type ReactElement } from 'react'
import { OLToast, OLToastContainer, type OLToastProps } from '@/components/ol/toast'
import { useEventListener } from '@/lib/hooks'
import { debugConsole } from '@/lib/debug'
import synctexToastGenerators from '@/features/pdf-preview/components/synctex-toasts'

export type GlobalToastGeneratorEntry = {
  key: string
  generator: GlobalToastGenerator
}

type GlobalToastGenerator = (args: Record<string, unknown>) => Omit<OLToastProps, 'onDismiss'>

const GENERATOR_LIST: GlobalToastGeneratorEntry[] = [...synctexToastGenerators]
const GENERATOR_MAP: Map<string, GlobalToastGenerator> = new Map(GENERATOR_LIST.map(({ key, generator }) => [key, generator]))

let toastCounter = 1

export const GlobalToasts = memo(function GlobalToasts() {
  const [toasts, setToasts] = useState<{ component: ReactElement; id: string; handle?: string }[]>([])

  const removeToast = useCallback((id: string) => {
    setToasts(current => current.filter(toast => toast.id !== id))
  }, [])

  const createToast = useCallback(
    (id: string, key: string, data: Record<string, unknown>): ReactElement | null => {
      const generator = GENERATOR_MAP.get(key)
      if (!generator) {
        debugConsole.error('No toast generator found for key:', key)
        return null
      }

      const props = generator(data)

      if (!props.autoHide && !props.isDismissible) {
        // no toast that neither goes away by itself nor can be dismissed
        props.isDismissible = true
      }
      if (props.autoHide && !props.isDismissible && props.delay !== undefined) {
        // a toast that cannot be dismissed must not hang around too long
        props.delay = Math.min(props.delay, 60_000)
      }

      return <OLToast {...props} onDismiss={() => removeToast(id)} />
    },
    [removeToast]
  )

  const addToast = useCallback(
    (key: string, handle?: string, data: Record<string, unknown> = {}) => {
      const id = `toast-${toastCounter++}`
      const component = createToast(id, key, data)
      if (!component) {
        return
      }
      setToasts(current => [...current, { component, id, handle }])
    },
    [createToast]
  )

  const removeToastByHandle = useCallback((handle: string) => {
    setToasts(current => current.filter(toast => toast.handle !== handle))
  }, [])

  useEventListener(
    'ide:show-toast' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const { key, handle, data } = (event as CustomEvent<{ key: string; handle?: string; data?: Record<string, unknown> }>).detail
        addToast(key, handle, data)
      },
      [addToast]
    )
  )

  useEventListener(
    'ide:remove-toast' as keyof WindowEventMap,
    useCallback(
      (event: Event) => {
        const { handle } = (event as CustomEvent<{ handle: string }>).detail
        removeToastByHandle(handle)
      },
      [removeToastByHandle]
    )
  )

  return (
    <OLToastContainer className="global-toasts">
      {toasts.map(({ component, id }) => (
        <Fragment key={id}>{component}</Fragment>
      ))}
    </OLToastContainer>
  )
})
