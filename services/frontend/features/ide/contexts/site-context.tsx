'use client'

/**
 * What the original passes to the page as meta tags: the things that decide
 * which parts of the editor exist on this deployment. On a self-hosted site
 * there is no support desk and no wiki, chat and link sharing are on, and
 * the symbol palette is there.
 *
 * Held in a context so that a component reads it the way the original read
 * getMeta(), and so that a deployment setting can change it in one place.
 */

import { createContext, useContext, type ReactNode } from 'react'
import type { PublicUser } from '@/lib/auth'

export type SiteValue = {
  /** The name of the site, for titles and the sharing link. */
  appName: string
  /** Where the site is reached from outside, for links that leave the page. */
  siteUrl: string
  showSupport: boolean
  wikiEnabled: boolean
  symbolPaletteAvailable: boolean
  capabilities: string[]
  /** Whether the git bridge is on, which is what puts Integrations in the rail. */
  gitBridgeEnabled: boolean
  /** Whether images can be added from a URL. */
  hasLinkUrlFeature: boolean
  /** The signed-in person. */
  user: PublicUser
}

const SiteContext = createContext<SiteValue | undefined>(undefined)

export function SiteProvider({ value, children }: { value: SiteValue; children: ReactNode }) {
  return <SiteContext.Provider value={value}>{children}</SiteContext.Provider>
}

export function useSite(): SiteValue {
  const value = useContext(SiteContext)
  if (!value) {
    throw new Error('useSite must be used inside a SiteProvider')
  }
  return value
}
