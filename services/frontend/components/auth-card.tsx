import type { ReactNode } from 'react'
import { SiteFooter, SiteHeader } from './site-header'

/**
 * The frame the sign-in and sign-up pages share.
 *
 * The whole page, not just the box: the original puts these forms on an
 * ordinary page with the site's bar above and its footer below, and a form
 * floating alone on a white field is one of the things that made this look
 * like a different product.
 *
 * The box itself is the light grey panel, 420px wide, sitting a little above
 * the middle -- measured, not chosen.
 */
export function AuthCard({
  title,
  siteName,
  notice,
  children,
  footer,
}: {
  title: string
  siteName: string
  /** Shown as the bordered info box the original puts above the fields. */
  notice?: ReactNode
  children: ReactNode
  footer?: ReactNode
}) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader user={null} siteName={siteName} />

      <main className="flex flex-1 justify-center px-4 py-10">
        <div className="w-full max-w-[420px]">
          <div className="rounded-[8px] bg-[var(--bg-light-secondary)] p-8">
            <h1 className="mb-6 text-[24px] font-bold leading-8 text-[var(--content-primary)]">
              {title}
            </h1>
            {notice ? (
              <div className="mb-6 flex gap-3 rounded-[4px] bg-[var(--bg-light-primary)] p-4">
                <InfoIcon />
                <div className="text-[14px] leading-5 text-[var(--content-primary)]">
                  {notice}
                </div>
              </div>
            ) : null}
            {children}
          </div>
          {footer ? (
            <p className="mt-6 text-center text-[14px] leading-5 text-[var(--content-secondary)]">
              {footer}
            </p>
          ) : null}
        </div>
      </main>

      <SiteFooter siteName={siteName} />
    </div>
  )
}

function InfoIcon() {
  return (
    <svg
      viewBox="0 0 16 16"
      className="mt-0.5 h-4 w-4 shrink-0 text-[var(--content-info)]"
      fill="currentColor"
      aria-hidden
    >
      <path d="M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zm0 1.2a5.3 5.3 0 1 1 0 10.6 5.3 5.3 0 0 1 0-10.6zM8 6.7a.7.7 0 0 1 .7.7v3.4a.7.7 0 0 1-1.4 0V7.4a.7.7 0 0 1 .7-.7zm0-2.4a.8.8 0 1 1 0 1.6.8.8 0 0 1 0-1.6z" />
    </svg>
  )
}
