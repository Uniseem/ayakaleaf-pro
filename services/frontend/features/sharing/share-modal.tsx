'use client'

/**
 * Sharing a project.
 *
 * Two ways to let somebody in, kept visibly apart: by name, which is a
 * decision about one person and can be undone for that person, and by link,
 * which is a decision about anybody who ends up holding it. Presenting them as
 * one control is how people turn on link sharing while meaning to add a
 * colleague.
 */

import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Spinner,
  Switch,
} from '@heroui/react'
import { useCallback, useEffect, useState } from 'react'
import {
  getSharing,
  invite as sendInvite,
  removeMember,
  revokeInvite,
  setPrivilege,
  setPublicAccess,
  type Privilege,
  type Sharing,
} from '@/lib/sharing'
import { messageFor } from '@/lib/api'
import { useProject } from '@/features/ide/contexts/project-context'

export function ShareModal({
  isOpen,
  onClose,
}: {
  isOpen: boolean
  onClose: () => void
}) {
  const { projectId, isOwner } = useProject()
  const [sharing, setSharing] = useState<Sharing | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [email, setEmail] = useState('')
  const [privilege, setPrivilegeChoice] = useState<Privilege>('readAndWrite')
  /** Shown once, after an invitation is made, because it is never readable again. */
  const [link, setLink] = useState<string | null>(null)
  const [emailed, setEmailed] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setSharing(await getSharing(projectId))
    } catch (thrown) {
      setError(messageFor(thrown))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    if (isOpen) {
      void load()
    }
  }, [isOpen, load])

  const run = useCallback(
    async (work: () => Promise<unknown>) => {
      setBusy(true)
      setError(null)
      try {
        await work()
        await load()
      } catch (thrown) {
        setError(messageFor(thrown))
      } finally {
        setBusy(false)
      }
    },
    [load]
  )

  return (
    <Modal isOpen={isOpen} onClose={onClose} size="lg">
      <ModalContent>
        <ModalHeader>Share this project</ModalHeader>
        <ModalBody className="gap-4">
          {loading ? (
            <div className="flex justify-center py-6">
              <Spinner size="sm" />
            </div>
          ) : (
            <>
              {isOwner ? (
                <form
                  className="flex items-end gap-2"
                  onSubmit={event => {
                    event.preventDefault()
                    const address = email.trim()
                    if (!address || busy) {
                      return
                    }
                    void run(async () => {
                      const answer = await sendInvite(projectId, address, privilege)
                      setEmail('')
                      setLink(answer.link ?? null)
                      setEmailed(Boolean(answer.sent))
                    })
                  }}
                >
                  <Input
                    size="sm"
                    type="email"
                    label="Add somebody"
                    placeholder="them@university.edu"
                    value={email}
                    onValueChange={setEmail}
                    className="flex-1"
                  />
                  <Select
                    size="sm"
                    aria-label="What they may do"
                    className="w-40"
                    selectedKeys={[privilege]}
                    onSelectionChange={keys => {
                      const chosen = [...keys][0]
                      if (chosen) {
                        setPrivilegeChoice(String(chosen) as Privilege)
                      }
                    }}
                  >
                    <SelectItem key="readAndWrite">Can edit</SelectItem>
                    <SelectItem key="readOnly">Can view</SelectItem>
                  </Select>
                  <Button
                    size="sm"
                    color="primary"
                    type="submit"
                    className="h-10"
                    isLoading={busy}
                    isDisabled={!email.trim()}
                  >
                    Add
                  </Button>
                </form>
              ) : null}

              {link ? (
                <div className="rounded-[4px] bg-[var(--bg-accent-03)] p-3">
                  <p className="text-[12px] font-medium leading-4 text-[var(--content-positive)]">
                    {emailed
                      ? 'They have no account yet, so an invitation has been emailed to them.'
                      : 'They have no account yet, and this site has no mail server — send them this link yourself.'}
                  </p>
                  <p className="mt-1 break-all font-mono text-[11px] leading-4 text-[var(--content-primary)]">
                    {window.location.origin + link}
                  </p>
                  <p className="mt-1 text-[11px] leading-4 text-[var(--content-secondary)]">
                    It is shown once. Anybody holding it can take the access it
                    offers.
                  </p>
                </div>
              ) : null}

              {error ? (
                <p className="rounded bg-danger-50 px-3 py-2 text-sm text-danger">
                  {error}
                </p>
              ) : null}

              <section>
                <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-default-500">
                  People
                </h3>
                <ul className="divide-y divide-divider rounded-md border border-divider">
                  {sharing?.members.map(person => (
                    <li key={person.id} className="flex items-center gap-2 px-3 py-2">
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm">{person.name}</p>
                        <p className="truncate text-[11px] text-default-500">
                          {person.email}
                        </p>
                      </div>
                      {person.owner ? (
                        <Chip size="sm" variant="flat">
                          Owner
                        </Chip>
                      ) : isOwner ? (
                        <>
                          <Select
                            size="sm"
                            aria-label={`What ${person.name} may do`}
                            className="w-32"
                            selectedKeys={[person.privilege]}
                            isDisabled={busy}
                            onSelectionChange={keys => {
                              const chosen = [...keys][0]
                              if (chosen && chosen !== person.privilege) {
                                void run(() =>
                                  setPrivilege(
                                    projectId,
                                    person.id,
                                    String(chosen) as Privilege
                                  )
                                )
                              }
                            }}
                          >
                            <SelectItem key="readAndWrite">Can edit</SelectItem>
                            <SelectItem key="readOnly">Can view</SelectItem>
                          </Select>
                          <Button
                            size="sm"
                            variant="light"
                            color="danger"
                            className="h-8"
                            isDisabled={busy}
                            onPress={() =>
                              void run(() => removeMember(projectId, person.id))
                            }
                          >
                            Remove
                          </Button>
                        </>
                      ) : (
                        <Chip size="sm" variant="flat">
                          {person.privilege === 'readOnly' ? 'Can view' : 'Can edit'}
                        </Chip>
                      )}
                    </li>
                  ))}
                </ul>
              </section>

              {sharing && sharing.invites.length > 0 ? (
                <section>
                  <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-default-500">
                    Invited, not yet accepted
                  </h3>
                  <ul className="divide-y divide-divider rounded-md border border-divider">
                    {sharing.invites.map(pending => (
                      <li key={pending.id} className="flex items-center gap-2 px-3 py-2">
                        <span className="min-w-0 flex-1 truncate text-sm">
                          {pending.email}
                        </span>
                        <Chip size="sm" variant="flat">
                          {pending.privilege === 'readOnly' ? 'Can view' : 'Can edit'}
                        </Chip>
                        <Button
                          size="sm"
                          variant="light"
                          color="danger"
                          className="h-8"
                          isDisabled={busy}
                          onPress={() =>
                            void run(() => revokeInvite(projectId, pending.id))
                          }
                        >
                          Withdraw
                        </Button>
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}

              {isOwner ? (
                <section className="rounded-md border border-divider p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium">Anybody with the link</p>
                      <p className="text-[11px] text-default-500">
                        Turning this on lets anybody who is given the address
                        open this project, without being added by name.
                      </p>
                    </div>
                    <Switch
                      size="sm"
                      isSelected={sharing?.publicAccess === 'tokenBased'}
                      isDisabled={busy}
                      aria-label="Anybody with the link"
                      onValueChange={on =>
                        void run(() =>
                          setPublicAccess(projectId, on ? 'tokenBased' : 'private')
                        )
                      }
                    />
                  </div>
                </section>
              ) : null}
            </>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            Done
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  )
}
