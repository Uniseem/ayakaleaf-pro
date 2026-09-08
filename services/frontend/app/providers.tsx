'use client'

import { HeroUIProvider } from '@heroui/react'
import { useRouter } from 'next/navigation'
import type { ReactNode } from 'react'

/**
 * The one client boundary the whole app sits inside.
 *
 * HeroUI needs a provider, and it needs to know how to navigate so that a link
 * inside one of its components goes through the router rather than reloading
 * the page. Everything else stays a server component until it has a reason not
 * to be.
 */
export function Providers({ children }: { children: ReactNode }) {
  const router = useRouter()
  return <HeroUIProvider navigate={router.push}>{children}</HeroUIProvider>
}
