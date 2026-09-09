'use client'

/**
 * Sharing a project, from features/share-project-modal.
 *
 * The owner adds people by email with a role, turns link sharing on and
 * off, and changes or removes what each person may do; everybody else sees
 * who has access. Errors and the in-flight state are kept in one place so
 * that any row can report them.
 */

import React, { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation, Trans } from '@/lib/i18n'
import { ApiError } from '@/lib/api'
import {
  getSharing,
  getTokens,
  invite as sendInviteRequest,
  removeMember as removeMemberRequest,
  resendInvite as resendInviteRequest,
  revokeInvite as revokeInviteRequest,
  setPrivilege,
  setPublicAccess,
  transferOwnership,
  type Invite as InviteData,
  type Member,
  type Privilege,
  type PublicAccessLevel,
  type Tokens,
} from '@/lib/sharing'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { Button } from '@/components/ol/button'
import { Spinner, FullSizeLoadingSpinner } from '@/components/ol/spinner'
import { Notification } from '@/components/ol/notification'
import MaterialIcon from '@/components/ol/material-icon'
import { Tooltip } from '@/components/ol/tooltip'
import { Select } from '@/components/ol/select'
import { Tag } from '@/components/ol/badge'
import { OLFormGroup, OLFormLabel, OLFormText } from '@/components/ol/form-control'
import { CopyToClipboard } from '@/components/ol/misc'
import { useProject } from '@/features/ide/contexts/project-context'
import { useSite } from '@/features/ide/contexts/site-context'

type PermissionsLevel = 'owner' | 'readAndWrite' | 'review' | 'readOnly'

type ShareProjectContextValue = {
  monitorRequest: <T>(request: () => Promise<T>) => Promise<T>
  inFlight: boolean
  setInFlight: React.Dispatch<React.SetStateAction<boolean>>
  error: string | undefined
  setError: React.Dispatch<React.SetStateAction<string | undefined>>
  members: Member[]
  invites: InviteData[]
  publicAccessLevel: PublicAccessLevel
  updateProject: (patch: { members?: Member[]; invites?: InviteData[]; publicAccessLevel?: PublicAccessLevel }) => void
  reload: () => Promise<void>
}

const ShareProjectContext = createContext<ShareProjectContextValue | undefined>(undefined)

function useShareProjectContext() {
  const context = useContext(ShareProjectContext)
  if (!context) {
    throw new Error('useShareProjectContext is only available inside ShareProjectProvider')
  }
  return context
}

function errorKey(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 429) {
      return 'too_many_requests'
    }
    if (error.code === 'cannot_invite_self' || error.code === 'cannot_invite_non_user' || error.code === 'invalid_email') {
      return error.code
    }
    return error.message || 'generic_something_went_wrong'
  }
  return 'generic_something_went_wrong'
}

export const ShareProjectModal = React.memo(function ShareProjectModal({
  handleHide,
  show,
  animation = true,
}: {
  handleHide: () => void
  show: boolean
  handleOpen: () => void
  animation?: boolean
}) {
  const { t } = useTranslation()
  const { projectId } = useProject()
  const [inFlight, setInFlight] = useState(false)
  const [error, setError] = useState<string | undefined>()
  const [members, setMembers] = useState<Member[]>([])
  const [invites, setInvites] = useState<InviteData[]>([])
  const [publicAccessLevel, setPublicAccessLevel] = useState<PublicAccessLevel>('private')
  const [loaded, setLoaded] = useState(false)

  const reload = useCallback(async () => {
    const sharing = await getSharing(projectId)
    setMembers(sharing.members)
    setInvites(sharing.invites)
    setPublicAccessLevel(sharing.publicAccess || 'private')
    setLoaded(true)
  }, [projectId])

  useEffect(() => {
    if (show) {
      setError(undefined)
      reload().catch(thrown => setError(errorKey(thrown)))
    }
  }, [show, reload])

  const cancel = useCallback(() => {
    if (!inFlight) {
      handleHide()
    }
  }, [handleHide, inFlight])

  const monitorRequest = useCallback(<T,>(request: () => Promise<T>) => {
    setError(undefined)
    setInFlight(true)
    const promise = request()
    promise.catch((thrown: unknown) => setError(errorKey(thrown)))
    promise.finally(() => setInFlight(false))
    return promise
  }, [])

  const updateProject = useCallback((patch: { members?: Member[]; invites?: InviteData[]; publicAccessLevel?: PublicAccessLevel }) => {
    if (patch.members) setMembers(patch.members)
    if (patch.invites) setInvites(patch.invites)
    if (patch.publicAccessLevel) setPublicAccessLevel(patch.publicAccessLevel)
  }, [])

  const value = useMemo(
    () => ({ monitorRequest, inFlight, setInFlight, error, setError, members, invites, publicAccessLevel, updateProject, reload }),
    [monitorRequest, inFlight, error, members, invites, publicAccessLevel, updateProject, reload]
  )

  return (
    <ShareProjectContext.Provider value={value}>
      <OLModal show={show} onHide={cancel} animation={animation}>
        <OLModalHeader>
          <div className="d-flex flex-grow-1 justify-content-between">
            <OLModalTitle>{t('share_project')}</OLModalTitle>
          </div>
        </OLModalHeader>

        <OLModalBody className="modal-body-share modal-link-share ol-ui">
          <div className="container-fluid">
            {loaded ? <ShareModalBody /> : <FullSizeLoadingSpinner minHeight="15rem" />}
            {error ? <Notification type="error" content={<ErrorMessage error={error} />} className="mb-0 mt-3" /> : null}
          </div>
        </OLModalBody>

        <OLModalFooter>
          <div className="d-flex flex-grow-1 flex-wrap gap-2">{inFlight ? <Spinner size="sm" /> : null}</div>
          <Button variant="secondary" onClick={cancel} disabled={inFlight}>
            {t('close')}
          </Button>
        </OLModalFooter>
      </OLModal>
    </ShareProjectContext.Provider>
  )
})

