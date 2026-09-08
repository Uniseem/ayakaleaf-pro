'use client'

import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Navbar,
  NavbarBrand,
  NavbarContent,
  NavbarItem,
} from '@heroui/react'
import Link from 'next/link'
import { displayName, logout, type PublicUser } from '@/lib/auth'

/**
 * The bar at the top of every signed-in page.
 *
 * The administrator's entries are here rather than on an admin page, because
 * somebody who needs them is usually in the middle of something else.
 */
export function SiteHeader({ user, siteName }: { user: PublicUser; siteName: string }) {
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
    <Navbar maxWidth="xl" isBordered>
      <NavbarBrand>
        <Link href="/projects" className="font-semibold tracking-tight">
          {siteName}
        </Link>
      </NavbarBrand>

      <NavbarContent justify="end">
        {user.isAdmin ? (
          <NavbarItem className="hidden sm:flex">
            <Button as={Link} href="/admin/settings" variant="light" size="sm">
              Settings
            </Button>
          </NavbarItem>
        ) : null}
        <NavbarItem>
          <Dropdown placement="bottom-end">
            <DropdownTrigger>
              <Button variant="light" size="sm">
                {displayName(user)}
              </Button>
            </DropdownTrigger>
            <DropdownMenu aria-label="Account">
              <DropdownItem key="account" href="/account">
                Account
              </DropdownItem>
              {user.isAdmin ? (
                <DropdownItem key="settings" href="/admin/settings">
                  Site settings
                </DropdownItem>
              ) : null}
              <DropdownItem key="logout" color="danger" onPress={signOut}>
                Sign out
              </DropdownItem>
            </DropdownMenu>
          </Dropdown>
        </NavbarItem>
      </NavbarContent>
    </Navbar>
  )
}
