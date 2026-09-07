import { beforeAll, describe, it, expect } from 'vitest'
import { fetchStringWithResponse } from '@overleaf/fetch-utils'
import { BASE_URL, ensureRunning } from './helpers/NotificationsApp.ts'
import './MongoHelper.ts'

describe('HealthCheck endpoint', () => {
  beforeAll(async () => {
    await ensureRunning()
  })
  it('should return 200 for GET /health_check', async () => {
    const { response } = await fetchStringWithResponse(`${BASE_URL}/health_check`)
    expect(response.status).toBe(200)
  })
})
