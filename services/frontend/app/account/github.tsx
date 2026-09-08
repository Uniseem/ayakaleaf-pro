'use client'

import {
  Autocomplete,
  AutocompleteItem,
  Button,
  Card,
  CardBody,
  CardHeader,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from '@heroui/react'
import { useRouter } from 'next/navigation'
import { useState } from 'react'
import { messageFor } from '@/lib/api'
import {
  importRepo,
  listRepos,
  unlinkGitHub,
  type GitHubRepo,
  type GitHubStatus,
} from '@/lib/github'

/**
 * The GitHub account this person has connected, and what it can be used for.
 *
 * Separate from signing in with GitHub, and says so: this connection can read
 * and write repositories, which is not something to ask of somebody who only
 * wanted a way in.
 */
export function GitHubAccount({ status }: { status: GitHubStatus }) {
  const router = useRouter()
  const [connected, setConnected] = useState(status.connected)
  const [login, setLogin] = useState(status.login)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const [importing, setImporting] = useState(false)
  const [repos, setRepos] = useState<GitHubRepo[] | null>(null)
  const [chosen, setChosen] = useState<GitHubRepo | null>(null)
  const [loadingRepos, setLoadingRepos] = useState(false)

  if (!status.enabled) {
    return null
  }

  async function disconnect() {
    setBusy(true)
    setError('')
    try {
      await unlinkGitHub()
      setConnected(false)
      setLogin(undefined)
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setBusy(false)
    }
  }

  async function openImport() {
    setImporting(true)
    setError('')
    if (repos) {
      return
    }
    setLoadingRepos(true)
    try {
      setRepos(await listRepos())
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setLoadingRepos(false)
    }
  }

  async function doImport() {
    if (!chosen) {
      return
    }
    setBusy(true)
    setError('')
    try {
      const answer = await importRepo({
        fullName: chosen.fullName,
        defaultBranchName: chosen.defaultBranchName,
        name: chosen.name,
      })
      router.push(`/projects/${answer.projectId}`)
    } catch (problem) {
      setError(messageFor(problem))
      setBusy(false)
    }
  }

  return (
    <Card shadow="sm">
      <CardHeader className="flex-col items-start gap-1 px-6 pt-6">
        <h2 className="text-lg font-medium">GitHub</h2>
        <p className="text-small text-default-500">
          Import a repository as a project, and push changes back to it.
        </p>
      </CardHeader>
      <CardBody className="gap-4 px-6 pb-6">
        {error ? (
          <div
            role="alert"
            className="rounded-medium border border-danger-200 bg-danger-50 px-4 py-3 text-small text-danger-700 dark:bg-danger-50/10"
          >
            {error}
          </div>
        ) : null}

        {connected ? (
          <>
            <p className="text-small">
              Connected as <span className="font-medium">{login}</span>.
            </p>
            <div className="flex gap-2">
              <Button size="sm" color="primary" onPress={() => void openImport()}>
                Import a repository
              </Button>
              <Button size="sm" variant="light" isLoading={busy} onPress={() => void disconnect()}>
                Disconnect
              </Button>
            </div>
          </>
        ) : (
          <div>
            <Button size="sm" color="primary" as="a" href="/api/github/start">
              Connect GitHub
            </Button>
          </div>
        )}
      </CardBody>

      <Modal isOpen={importing} onClose={() => setImporting(false)} size="lg">
        <ModalContent>
          <ModalHeader className="text-base">Import a repository</ModalHeader>
          <ModalBody className="gap-3">
            <Autocomplete
              label="Repository"
              size="sm"
              isLoading={loadingRepos}
              items={repos ?? []}
              onSelectionChange={key => {
                setChosen(repos?.find(repo => repo.fullName === key) ?? null)
              }}
            >
              {repo => (
                <AutocompleteItem key={repo.fullName} textValue={repo.fullName}>
                  {repo.fullName}
                </AutocompleteItem>
              )}
            </Autocomplete>
            <p className="text-tiny text-default-400">
              The default branch is imported. The project stays linked, so it can
              be synced afterwards from the editor.
            </p>
          </ModalBody>
          <ModalFooter>
            <Button size="sm" variant="light" onPress={() => setImporting(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              color="primary"
              isDisabled={!chosen}
              isLoading={busy}
              onPress={() => void doImport()}
            >
              Import
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </Card>
  )
}
