'use client'

/**
 * Which of the panel's two views is showing, from
 * review-panel/context/review-panel-view-context.
 *
 * "Current file" puts each card level with the line it belongs to, which only
 * means anything for the document that is open. "Overview" lists every file
 * with anything outstanding, which is the view for finding what is left.
 */

import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

export type SubView = 'cur_file' | 'overview'

const ReviewPanelViewContext = createContext<SubView>('cur_file')
const ReviewPanelViewActionsContext = createContext<{ setView: (view: SubView) => void }>({
  setView: () => {},
})

export function ReviewPanelViewProvider({ children }: { children: ReactNode }) {
  const [view, setView] = useState<SubView>('cur_file')

  const actions = useMemo(() => ({ setView }), [])

  return (
    <ReviewPanelViewContext.Provider value={view}>
      <ReviewPanelViewActionsContext.Provider value={actions}>
        {children}
      </ReviewPanelViewActionsContext.Provider>
    </ReviewPanelViewContext.Provider>
  )
}

export function useReviewPanelViewContext(): SubView {
  return useContext(ReviewPanelViewContext)
}

export function useReviewPanelViewActionsContext() {
  return useContext(ReviewPanelViewActionsContext)
}
