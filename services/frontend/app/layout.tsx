import type { Metadata } from 'next'
import type { ReactNode } from 'react'
import { Noto_Sans, DM_Mono } from 'next/font/google'
import { Providers } from './providers'
import './globals.css'

/**
 * The two typefaces the original uses, named in
 * stylesheets/abstracts/_variable-overrides.scss.
 *
 * Loaded through next/font, which self-hosts them: the font files are served
 * from this deployment rather than from Google, so a self-hosted instance does
 * not make a request to a third party on every page load, and there is no
 * flash while a remote stylesheet resolves.
 */
const sans = Noto_Sans({
  subsets: ['latin'],
  weight: ['400', '500', '600', '700'],
  variable: '--font-noto-sans',
  display: 'swap',
})

const mono = DM_Mono({
  subsets: ['latin'],
  weight: ['300', '400', '500'],
  variable: '--font-dm-mono',
  display: 'swap',
})

export const metadata: Metadata = {
  title: 'Ayakaleaf Pro',
  description: 'A self-hosted LaTeX editor.',
}

// Nothing here is cached: every page is about the person looking at it.
export const dynamic = 'force-dynamic'

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="en"
      className={`${sans.variable} ${mono.variable}`}
      suppressHydrationWarning
    >
      <body className="min-h-full">
        <Providers>{children}</Providers>
      </body>
    </html>
  )
}
