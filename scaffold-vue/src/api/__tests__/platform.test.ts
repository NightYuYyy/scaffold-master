import test from 'node:test'
import assert from 'node:assert/strict'
import { createHttpClient } from '../http.ts'
import { createPlatformApi } from '../platform.ts'

test('platform api maps every management endpoint in docs/platform-api.md', async () => {
  const calls: Array<{ url: string; method: string; body?: string }> = []
  const fetcher = async (url: string | URL | Request, init?: RequestInit) => {
    calls.push({ url: String(url), method: init?.method ?? 'GET', body: init?.body as string | undefined })
    if (String(url).includes('/export')) {
      return new Response('编码,名称\ndemo,示例应用\n', { status: 200, headers: { 'content-type': 'text/csv;charset=UTF-8' } })
    }
    return new Response(JSON.stringify({ code: 200, message: 'success', data: { id: 7, rows: [], total: 0, apiKey: 'demo_x', apiSecret: 's' }, traceId: 't' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    })
  }
  const api = createPlatformApi(createHttpClient({ baseUrl: '/api', tokenProvider: () => 'token', fetcher }))

  await api.apps.list({ keyword: 'demo', status: 1, pageNum: 1, pageSize: 10 })
  await api.apps.options()
  await api.apps.get(1)
  await api.apps.create({ code: 'demo', name: '示例应用', allowRegister: 1 })
  await api.apps.update(1, { name: '示例应用 2' })
  await api.apps.updateStatus(1, 0)
  await api.apps.remove(1)
  const csv = await api.apps.export({ keyword: 'demo' })
  await api.apps.keys.list(1)
  const created = await api.apps.keys.create(1, '本地联调')
  await api.apps.keys.updateStatus(1, 2, 0)
  await api.apps.keys.remove(1, 2)
  await api.users.list({ appId: 1, keyword: 'alice', status: '' })
  await api.users.get(3)
  await api.users.create({ appId: 1, username: 'alice', profile: { level: 'gold' } })
  await api.users.update(3, { nickname: 'Alice' })
  await api.users.updateStatus(3, 0)
  await api.users.resetPassword(3, 'secret1')
  await api.users.remove(3)
  await api.users.adjustCounter(3, 'coins', { amount: -10, reason: '兑换' })
  await api.users.export({ appId: 1 })
  await api.fields.list(1)
  await api.fields.create({ appId: 1, key: 'level', label: '等级', type: 'enum', options: ['a', 'b'] })
  await api.fields.update(5, { label: '会员等级' })
  await api.fields.remove(5)
  await api.counterLogs.list({ appId: 1, userId: 3, fieldKey: 'coins' })
  await api.counterLogs.export({ appId: 1 })
  await api.authLogs.list({ appId: 1, event: 'login', status: 0 })
  await api.authLogs.export({ appId: 1 })

  assert.equal(await csv.text(), '编码,名称\ndemo,示例应用\n')
  assert.equal(created.apiSecret, 's')
  assert.deepEqual(calls.map(c => `${c.method} ${c.url}`), [
    'GET /api/platform/apps?keyword=demo&status=1&pageNum=1&pageSize=10',
    'GET /api/platform/apps/options',
    'GET /api/platform/apps/1',
    'POST /api/platform/apps',
    'PUT /api/platform/apps/1',
    'PUT /api/platform/apps/1/status',
    'DELETE /api/platform/apps/1',
    'GET /api/platform/apps/export?keyword=demo',
    'GET /api/platform/apps/1/keys',
    'POST /api/platform/apps/1/keys',
    'PUT /api/platform/apps/1/keys/2/status',
    'DELETE /api/platform/apps/1/keys/2',
    'GET /api/platform/users?appId=1&keyword=alice',
    'GET /api/platform/users/3',
    'POST /api/platform/users',
    'PUT /api/platform/users/3',
    'PUT /api/platform/users/3/status',
    'PUT /api/platform/users/3/password',
    'DELETE /api/platform/users/3',
    'POST /api/platform/users/3/counters/coins/adjust',
    'GET /api/platform/users/export?appId=1',
    'GET /api/platform/fields?appId=1',
    'POST /api/platform/fields',
    'PUT /api/platform/fields/5',
    'DELETE /api/platform/fields/5',
    'GET /api/platform/counter-logs?appId=1&userId=3&fieldKey=coins',
    'GET /api/platform/counter-logs/export?appId=1',
    'GET /api/platform/auth-logs?appId=1&event=login&status=0',
    'GET /api/platform/auth-logs/export?appId=1',
  ])
  assert.deepEqual(JSON.parse(calls[5].body ?? '{}'), { status: 0 })
  assert.deepEqual(JSON.parse(calls[9].body ?? '{}'), { name: '本地联调' })
  assert.deepEqual(JSON.parse(calls[19].body ?? '{}'), { amount: -10, reason: '兑换' })
  assert.deepEqual(JSON.parse(calls[22].body ?? '{}'), { appId: 1, key: 'level', label: '等级', type: 'enum', options: ['a', 'b'] })
})
