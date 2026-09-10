'use client'

import { Button } from '@/components/ol/button'
import { Spinner } from '@/components/ol/spinner'
import { OLFormControl, OLFormGroup, OLFormLabel } from '@/components/ol/form-control'
import { OLModal, OLModalBody, OLModalFooter, OLModalHeader, OLModalTitle } from '@/components/ol/modal'
import { useCallback, useEffect, useState } from 'react'
import { messageFor } from '@/lib/api'
import {
  exportProject,
  mergeOverview,
  projectGitHub,
  syncProject,
  unlinkRepo,
  type MergeOverview,
  type ProjectGitHub,
} from '@/lib/github'

/**
 * The repository this project is kept in step with.
 *
 * Everything here is one of three answers: it is not linked and could be, it is
 * linked and in step, or a merge needs a person. The third is the one worth
 * being careful about -- there is a branch waiting on GitHub, and nothing here
 * can tell whether somebody has merged it, so it says what to do and waits to
 * be told.
 */
export function GitHubPanel({
  projectId,
  projectName,
  initial,
  open,
  onClose,
}: {
  projectId: string
  projectName: string
  initial: ProjectGitHub
  open: boolean
  onClose: () => void
}) {
  const [status, setStatus] = useState(initial)
  const [overview, setOverview] = useState<MergeOverview | null>(null)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  const [repoName, setRepoName] = useState(slug(projectName))
  const [isPublic, setIsPublic] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const next = await projectGitHub(projectId)
      setStatus(next)
      if (next.linked && next.state?.mergeStatus !== 'conflict') {
        setOverview(await mergeOverview(projectId))
      } else {
        setOverview(null)
      }
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    if (open) {
      void refresh()
    }
  }, [open, refresh])

  async function sync(resolved: boolean) {
    setBusy(true)
    setError('')
    try {
      await syncProject(projectId, { message: message.trim() || undefined, resolved })
      await refresh()
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setBusy(false)
    }
  }

  async function create() {
    setBusy(true)
    setError('')
    try {
      await exportProject(projectId, { name: repoName.trim(), isPublic })
      await refresh()
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setBusy(false)
    }
  }

  async function unlink() {
    setBusy(true)
    setError('')
    try {
      await unlinkRepo(projectId)
      await refresh()
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setBusy(false)
    }
  }

  const state = status.state
  const conflicted = state?.mergeStatus === 'conflict'

  return (
    <OLModal show={open} onHide={onClose} size="lg">
      <OLModalHeader>
        <OLModalTitle>GitHub</OLModalTitle>
      </OLModalHeader>
      <OLModalBody className="flex flex-col gap-4">
          {error ? (
            <div
              role="alert"
              className="alert alert-danger"
            >
              {error}
            </div>
          ) : null}

          {loading ? (
            <div className="flex justify-center py-6">
              <Spinner size="sm" />
            </div>
          ) : !status.linked ? (
            <>
              <p className="text-[var(--content-secondary)]">
                Not linked to a repository. Making one puts this project in it and
                keeps the two in step from then on.
              </p>
              <OLFormGroup controlId="github-repo-name">
                <OLFormLabel>Repository name</OLFormLabel>
                <OLFormControl
                  size="sm"
                  value={repoName}
                  onChange={event => setRepoName(event.target.value)}
                />
              </OLFormGroup>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  className="form-check-input"
                  checked={isPublic}
                  onChange={event => setIsPublic(event.target.checked)}
                />
                Anyone can see it
              </label>
            </>
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-2">
                <a
                  className="font-medium underline"
                  href={`https://github.com/${state?.repoFullName}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {state?.repoFullName}
                </a>
                <span className="text-[var(--content-secondary)]">
                  {state?.defaultBranchName ? `on ${state.defaultBranchName}` : ''}
                </span>
              </div>

              {status.canPush === false ? (
                <p className="text-[var(--content-warning)]">
                  This GitHub account cannot write to that repository, so a sync
                  would only be able to pull.
                </p>
              ) : null}

              {conflicted ? (
                <div className="alert alert-warning flex flex-col gap-2">
                  <p className="font-medium">This one needs merging by hand.</p>
                  <p>
                    The changes that could not be merged are on the branch{' '}
                    <code>{state?.unmergedBranchName}</code>. Merge it into{' '}
                    <code>{state?.defaultBranchName}</code> on GitHub, then come
                    back and say it is done.
                  </p>
                </div>
              ) : overview ? (
                <div className="flex flex-col gap-2">
                  <p className="text-[var(--content-secondary)]">
                    {overview.projectChanged
                      ? 'This project has changes to push.'
                      : 'Nothing new here.'}
                    {overview.commits.length > 0
                      ? ` ${overview.commits.length} new commit${
                          overview.commits.length === 1 ? '' : 's'
                        } to pull.`
                      : ' Nothing new there.'}
                  </p>
                  {overview.repoRewritten ? (
                    <p className="text-[var(--content-warning)]">
                      That repository was rewritten since the last sync, so this
                      will be worked out file by file.
                    </p>
                  ) : null}
                  {overview.commits.length > 0 ? (
                    <ul className="flex flex-col gap-1">
                      {overview.commits.slice(0, 8).map(commit => (
                        <li key={commit.sha} className="flex gap-2">
                          <code className="shrink-0 text-xs text-[var(--content-secondary)]">
                            {commit.sha.slice(0, 7)}
                          </code>
                          <span className="truncate">
                            {commit.message.split('\n')[0]}
                          </span>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </div>
              ) : null}

              {!conflicted ? (
                <OLFormGroup controlId="github-commit-message">
                  <OLFormLabel>Commit message</OLFormLabel>
                  <OLFormControl
                    size="sm"
                    placeholder={`Updates from ${projectName}`}
                    value={message}
                    onChange={event => setMessage(event.target.value)}
                  />
                </OLFormGroup>
              ) : null}
            </>
          )}
      </OLModalBody>
      <OLModalFooter className="flex-wrap gap-2">
          {status.linked ? (
            <Button size="sm" variant="danger-ghost" isLoading={busy} onClick={() => void unlink()}>
              Unlink
            </Button>
          ) : null}
          <Button size="sm" variant="secondary" onClick={onClose}>
            Close
          </Button>
          {!status.linked ? (
            <Button size="sm" variant="primary" isLoading={busy} onClick={() => void create()}>
              Create repository
            </Button>
          ) : conflicted ? (
            <Button size="sm" variant="primary" isLoading={busy} onClick={() => void sync(true)}>
              I have merged it
            </Button>
          ) : (
            <Button size="sm" variant="primary" isLoading={busy} onClick={() => void sync(false)}>
              Sync
            </Button>
          )}
      </OLModalFooter>
    </OLModal>
  )
}

/** A project name as a repository name: what GitHub will accept. */
function slug(name: string): string {
  const cleaned = name.replace(/[^A-Za-z0-9._-]+/g, '-').replace(/^-+|-+$/g, '')
  return cleaned || 'project'
}
