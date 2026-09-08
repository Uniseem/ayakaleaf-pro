import type { Metadata } from 'next'
import type { ReactNode } from 'react'
import { Providers } from './providers'
import './globals.css'

export const metadata: Metadata = {
  title: 'Ayakaleaf Pro',
  description: 'A self-hosted LaTeX editor.',
}

// Nothing here is cached: every page is about the person looking at it.
export const dynamic = 'force-dynamic'

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="min-h-full">
        <Providers>{children}</Providers>
      </body>
    </html>
  )
}
