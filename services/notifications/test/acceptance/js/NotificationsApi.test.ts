import { randomBytes } from 'node:crypto'
import { describe, it, expect } from 'vitest'
import { BASE_URL } from './helpers/NotificationsApp.ts'

// A black-box contract suite for the notifications HTTP API.
//
// Everything here goes over HTTP and asserts only on status codes and response
// bodies, so the same suite holds any implementation of the service to the
// same contract. Run it with NOTIFICATIONS_EXTERNAL=true to point it at the Go
// port in services-go.

function newObjectId(): string {
  return randomBytes(12).toString('hex')
}

type Notification = {
  _id: string
  user_id: string
  key: string
  messageOpts?: unknown
  templateKey?: string
}

async function addNotification(userId: string, body: Record<string, unknown>) {
  return await fetch(`${BASE_URL}/user/${userId}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

async function getNotifications(userId: string): Promise<Notification[]> {
  const response = await fetch(`${BASE_URL}/user/${userId}`)
  expect(response.status).toBe(200)
  return (await response.json()) as Notification[]
}

function sampleNotification(key: string, overrides: Record<string, unknown> = {}) {
  return { key, messageOpts: 'some-opts', templateKey: 'template-key', ...overrides }
}

describe('notifications API', () => {
  describe('creating and reading notifications', () => {
    it('stores a notification and returns it for the user', async () => {
      const userId = newObjectId()
      const key = `key-${newObjectId()}`

      const response = await addNotification(userId, sampleNotification(key))
      expect(response.status).toBe(200)

      const notifications = await getNotifications(userId)
      expect(notifications.length).toBe(1)
      expect(notifications[0].key).toBe(key)
      expect(notifications[0].user_id).toBe(userId)
      expect(notifications[0].templateKey).toBe('template-key')
      expect(notifications[0].messageOpts).toBe('some-opts')
      // Ids are serialised as hex strings, not as extended JSON.
      expect(notifications[0]._id).toMatch(/^[0-9a-f]{24}$/)
    })

    it('returns an empty array for a user with no notifications', async () => {
      expect(await getNotifications(newObjectId())).toEqual([])
    })

    it('does not duplicate a notification with an existing key', async () => {
      const userId = newObjectId()
      const key = `key-${newObjectId()}`

      expect((await addNotification(userId, sampleNotification(key))).status).toBe(200)
      const second = await addNotification(
        userId,
        sampleNotification(key, { templateKey: 'ignored' })
      )
      expect(second.status).toBe(200)

      const notifications = await getNotifications(userId)
      expect(notifications.length).toBe(1)
      // The second write is skipped entirely, so the original survives.
      expect(notifications[0].templateKey).toBe('template-key')
    })

    it('overwrites an existing notification when forceCreate is set', async () => {
      const userId = newObjectId()
      const key = `key-${newObjectId()}`

      await addNotification(userId, sampleNotification(key))
      const forced = await addNotification(
        userId,
        sampleNotification(key, { templateKey: 'replacement', forceCreate: true })
      )
      expect(forced.status).toBe(200)

      const notifications = await getNotifications(userId)
      expect(notifications.length).toBe(1)
      expect(notifications[0].templateKey).toBe('replacement')
    })

    it('accepts an expires date', async () => {
      const userId = newObjectId()
      const key = `key-${newObjectId()}`
      const expires = new Date(Date.now() + 60_000).toISOString()

      const response = await addNotification(userId, sampleNotification(key, { expires }))
      expect(response.status).toBe(200)
      expect((await getNotifications(userId)).length).toBe(1)
    })

    it('rejects an unparseable expires date', async () => {
      const userId = newObjectId()
      const response = await addNotification(
        userId,
        sampleNotification(`key-${newObjectId()}`, { expires: 'not-a-date' })
      )
      expect(response.status).toBe(500)
    })
  })

  describe('marking notifications as read', () => {
    it('removes a notification by id', async () => {
      const userId = newObjectId()
      const key = `key-${newObjectId()}`
      await addNotification(userId, sampleNotification(key))
      const [notification] = await getNotifications(userId)

      const response = await fetch(
        `${BASE_URL}/user/${userId}/notification/${notification._id}`,
        { method: 'DELETE' }
      )
      expect(response.status).toBe(200)
      expect(await getNotifications(userId)).toEqual([])
    })

    it('removes a notification by key', async () => {
      const userId = newObjectId()
      const key = `key-${newObjectId()}`
      await addNotification(userId, sampleNotification(key))

      const response = await fetch(`${BASE_URL}/user/${userId}`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key }),
      })
      expect(response.status).toBe(200)
      expect(await getNotifications(userId)).toEqual([])
    })

    it('rejects a remove-by-key request with no key', async () => {
      const response = await fetch(`${BASE_URL}/user/${newObjectId()}`, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      })
      expect(response.status).toBe(400)
    })

    it('removes a notification by key alone, across users', async () => {
      const key = `key-${newObjectId()}`
      const userId = newObjectId()
      await addNotification(userId, sampleNotification(key))

      const response = await fetch(`${BASE_URL}/key/${key}`, { method: 'DELETE' })
      expect(response.status).toBe(200)
      expect(await getNotifications(userId)).toEqual([])
    })
  })

  describe('key-scoped operations', () => {
    it('counts unread notifications for a key', async () => {
      const key = `key-${newObjectId()}`
      await addNotification(newObjectId(), sampleNotification(key))
      await addNotification(newObjectId(), sampleNotification(key))

      const response = await fetch(`${BASE_URL}/key/${key}/count`)
      expect(response.status).toBe(200)
      expect(await response.json()).toEqual({ count: 2 })
    })

    it('bulk deletes unread notifications for a key and reports the count', async () => {
      const key = `key-${newObjectId()}`
      const userId = newObjectId()
      await addNotification(userId, sampleNotification(key))
      await addNotification(newObjectId(), sampleNotification(key))

      const response = await fetch(`${BASE_URL}/key/${key}/bulk`, { method: 'DELETE' })
      expect(response.status).toBe(200)
      expect(await response.json()).toEqual({ count: 2 })
      expect(await getNotifications(userId)).toEqual([])

      const after = await fetch(`${BASE_URL}/key/${key}/count`)
      expect(await after.json()).toEqual({ count: 0 })
    })
  })

  describe('validation and routing', () => {
    it('returns 404 for a malformed user id', async () => {
      const response = await fetch(`${BASE_URL}/user/not-an-object-id`)
      expect(response.status).toBe(404)
    })

    it('returns 404 for a malformed notification id', async () => {
      const response = await fetch(
        `${BASE_URL}/user/${newObjectId()}/notification/not-an-object-id`,
        { method: 'DELETE' }
      )
      expect(response.status).toBe(404)
    })

    it('returns 404 for an unknown route', async () => {
      const response = await fetch(`${BASE_URL}/no/such/route`)
      expect(response.status).toBe(404)
    })

    it('reports its status', async () => {
      const response = await fetch(`${BASE_URL}/status`)
      expect(response.status).toBe(200)
      expect(await response.text()).toBe('notifications is up')
    })

    it('exposes prometheus metrics', async () => {
      const response = await fetch(`${BASE_URL}/metrics`)
      expect(response.status).toBe(200)
      expect(await response.text()).toContain('timer_http_request')
    })
  })
})
