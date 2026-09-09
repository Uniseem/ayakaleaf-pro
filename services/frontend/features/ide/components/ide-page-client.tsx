'use client'

/**
 * The editor page, rendered in the browser only.
 *
 * The IDE is built from what the browser remembers -- which file was open,
 * how the panels were sized, which rail tab was chosen -- and none of that is
 * known on the server. Rendering it there produces a page that disagrees with
 * the one the browser builds, and React refuses a hydration that disagrees.
 * The original drew the editor client-side too; the server's job here is
 * fetching the project so the first render already has it.
 */

import dynamic from 'next/dynamic'
import { FullSizeLoadingSpinner } from '@/components/ol/spinner'
import type { IdePageProps } from './ide-page'

const IdePage = dynamic(() => import('./ide-page').then(module => module.IdePage), {
  ssr: false,
  loading: () => (
    <div className="ide-loading-screen">
      <FullSizeLoadingSpinner delay={500} />
    </div>
  ),
})

export function IdePageClient(props: IdePageProps) {
  return <IdePage {...props} />
}
