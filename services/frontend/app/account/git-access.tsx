'use client'

import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Snippet,
} from '@heroui/react'
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
    <Card shadow="sm">
      <CardHeader className="flex-col items-start gap-1 px-6 pt-6">
        <h2 className="text-lg font-medium">Git access</h2>
        <p className="text-small text-default-500">
          Clone a project with the username <code>git</code> and one of these as
          the password.
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

        {secret ? (
          <div className="flex flex-col gap-2 rounded-medium border border-success-200 bg-success-50 p-4 dark:bg-success-50/10">
            <p className="text-small font-medium">
              Copy this now. It is not shown again.
            </p>
            <Snippet size="sm" symbol="" variant="bordered" className="w-full">
              {secret}
            </Snippet>
          </div>
        ) : null}

        {tokens.length === 0 ? (
          <p className="text-small text-default-400">No tokens yet.</p>
        ) : (
          <ul className="flex flex-col divide-y divide-divider">
            {tokens.map(token => (
              <li key={token.id} className="flex items-center gap-4 py-3">
                <code className="text-small">{token.partial}…</code>
                <span className="text-tiny text-default-400">
                  {token.lastUsedAt
                    ? `last used ${when(token.lastUsedAt)}`
                    : `made ${when(token.createdAt)}, never used`}
                </span>
                <Button
                  size="sm"
                  variant="light"
                  color="danger"
                  className="ml-auto"
                  onPress={() => void revoke(token.id)}
                >
                  Revoke
                </Button>
              </li>
            ))}
          </ul>
        )}

        <div>
          <Button size="sm" color="primary" isLoading={busy} onPress={() => void make()}>
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
