'use client'

import { Card, CardBody, CardHeader } from '@/components/ol/card'
import { Button } from '@/components/ol/button'
import { Select } from '@/components/ol/select'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
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
    <Card>
      <CardHeader
        title="GitHub"
        subtitle="Import a repository as a project, and push changes back to it."
      />
      <CardBody className="flex flex-col gap-4">
        {error ? (
          <div
            role="alert"
            className="alert alert-danger"
          >
            {error}
          </div>
        ) : null}

        {connected ? (
          <>
            <p>
              Connected as <span className="font-medium">{login}</span>.
            </p>
            <div className="flex gap-2">
              <Button size="sm" variant="primary" onClick={() => void openImport()}>
                Import a repository
              </Button>
              <Button size="sm" variant="secondary" isLoading={busy} onClick={() => void disconnect()}>
                Disconnect
              </Button>
            </div>
          </>
        ) : (
          <div>
            <Button size="sm" variant="primary" href="/api/github/start">
              Connect GitHub
            </Button>
          </div>
        )}
      </CardBody>

      <OLModal show={importing} onHide={() => setImporting(false)} size="lg">
        <OLModalHeader>
          <OLModalTitle>Import a repository</OLModalTitle>
        </OLModalHeader>
        <OLModalBody className="flex flex-col gap-3">
          <Select
            label="Repository"
            items={repos ?? []}
            loading={loadingRepos}
            itemToKey={repo => repo.fullName}
            itemToString={repo => repo?.fullName ?? ''}
            selected={chosen}
            size="sm"
            onSelectedItemChanged={repo => setChosen(repo ?? null)}
          />
          <p className="text-xs text-[var(--content-secondary)]">
            The default branch is imported. The project stays linked, so it can be synced
            afterwards from the editor.
          </p>
        </OLModalBody>
        <OLModalFooter>
          <Button size="sm" variant="secondary" onClick={() => setImporting(false)}>
            Cancel
          </Button>
          <Button
            size="sm"
            variant="primary"
            disabled={!chosen}
            isLoading={busy}
            onClick={() => void doImport()}
          >
            Import
          </Button>
        </OLModalFooter>
      </OLModal>
    </Card>
  )
}