function ShareModalBody() {
  const { isOwner, isRestrictedTokenMember, features } = useProject()
  const { members, invites } = useShareProjectContext()

  const canAddCollaborators = useMemo(() => {
    if (!isOwner) return false
    if (features.collaborators === -1) return true
    const editorInvites = invites.filter(invite => invite.privilege !== 'readOnly').length
    return members.filter(member => !member.owner && member.privilege !== 'readOnly').length + editorInvites < (features.collaborators ?? 1)
  }, [members, invites, features, isOwner])

  const hasExceededCollaboratorLimit = useMemo(() => {
    if (!isOwner) return false
    if (features.collaborators === -1) return false
    return members.filter(member => !member.owner && member.privilege !== 'readOnly').length > (features.collaborators ?? 1)
  }, [features, isOwner, members])

  const sortedMembers = useMemo(() => {
    const collaborators = members.filter(member => !member.owner)
    return [
      ...collaborators.filter(member => member.privilege === 'readAndWrite'),
      ...collaborators.filter(member => member.privilege === 'review'),
      ...collaborators.filter(member => member.privilege !== 'readAndWrite' && member.privilege !== 'review'),
    ]
  }, [members])

  if (isRestrictedTokenMember) {
    return <ReadOnlyTokenLink />
  }

  return (
    <>
      {isOwner ? <SendInvites canAddCollaborators={canAddCollaborators} /> : <SendInvitesNotice />}

      {isOwner ? <LinkSharing /> : null}

      <OwnerInfo />

      {sortedMembers.map(member =>
        isOwner ? (
          <EditMember
            key={member.id}
            member={member}
            hasExceededCollaboratorLimit={hasExceededCollaboratorLimit}
            canAddCollaborators={canAddCollaborators}
            isReviewerOnFreeProject={member.privilege === 'review' && !features.trackChanges}
          />
        ) : (
          <ViewMember key={member.id} member={member} />
        )
      )}

      {invites.map(invite => (
        <InviteRow key={invite.id} invite={invite} isProjectOwner={isOwner} />
      ))}
    </>
  )
}

function ErrorMessage({ error }: { error: string }) {
  const { t } = useTranslation()
  switch (error) {
    case 'cannot_invite_non_user':
      return <>{t('cannot_invite_non_user')}</>
    case 'cannot_verify_user_not_robot':
      return <>{t('cannot_verify_user_not_robot')}</>
    case 'cannot_invite_self':
      return <>{t('cannot_invite_self')}</>
    case 'invalid_email':
      return <>{t('invalid_email')}</>
    case 'too_many_requests':
      return <>{t('too_many_requests')}</>
    case 'invite_expired':
      return <>{t('invite_expired')}</>
    case 'invite_resend_limit_hit':
      return <>{t('invite_resend_limit_hit')}</>
    case 'generic_something_went_wrong':
      return <>{t('generic_something_went_wrong')}</>
    default:
      return <>{error}</>
  }
}

/* Inviting ---------------------------------------------------------------- */

function SendInvites({ canAddCollaborators }: { canAddCollaborators: boolean }) {
  return (
    <div className="row invite-controls">
      <AddCollaborators readOnly={!canAddCollaborators} />
    </div>
  )
}

