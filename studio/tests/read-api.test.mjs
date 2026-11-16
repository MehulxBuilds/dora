import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readStudio } from '../src/lib/studio-api.ts'

test('Studio API handles disconnected and malformed responses without leaking parser errors', async () => {
  const original = globalThis.fetch
  try {
    let requested
    globalThis.fetch = async (url) => {
      requested = url
      return new Response(JSON.stringify({ name: 'demo.db', records: [] }), { headers: { 'content-type': 'application/json' } })
    }
    assert.deepEqual(await readStudio('overview', 'nested/demo.db'), { name: 'demo.db', records: [] })
    assert.equal(requested, '/api/overview?db=nested%2Fdemo.db')
    globalThis.fetch = async () => new Response('', { headers: { 'content-type': 'application/json' } })
    await assert.rejects(readStudio('overview'), /empty response.*restart/i)
    globalThis.fetch = async () => new Response('{bad', { headers: { 'content-type': 'application/json' } })
    await assert.rejects(readStudio('overview'), /invalid JSON.*Restart/)
    globalThis.fetch = async () => new Response('<html>Not API</html>', { headers: { 'content-type': 'text/html' } })
    await assert.rejects(readStudio('overview'), /Check the local server/)
    globalThis.fetch = async () => new Response(JSON.stringify({ error: 'Database not found' }), { status: 404, headers: { 'content-type': 'application/json' } })
    await assert.rejects(readStudio('overview'), /Database not found/)
  } finally { globalThis.fetch = original }
})
