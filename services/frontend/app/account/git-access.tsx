'use client'

import { Card, CardBody, CardHeader } from '@/components/ol/card'
import { Button } from '@/components/ol/button'
import { CopyToClipboard } from '@/components/ol/misc'
import { useState } from 'react'
import { messageFor } from '@/lib/api'
import { createToken, revokeToken, type AccessToken } from '@/lib/tokens'

/**
 * Git access.
 *
 * A token rather than the account password, because a git client has one place
 * to put a secret and no way to show anything else: a token can be revoked on
 * its own, it says when it was last used, and losing one does not lose the
 * account.
 */
export function GitAccess({ tokens: initial }: { tokens: AccessToken[] }) {
  const [tokens, setTokens] = useState(initial)
  const [secret, setSecret] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function make() {
    setBusy(true)
    setError('')
    try {
      const answer = await createToken()
      setTokens(previous => [answer.token, ...previous])
      setSecret(answer.accessToken)
    } catch (problem) {
      setError(messageFor(problem))
    } finally {
      setBusy(false)
    }
  }

  async function revoke(id: string) {
    setError('')
    try {
      await revokeToken(id)
      setTokens(previous => previous.filter(token => token.id !== id))
    } catch (problem) {
      setError(messageFor(problem))
    }
  }

  return (
    <Card>
      <CardHeader title="Git access">
        <p className="card-subtitle">
          Clone a project with the username <code>git</code> and one of these as the password.
        </p>
      </CardHeader>
      <CardBody className="flex flex-col gap-4">
        {error ? (
          <div
            role="alert"
            className="alert alert-danger"
          >
            {error}
          </div>
        ) : null}

        {secret ? (
          <div className="alert alert-success flex flex-col gap-2">
            <p className="font-medium">Copy this now. It is not shown again.</p>
            <div className="access-token">
              <code>{secret}</code>
              <CopyToClipboard content={secret} tooltipId="copy-git-token" kind="button" />
            </div>
          </div>
        ) : null}

        {tokens.length === 0 ? (
          <p className="text-[var(--content-secondary)]">No tokens yet.</p>
        ) : (
          <ul className="flex list-none flex-col p-0">
            {tokens.map(token => (
              <li
                key={token.id}
                className="flex items-center gap-4 border-b border-[var(--border-divider)] py-3 last:border-0"
              >
                <code>{token.partial}…</code>
                <span className="text-xs text-[var(--content-secondary)]">
                  {token.lastUsedAt
                    ? `last used ${when(token.lastUsedAt)}`
                    : `made ${when(token.createdAt)}, never used`}
                </span>
                <Button
                  size="sm"
                  variant="danger-ghost"
                  className="ml-auto"
                  onClick={() => void revoke(token.id)}
                >
                  Revoke
                </Button>
              </li>
            ))}
          </ul>
        )}

        <div>
          <Button size="sm" variant="primary" isLoading={busy} onClick={() => void make()}>
            New token
          </Button>
        </div>
      </CardBody>
    </Card>
  )
}

function when(value: string): string {
  const at = new Date(value)
  return Number.isNaN(at.getTime()) ? 'once' : at.toLocaleDateString()
}
