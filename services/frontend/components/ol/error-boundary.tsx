'use client'

/**
 * A boundary that shows something in place of a part of the page that
 * threw, so a failure in one pane does not take the whole editor with it.
 */

import { Component, type ComponentType, type ErrorInfo, type ReactNode } from 'react'
import { useTranslation } from '@/lib/i18n'
import { Notification } from './notification'
import { debugConsole } from '@/lib/debug'

export type FallbackProps = {
  error: Error
  resetErrorBoundary: () => void
}

type Props = {
  FallbackComponent: ComponentType<FallbackProps>
  children: ReactNode
}

type State = { error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    debugConsole.error('react-error-boundary', error, errorInfo.componentStack)
  }

  resetErrorBoundary = () => {
    this.setState({ error: null })
  }

  render() {
    if (this.state.error) {
      const Fallback = this.props.FallbackComponent
      return <Fallback error={this.state.error} resetErrorBoundary={this.resetErrorBoundary} />
    }
    return this.props.children
  }
}

function DefaultFallbackComponent() {
  return <></>
}

export function withErrorBoundary<P extends object>(
  WrappedComponent: ComponentType<P>,
  FallbackComponent?: ComponentType<FallbackProps>
): ComponentType<P> {
  const Wrapped = (props: P) => (
    <ErrorBoundary FallbackComponent={FallbackComponent || DefaultFallbackComponent}>
      <WrappedComponent {...props} />
    </ErrorBoundary>
  )
  Wrapped.displayName = `withErrorBoundary(${WrappedComponent.displayName || WrappedComponent.name || 'Component'})`
  return Wrapped
}

function DefaultMessage() {
  const { t } = useTranslation()
  return (
    <>
      <p>{t('generic_something_went_wrong')}</p>
      <p>{t('please_refresh')}</p>
    </>
  )
}

export function ErrorBoundaryFallback({ children, modal }: { children?: ReactNode; modal?: ReactNode }) {
  return (
    <div className="error-boundary-alert">
      <Notification type="error" content={children || <DefaultMessage />} />
      {modal}
    </div>
  )
}
