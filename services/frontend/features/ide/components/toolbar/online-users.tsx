'use client'

/**
 * Who else is here, from ide-react/components/toolbar/online-users.tsx and
 * editor-navigation-toolbar/components/online-users-widget.tsx.
 *
 * One coloured circle per person, overlapping, five at most; the rest go in
 * a "+n" menu. Clicking somebody goes to where their cursor is.
 */

import { useCallback, useMemo } from 'react'
import { useTranslation } from '@/lib/i18n'
import { Tooltip } from '@/components/ol/tooltip'
import { Dropdown, DropdownHeader, DropdownItem, DropdownMenu, DropdownToggle } from '@/components/ol/dropdown'
import { firstCharacter, getBackgroundColorForUserId, hslStringToLuminance } from '@/lib/colors'
import { useConnection, type PresentUser } from '@/features/ide/contexts/connection-context'
import { useEditor } from '@/features/ide/contexts/editor-context'
import { useProject } from '@/features/ide/contexts/project-context'

const MAX_USER_CIRCLES_DISPLAYED = 5
const MAX_USERS_WITH_OVERFLOW_VISIBLE = MAX_USER_CIRCLES_DISPLAYED - 1

export function OnlineUsers() {
  const { others } = useConnection()
  const editor = useEditor()
  const { entryById } = useProject()

  const goToUser = useCallback(
    (user: PresentUser) => {
      if (user.docId && typeof user.row === 'number') {
        const entry = entryById(user.docId)
        if (entry) {
          editor.open(entry)
          window.dispatchEvent(new CustomEvent('ide:goto-line', { detail: { docId: user.docId, line: user.row + 1 } }))
        }
      }
    },
    [editor, entryById]
  )

  return (
    <div className="ide-redesign-online-users">
      <OnlineUsersWidget onlineUsers={others} goToUser={goToUser} />
    </div>
  )
}

export function OnlineUsersWidget({ onlineUsers, goToUser }: { onlineUsers: PresentUser[]; goToUser: (user: PresentUser) => void }) {
  const hasOverflow = onlineUsers.length > MAX_USER_CIRCLES_DISPLAYED
  const usersBeforeOverflow = useMemo(
    () => (hasOverflow ? onlineUsers.slice(0, MAX_USERS_WITH_OVERFLOW_VISIBLE) : onlineUsers),
    [onlineUsers, hasOverflow]
  )
  const usersInOverflow = useMemo(() => (hasOverflow ? onlineUsers.slice(MAX_USERS_WITH_OVERFLOW_VISIBLE) : []), [onlineUsers, hasOverflow])

  return (
    <div className="online-users-row">
      {usersBeforeOverflow.map((user, index) => (
        <OnlineUserWidget key={`${user.id}_${index}`} user={user} goToUser={goToUser} id={`online-user-${user.id}_${index}`} />
      ))}
      {hasOverflow ? <OnlineUserOverflow goToUser={goToUser} users={usersInOverflow} /> : null}
    </div>
  )
}

function OnlineUserWidget({ user, goToUser, id }: { user: PresentUser; goToUser: (user: PresentUser) => void; id: string }) {
  const onClick = useCallback(() => goToUser(user), [goToUser, user])
  return (
    <Tooltip id={id} description={user.name} overlayProps={{ placement: 'bottom', trigger: ['hover', 'focus'], delay: 0 }}>
      <button type="button" className="online-users-row-button" onClick={onClick}>
        <OnlineUserCircle user={user} />
      </button>
    </Tooltip>
  )
}

function OnlineUserCircle({ user }: { user: PresentUser }) {
  const backgroundColor = getBackgroundColorForUserId(user.id)
  const luminance = hslStringToLuminance(backgroundColor)
  const character = firstCharacter(user.name)
  return (
    <span
      className={['online-user-circle', luminance < 0.5 ? 'online-user-circle-light-font' : 'online-user-circle-dark-font'].join(' ')}
      style={{ backgroundColor }}
    >
      {character}
    </span>
  )
}

function OnlineUserOverflow({ goToUser, users }: { goToUser: (user: PresentUser) => void; users: PresentUser[] }) {
  const { t } = useTranslation()
  return (
    <Dropdown align="end">
      <DropdownToggle className="online-users-row-button online-user-overflow-toggle" bsPrefix="dropdown-toggle">
        <Tooltip id="connected-users" description={t('n_more_collaborators', { count: users.length })} overlayProps={{ placement: 'bottom' }}>
          <span className="online-user-circle">+{users.length}</span>
        </Tooltip>
      </DropdownToggle>
      <DropdownMenu className="online-user-overflow-dropdown">
        <DropdownHeader aria-hidden="true">{t('connected_users')}</DropdownHeader>
        {users.map((user, index) => (
          <li role="none" key={`${user.id}_${index}`}>
            <DropdownItem as="button" tabIndex={-1} onClick={() => goToUser(user)}>
              <OnlineUserCircle user={user} /> {user.name}
            </DropdownItem>
          </li>
        ))}
      </DropdownMenu>
    </Dropdown>
  )
}
