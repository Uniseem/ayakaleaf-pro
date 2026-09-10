'use client'

/**
 * The bar at the top of every page outside the editor.
 *
 * 68px tall, white, the site's name at 20px on the left and pill buttons on
 * the right -- which is what the original is, measured. The editor has its own
 * 40px toolbar instead; the two are different bars and always have been.
 */

import { Dropdown, DropdownItem, DropdownMenu, DropdownToggle } from '@/components/ol/dropdown'
import Link from 'next/link'
import { displayName, logout, type PublicUser } from '@/lib/auth'
import { ButtonLink } from './ui'

export function SiteHeader({
  user,
  siteName,
}: {
  user: PublicUser | null
  siteName: string
}) {
  async function signOut() {
    try {
      const answer = await logout()
      window.location.assign(answer.redirect)
    } catch {
      // Whatever went wrong, the person asked to leave: send them to the
      // sign-in page and let the server sort the session out.
      window.location.assign('/login')
    }
  }

  return (
    <nav className="flex h-[68px] shrink-0 items-center gap-2 border-b border-[var(--border-divider)] bg-[var(--bg-light-primary)] px-3">
      <Link
        href={user ? '/projects' : '/'}
        className="text-[20px] leading-7 text-[var(--content-primary)] hover:no-underline"
      >
        {siteName}
      </Link>

      <div className="flex-1" />

      {user ? (
        <>
          {user.isAdmin ? (
            <Dropdown align="end">
              <DropdownToggle
                bsPrefix="site-header-menu-toggle"
                className="inline-flex h-9 items-center gap-1 rounded-full px-4 text-[16px] leading-6 text-[var(--content-primary)] hover:bg-[var(--hover-interaction)]"
              >
                Admin
                <Caret />
              </DropdownToggle>
              <DropdownMenu aria-labelledby="admin-menu">
                <li role="none">
                  <DropdownItem href="/admin/settings">Site settings</DropdownItem>
                </li>
              </DropdownMenu>
            </Dropdown>
          ) : null}

          <Dropdown align="end">
            <DropdownToggle
              bsPrefix="site-header-menu-toggle"
              className="inline-flex h-9 items-center gap-1 rounded-full border-2 border-[var(--border-primary)] px-4 text-[16px] leading-6 text-[var(--content-primary)] hover:bg-[var(--hover-interaction)]"
            >
              Account
              <Caret />
            </DropdownToggle>
            <DropdownMenu aria-labelledby="account-menu">
              <li role="none">
                <span className="dropdown-header text-[14px] text-[var(--content-secondary)]">
                  {displayName(user)}
                </span>
              </li>
              <li role="none">
                <DropdownItem href="/account">Account settings</DropdownItem>
              </li>
              <li role="none">
                <DropdownItem as="button" onClick={signOut}>
                  Log out
                </DropdownItem>
              </li>
            </DropdownMenu>
          </Dropdown>
        </>
      ) : (
        <>
          <ButtonLink href="/register" kind="primary">
            Sign up
          </ButtonLink>
          <ButtonLink href="/login" kind="secondary">
            Log in
          </ButtonLink>
        </>
      )}
    </nav>
  )
}

/** The footer at the bottom of every page outside the editor. */
export function SiteFooter({ siteName }: { siteName: string }) {
  return (
    <footer className="flex shrink-0 items-center gap-4 border-t border-[var(--border-divider)] bg-[var(--bg-light-primary)] px-4 py-3 text-[12px] leading-4 text-[var(--content-secondary)]">
      <span>
        © {new Date().getFullYear()} Powered by {siteName}
      </span>
      <div className="flex-1" />
      <a
        href="https://github.com/Uniseem/ayakaleaf-pro"
        className="text-[var(--link-web)] hover:text-[var(--link-web-hover)] hover:underline"
        target="_blank"
        rel="noreferrer"
      >
        Fork on GitHub
      </a>
    </footer>
  )
}

function Caret() {
  return (
    <svg viewBox="0 0 16 16" className="h-3 w-3" fill="none" stroke="currentColor" strokeWidth="1.6" aria-hidden>
      <path d="m4 6 4 4 4-4" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
