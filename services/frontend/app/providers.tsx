'use client'

import type { ReactNode } from 'react'

/**
 * The one client boundary the whole app sits inside.
 *
 * Nothing is wrapped around the tree any more: the components are the
 * original's own, drawn by the ported stylesheets, and none of them needs a
 * provider. Kept as a component so the layout has one place to put anything
 * that does later.
 */
export function Providers({ children }: { children: ReactNode }) {
  return <>{children}</>
}