function SendInvitesNotice() {
  const { t } = useTranslation()
  const { publicAccessLevel } = useShareProjectContext()

  let accessLevelText = ''
  if (publicAccessLevel === 'private') {
    accessLevelText = t('to_add_more_collaborators')
  } else if (publicAccessLevel === 'tokenBased') {
    accessLevelText = t('to_change_access_permissions')
  }

  return (
    <div>
      {accessLevelText ? (
        <div className="row public-access-level public-access-level-notice">
          <div className="col text-center">
            <span>{accessLevelText}</span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

type ContactItem = { email: string; display: string; type: string }

const matchAllSpaces = /[\u061C\u2000-\u200F\u202A-\u202E\u2060\u2066-\u2069\u2028\u2029\u202F]/g

function isValidEmail(email: string) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
}

function AddCollaborators({ readOnly }: { readOnly?: boolean }) {
  const { t } = useTranslation()
  const { members } = useShareProjectContext()
  const [selectedItems, setSelectedItems] = useState<ContactItem[]>([])

  const currentMemberEmails = useMemo(() => members.map(member => member.email.toLowerCase()).sort(), [members])

  const addSelectedItem = useCallback((item: ContactItem) => {
    setSelectedItems(items => (items.some(existing => existing.email === item.email) ? items : [...items, item]))
  }, [])
  const removeSelectedItem = useCallback((item: ContactItem) => {
    setSelectedItems(items => items.filter(existing => existing.email !== item.email))
  }, [])
  const reset = useCallback(() => setSelectedItems([]), [])

  return (
    <form className="add-collabs" onSubmit={event => event.preventDefault()}>
      <OLFormGroup>
        <SelectCollaborators
          selectedItems={selectedItems}
          addSelectedItem={addSelectedItem}
          removeSelectedItem={removeSelectedItem}
          currentMemberEmails={currentMemberEmails}
          readOnly={readOnly}
        />
        <OLFormText id="add-collaborator-help-text">{t('add_comma_separated_emails_help')}</OLFormText>
      </OLFormGroup>
      <OLFormGroup>
        <div className="float-end add-collaborator-controls add-collaborator-controls-legacy">
          <AddCollaboratorsSelect readOnly={readOnly} selectedItems={selectedItems} reset={reset} currentMemberEmails={currentMemberEmails} />
        </div>
      </OLFormGroup>
    </form>
  )
}

function SelectCollaborators({
  selectedItems,
  addSelectedItem,
  removeSelectedItem,
  currentMemberEmails,
  readOnly,
}: {
  selectedItems: ContactItem[]
  addSelectedItem: (item: ContactItem) => void
  removeSelectedItem: (item: ContactItem) => void
  currentMemberEmails: string[]
  readOnly?: boolean
}) {
  const { t } = useTranslation()
  const [inputValue, setInputValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const selectedEmails = useMemo(() => selectedItems.map(item => item.email), [selectedItems])

  const focusInput = useCallback(() => {
    window.setTimeout(() => inputRef.current?.focus(), 10)
  }, [])

  const isValidInput = useMemo(() => {
    if (inputValue.includes('@')) {
      for (const selectedItem of selectedItems) {
        if (selectedItem.email === inputValue) {
          return false
        }
      }
    }
    return true
  }, [inputValue, selectedItems])

  const addNewItem = useCallback(
    (raw: string, focus = true) => {
      const email = raw.replace(matchAllSpaces, '')
      if (isValidInput && email.includes('@') && !selectedEmails.includes(email)) {
        addSelectedItem({ email, display: email, type: 'user' })
        setInputValue('')
        if (focus) {
          focusInput()
        }
        return true
      }
      return false
    },
    [addSelectedItem, selectedEmails, isValidInput, focusInput]
  )

  void currentMemberEmails

  return (
    <div className="tags-input tags-new">
      <OLFormLabel className="small" htmlFor="collaborator-email-input">
        {t('add_email_address')}
      </OLFormLabel>

      <div className="row align-items-start">
        <div className="col">
          <div className="host">
            {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
            <div className={['tags form-control', !isValidInput ? 'is-invalid' : ''].filter(Boolean).join(' ')} onClick={focusInput}>
              {selectedItems.map((selectedItem, index) => (
                <Tag
                  key={`selected-item-${index}`}
                  prepend={<MaterialIcon type="person" />}
                  closeBtnProps={{
                    onClick: event => {
                      event.preventDefault()
                      event.stopPropagation()
                      removeSelectedItem(selectedItem)
                      focusInput()
                    },
                  }}
                  translate="no"
                >
                  {selectedItem.display}
                </Tag>
              ))}

              <input
                id="collaborator-email-input"
                data-testid="collaborator-email-input"
                aria-describedby="add-collaborator-help-text"
                className={['input', !isValidInput ? 'invalid-tag is-invalid' : ''].filter(Boolean).join(' ')}
                type="email"
                size={inputValue.length ? inputValue.length + 5 : 5}
                ref={inputRef}
                value={inputValue}
                disabled={readOnly}
                onBlur={() => addNewItem(inputValue, false)}
                onChange={event => setInputValue(event.target.value)}
                onKeyDown={event => {
                  switch (event.key) {
                    case 'Enter':
                      event.preventDefault()
                      event.stopPropagation()
                      addNewItem(inputValue)
                      break
                    case 'Tab':
                      if (addNewItem(inputValue)) {
                        event.preventDefault()
                        event.stopPropagation()
                      }
                      break
                    case ',':
                      event.preventDefault()
                      addNewItem(inputValue)
                      break
                  }
                }}
                onPaste={event => {
                  const data = event.clipboardData?.getData('text/plain')
                  if (data) {
                    const emails = data
                      .split(/[\r\n,; ]+/)
                      .filter(item => item.includes('@'))
                      .map(email => email.replace(matchAllSpaces, ''))
                    if (emails.length) {
                      event.preventDefault()
                      const uniqueEmails = [...new Set(emails)].filter(email => !selectedEmails.includes(email))
                      for (const email of uniqueEmails) {
                        addNewItem(email)
                      }
                    }
                  }
                }}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function AddCollaboratorsSelect({
  readOnly,
  selectedItems,
  reset,
  currentMemberEmails,
}: {
  readOnly?: boolean
  selectedItems: ContactItem[]
  reset: () => void
  currentMemberEmails: string[]
}) {
  const { t } = useTranslation()
  const { features, projectId } = useProject()
  const [privileges, setPrivileges] = useState<Privilege>('readAndWrite')
  const { setInFlight, setError, members, invites, updateProject } = useShareProjectContext()

  const privilegeOptions = useMemo(() => {
    const options: { key: Privilege; label: string; description?: string | null }[] = [{ key: 'readAndWrite', label: t('editor') }]
    if (features.trackChangesVisible) {
      options.push({
        key: 'review',
        label: t('reviewer'),
        description: !features.trackChanges ? t('comment_only_upgrade_for_track_changes') : null,
      })
    }
    options.push({ key: 'readOnly', label: t('viewer') })
    return options
  }, [features.trackChanges, features.trackChangesVisible, t])

  useEffect(() => {
    if (readOnly && privileges !== 'readOnly') {
      setPrivileges('readOnly')
    }
  }, [privileges, readOnly])

  const handleSubmit = useCallback(async () => {
    if (!selectedItems.length) {
      return
    }
    reset()
    setError(undefined)
    setInFlight(true)

    let currentMembers = members
    let currentInvites = invites

    for (const contact of selectedItems) {
      const email = contact.type === 'user' ? contact.email : contact.display
      const normalisedEmail = email.toLowerCase()
      if (currentMemberEmails.includes(normalisedEmail)) {
        continue
      }
      try {
        const existing = currentInvites.find(invite => invite.email === normalisedEmail)
        if (existing) {
          await resendInviteRequest(projectId, existing.id)
        } else {
          const data = await sendInviteRequest(projectId, email, privileges)
          if (data.error) {
            setError(data.error)
            setInFlight(false)
            return
          }
          if (data.invite) {
            currentInvites = [...currentInvites, data.invite]
            updateProject({ invites: currentInvites })
          } else if (data.member) {
            currentMembers = [...currentMembers, data.member]
            updateProject({ members: currentMembers })
          }
        }
      } catch (thrown) {
        setInFlight(false)
        setError(errorKey(thrown))
        return
      }
      await new Promise(resolve => setTimeout(resolve, 100))
    }
    setInFlight(false)
  }, [currentMemberEmails, invites, members, privileges, projectId, reset, selectedItems, setError, setInFlight, updateProject])

  return (
    <>
      <Select
        dataTestId="add-collaborator-select"
        items={privilegeOptions}
        itemToKey={item => item.key}
        itemToString={item => item?.label || ''}
        itemToSubtitle={item => item?.description || ''}
        itemToDisabled={item => Boolean(readOnly && item?.key !== 'readOnly')}
        selected={privilegeOptions.find(option => option.key === privileges)}
        onSelectedItemChanged={item => {
          if (item) {
            setPrivileges(item.key)
          }
        }}
      />
      <Button onClick={handleSubmit} variant="primary">
        {t('invite')}
      </Button>
    </>
  )
}

/* Link sharing ------------------------------------------------------------ */

function useProjectTokens() {
  const { projectId } = useProject()
  const [tokens, setTokens] = useState<Tokens | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    getTokens(projectId, controller.signal)
      .then(setTokens)
      .catch(() => {})
    return () => controller.abort()
  }, [projectId])

  return tokens
}

function LinkSharing() {
  const { t } = useTranslation()
  const site = useSite()
  const [inflight, setInflight] = useState(false)
  const [showLinks, setShowLinks] = useState(true)
  const linkSharingEnabled = site.capabilities.includes('link-sharing')
  const { monitorRequest, publicAccessLevel, updateProject } = useShareProjectContext()
  const { projectId } = useProject()

  const setAccessLevel = useCallback(
    (level: 'private' | 'tokenBased') => {
      setInflight(true)
      monitorRequest(() => setPublicAccess(projectId, level))
        .then(() => updateProject({ publicAccessLevel: level }))
        .finally(() => setInflight(false))
    },
    [monitorRequest, projectId, updateProject]
  )

  if (!linkSharingEnabled) {
    return null
  }

  switch (publicAccessLevel) {
    case 'private':
      return (
        <div className="row public-access-level">
          <div className="col-12 text-center">
            <strong>{t('link_sharing_is_off_short')}</strong>
            <span>&nbsp;&nbsp;</span>
            <button
              type="button"
              className="btn btn-link btn-inline-link"
              onClick={() => {
                setAccessLevel('tokenBased')
                setShowLinks(true)
              }}
              disabled={inflight}
            >
              {t('turn_on_link_sharing')}
            </button>
            <span>&nbsp;&nbsp;</span>
            <LinkSharingInfo />
          </div>
        </div>
      )

    case 'tokenBased':
      return <TokenBasedSharing setAccessLevel={setAccessLevel} inflight={inflight} setShowLinks={setShowLinks} showLinks={showLinks} />

    case 'readAndWrite':
    case 'readOnly':
      return (
        <div className="row public-access-level">
          <div className="col-12 text-center">
            <strong>{publicAccessLevel === 'readAndWrite' ? t('this_project_is_public') : t('this_project_is_public_read_only')}</strong>
            <span>&nbsp;&nbsp;</span>
            <button type="button" className="btn btn-link btn-inline-link" onClick={() => setAccessLevel('private')} disabled={inflight}>
              {t('make_private')}
            </button>
            <span>&nbsp;&nbsp;</span>
            <LinkSharingInfo />
          </div>
        </div>
      )

    default:
      return null
  }
}

function TokenBasedSharing({
  setAccessLevel,
  inflight,
  setShowLinks,
  showLinks,
}: {
  setAccessLevel: (level: 'private' | 'tokenBased') => void
  inflight: boolean
  setShowLinks: (show: boolean) => void
  showLinks: boolean
}) {
  const { t } = useTranslation()
  const tokens = useProjectTokens()

  return (
    <div className="row public-access-level">
      <div className="col-12 text-center">
        <strong>{t('link_sharing_is_on')}</strong>
        <span>&nbsp;&nbsp;</span>
        <button type="button" className="btn btn-link btn-inline-link" onClick={() => setAccessLevel('private')} disabled={inflight}>
          {t('turn_off_link_sharing')}
        </button>
        <span>&nbsp;&nbsp;</span>
        <LinkSharingInfo />
        <button type="button" className="btn btn-link btn-chevron align-middle" onClick={() => setShowLinks(!showLinks)}>
          <MaterialIcon type={showLinks ? 'keyboard_arrow_up' : 'keyboard_arrow_down'} />
        </button>
      </div>
      {showLinks ? (
        <div className="col-12 access-token-display-area">
          <AccessTokenEditDisplayArea tokens={tokens} />
          <AccessTokenViewDisplayArea tokens={tokens} />
        </div>
      ) : null}
    </div>
  )
}

function ReadOnlyTokenLink() {
  const tokens = useProjectTokens()
  return (
    <div className="row public-access-level">
      <div className="col access-token-display-area">
        <AccessTokenViewDisplayArea tokens={tokens} />
      </div>
    </div>
  )
}

function AccessToken({ token, tokenHashPrefix, path, tooltipId }: { token?: string; tokenHashPrefix?: string; path: string; tooltipId: string }) {
  const { t } = useTranslation()
  const site = useSite()

  if (!token) {
    return (
      <pre className="access-token">
        <span>{t('loading')}…</span>
      </pre>
    )
  }

  let origin = window.location.origin
  if (site.user.isAdmin && site.siteUrl) {
    origin = site.siteUrl
  }
  const link = `${origin}${path}${token}${tokenHashPrefix ? `#${tokenHashPrefix}` : ''}`

  return (
    <div className="access-token">
      <code>{link}</code>
      <CopyToClipboard content={link} tooltipId={tooltipId} kind="button" />
    </div>
  )
}

function LinkSharingInfo() {
  const { t } = useTranslation()
  return (
    <Tooltip id="link-sharing-info" description={t('learn_more_about_link_sharing')}>
      <a href="/learn/how-to/What_is_Link_Sharing%3F" target="_blank" rel="noopener">
        <MaterialIcon type="help" className="align-middle" />
      </a>
    </Tooltip>
  )
}

function AccessTokenEditDisplayArea({ tokens }: { tokens: Tokens | null }) {
  const { t } = useTranslation()
  return (
    <div className="access-token-wrapper">
      <strong className="access-token-wrapper-title">{t('anyone_with_link_can_edit')}</strong>
      <AccessToken token={tokens?.readAndWrite} tokenHashPrefix={tokens?.readAndWriteHashPrefix} path="/" tooltipId="tooltip-copy-link-rw" />
    </div>
  )
}

function AccessTokenViewDisplayArea({ tokens }: { tokens: Tokens | null }) {
  const { t } = useTranslation()
  return (
    <div className="access-token-wrapper">
      <strong className="access-token-wrapper-title">{t('anyone_with_link_can_view')}</strong>
      <AccessToken token={tokens?.readOnly} tokenHashPrefix={tokens?.readOnlyHashPrefix} path="/read/" tooltipId="tooltip-copy-link-ro" />
    </div>
  )
}

/* People ------------------------------------------------------------------ */

function OwnerInfo() {
  const { t } = useTranslation()
  const { members } = useShareProjectContext()
  const owner = members.find(member => member.owner)
  return (
    <div className="row project-member">
      <div className="col-8">
        <div className="project-member-email-icon">
          <MaterialIcon type="person" />
          <div className="email-warning">{owner?.email}</div>
        </div>
      </div>
      <div className="col-4 text-end">{t('owner')}</div>
    </div>
  )
}

function MemberPrivileges({ privileges }: { privileges: Privilege }) {
  const { t } = useTranslation()
  switch (privileges) {
    case 'readAndWrite':
      return <>{t('editor')}</>
    case 'readOnly':
      return <>{t('viewer')}</>
    case 'review':
      return <>{t('reviewer')}</>
    default:
      return null
  }
}

function ViewMember({ member }: { member: Member }) {
  return (
    <div className="row project-member">
      <div className="col-8">
        <div className="project-member-email-icon">
          <MaterialIcon type="person" />
          <div className="email-warning">{member.email}</div>
        </div>
      </div>
      <div className="col-4 text-end">
        <MemberPrivileges privileges={member.privilege} />
      </div>
    </div>
  )
}

type PermissionsOption = PermissionsLevel | 'removeAccess' | 'downgraded'

function EditMember({
  member,
  hasExceededCollaboratorLimit,
  canAddCollaborators,
  isReviewerOnFreeProject,
}: {
  member: Member
  hasExceededCollaboratorLimit: boolean
  canAddCollaborators: boolean
  isReviewerOnFreeProject?: boolean
}) {
  const { t } = useTranslation()
  const [privileges, setPrivilegesState] = useState<PermissionsOption>(member.privilege)
  const [confirmingOwnershipTransfer, setConfirmingOwnershipTransfer] = useState(false)
  const [privilegeChangePending, setPrivilegeChangePending] = useState(false)

  useEffect(() => {
    setPrivilegesState(member.privilege)
  }, [member.privilege])

  const { monitorRequest, members, invites, updateProject } = useShareProjectContext()
  const { projectId } = useProject()

  function handlePrivilegeChange(newPrivileges: PermissionsOption) {
    setPrivilegesState(newPrivileges)
    if (newPrivileges !== 'removeAccess') {
      commitPrivilegeChange(newPrivileges)
    } else {
      setPrivilegeChangePending(true)
    }
  }

  function shouldWarnMember() {
    return hasExceededCollaboratorLimit && ['readAndWrite', 'review'].includes(privileges)
  }

  function commitPrivilegeChange(newPrivileges: PermissionsOption) {
    setPrivilegesState(newPrivileges)
    setPrivilegeChangePending(false)

    if (newPrivileges === 'owner') {
      setConfirmingOwnershipTransfer(true)
    } else if (newPrivileges === 'removeAccess') {
      monitorRequest(() => removeMemberRequest(projectId, member.id)).then(() => {
        updateProject({ members: members.filter(existing => existing.id !== member.id) })
      })
    } else if (newPrivileges === 'readAndWrite' || newPrivileges === 'review' || newPrivileges === 'readOnly') {
      monitorRequest(() => setPrivilege(projectId, member.id, newPrivileges)).then(() => {
        updateProject({
          members: members.map(item => (item.id === member.id ? { ...item, privilege: newPrivileges } : item)),
        })
      })
    }
  }

  void invites

  if (confirmingOwnershipTransfer) {
    return (
      <TransferOwnershipModal
        member={member}
        cancel={() => {
          setConfirmingOwnershipTransfer(false)
          setPrivilegesState(member.privilege)
        }}
      />
    )
  }

  const confirmRemoval = privileges !== member.privilege && privilegeChangePending

  return (
    <form
      id="share-project-form"
      onSubmit={event => {
        event.preventDefault()
        if (privilegeChangePending) {
          commitPrivilegeChange(privileges)
        }
      }}
    >
      <div className="form-group project-member row">
        <div className="col-8">
          <div className="project-member-email-icon">
            <MaterialIcon type={shouldWarnMember() ? 'warning' : 'person'} className={shouldWarnMember() ? 'project-member-warning' : undefined} />
            <div className="email-warning">
              {member.email}
              {isReviewerOnFreeProject ? (
                <div className="small">
                  <Trans i18nKey="comment_only_upgrade_to_enable_track_changes" components={[<button key="upgrade" type="button" className="btn btn-link btn-inline-link" />]} />
                </div>
              ) : null}
            </div>
          </div>
        </div>

        <div className="col-4 project-member-actions">
          <div className="project-member-select">
            <SelectPrivilege
              value={privileges}
              handleChange={value => {
                if (value) {
                  handlePrivilegeChange(value.key)
                }
              }}
              hasBeenDowngraded={false}
              canAddCollaborators={canAddCollaborators}
            />
          </div>

          {confirmRemoval ? <ChangePrivilegesActions handleReset={() => setPrivilegesState(member.privilege)} /> : null}
        </div>
      </div>
    </form>
  )
}

type PrivilegeOption = { key: PermissionsOption; label: string }

function SelectPrivilege({
  value,
  handleChange,
  hasBeenDowngraded,
  canAddCollaborators,
}: {
  value: string
  handleChange: (item: PrivilegeOption | null | undefined) => void
  hasBeenDowngraded: boolean
  canAddCollaborators: boolean
}) {
  const { t } = useTranslation()
  const { features } = useProject()

  const privileges = useMemo(
    (): PrivilegeOption[] =>
      features.trackChangesVisible
        ? [
            { key: 'owner', label: t('make_owner') },
            { key: 'readAndWrite', label: t('editor') },
            { key: 'review', label: t('reviewer') },
            { key: 'readOnly', label: t('viewer') },
            { key: 'removeAccess', label: t('remove_access') },
          ]
        : [
            { key: 'owner', label: t('make_owner') },
            { key: 'readAndWrite', label: t('editor') },
            { key: 'readOnly', label: t('viewer') },
            { key: 'removeAccess', label: t('remove_access') },
          ],
    [features.trackChangesVisible, t]
  )

  const downgradedPseudoPrivilege: PrivilegeOption = { key: 'downgraded', label: t('select_access_level') }

  function getPrivilegeSubtitle(privilege: PermissionsOption) {
    if (!['readAndWrite', 'review'].includes(privilege)) {
      return ''
    }
    if (hasBeenDowngraded || (!canAddCollaborators && !['readAndWrite', 'review'].includes(value))) {
      return t('limited_to_n_collaborators_per_project', { count: features.collaborators })
    }
    return ''
  }

  function isPrivilegeDisabled(privilege: PermissionsOption) {
    return !canAddCollaborators && ['readAndWrite', 'review'].includes(privilege) && (hasBeenDowngraded || !['readAndWrite', 'review'].includes(value))
  }

  return (
    <Select
      items={privileges}
      itemToKey={item => item.key}
      itemToString={item => (item ? item.label : '')}
      itemToSubtitle={item => (item ? getPrivilegeSubtitle(item.key) : '')}
      itemToDisabled={item => (item ? isPrivilegeDisabled(item.key) : false)}
      defaultItem={privileges.find(item => item.key === value)}
      selected={hasBeenDowngraded ? downgradedPseudoPrivilege : privileges.find(item => item.key === value)}
      name="privileges"
      onSelectedItemChanged={handleChange}
      selectedIcon
    />
  )
}

function ChangePrivilegesActions({ handleReset }: { handleReset: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="text-center d-flex gap-1 me-3">
      <Button type="button" size="sm" variant="secondary" onClick={handleReset}>
        {t('cancel')}
      </Button>
      <Button type="submit" size="sm" variant="primary">
        {t('confirm')}
      </Button>
    </div>
  )
}

function InviteRow({ invite, isProjectOwner }: { invite: InviteData; isProjectOwner: boolean }) {
  const { t } = useTranslation()
  return (
    <div className="row project-invite">
      <div className="col-8">
        <div>{invite.email}</div>
        <div className="small">
          {t('invite_not_accepted')}
          .&nbsp;
          {isProjectOwner ? <ResendInvite invite={invite} /> : null}
        </div>
      </div>
      <div className="col-3 text-end">
        <MemberPrivileges privileges={invite.privilege} />
      </div>
      {isProjectOwner ? (
        <div className="col-1 text-center">
          <RevokeInvite invite={invite} />
        </div>
      ) : null}
    </div>
  )
}

function ResendInvite({ invite }: { invite: InviteData }) {
  const { t } = useTranslation()
  const { monitorRequest, setError, inFlight } = useShareProjectContext()
  const { projectId } = useProject()

  const handleClick = useCallback(
    () =>
      monitorRequest(() => resendInviteRequest(projectId, invite.id))
        .catch((error: unknown) => {
          if (error instanceof ApiError && error.status === 404) {
            setError('invite_expired')
          }
          if (error instanceof ApiError && error.status === 429) {
            setError('invite_resend_limit_hit')
          }
        })
        .finally(() => {
          if (document.activeElement) {
            ;(document.activeElement as HTMLElement).blur()
          }
        }),
    [invite, monitorRequest, projectId, setError]
  )

  return (
    <button type="button" className="btn btn-link btn-inline-link" onClick={handleClick} disabled={inFlight}>
      {t('resend')}
    </button>
  )
}

function RevokeInvite({ invite }: { invite: InviteData }) {
  const { t } = useTranslation()
  const { monitorRequest, invites, updateProject } = useShareProjectContext()
  const { projectId } = useProject()

  function handleClick(event: React.MouseEvent) {
    event.preventDefault()
    monitorRequest(() => revokeInviteRequest(projectId, invite.id)).then(() => {
      updateProject({ invites: invites.filter(existing => existing.id !== invite.id) })
    })
  }

  return (
    <Tooltip id="revoke-invite" description={t('revoke_invite')} overlayProps={{ placement: 'bottom' }}>
      <button type="button" className="btn btn-link btn-inline-link text-decoration-none" onClick={handleClick} aria-label={t('revoke')}>
        <MaterialIcon type="clear" />
      </button>
    </Tooltip>
  )
}

function TransferOwnershipModal({ member, cancel }: { member: Member; cancel: () => void }) {
  const { t } = useTranslation()
  const [inflight, setInflight] = useState(false)
  const [error, setError] = useState(false)
  const { projectId, project } = useProject()

  function confirm() {
    setError(false)
    setInflight(true)
    transferOwnership(projectId, member.id)
      .then(() => window.location.reload())
      .catch(() => {
        setError(true)
        setInflight(false)
      })
  }

  return (
    <OLModal show onHide={cancel}>
      <OLModalHeader>
        <OLModalTitle>{t('change_project_owner')}</OLModalTitle>
      </OLModalHeader>
      <OLModalBody className="ol-ui">
        <p>
          <Trans
            i18nKey="project_ownership_transfer_confirmation_1"
            values={{ user: member.email, project: project.name }}
            components={[<strong key="strong-1" />, <strong key="strong-2" />]}
            shouldUnescape
          />
        </p>
        <p>{t('project_ownership_transfer_confirmation_2')}</p>
        {error ? <Notification type="error" content={t('generic_something_went_wrong')} className="mb-0 mt-3" /> : null}
      </OLModalBody>
      <OLModalFooter>
        <div className="me-auto">{inflight ? <Spinner size="sm" /> : null}</div>
        <Button variant="secondary" onClick={cancel} disabled={inflight}>
          {t('cancel')}
        </Button>
        <Button variant="primary" onClick={confirm} disabled={inflight}>
          {t('change_owner')}
        </Button>
      </OLModalFooter>
    </OLModal>
  )
}
