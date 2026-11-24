import { test, expect } from '@playwright/test'
import { createServer, type Server } from 'node:http'
import { spawn, type ChildProcess } from 'node:child_process'
import type { AddressInfo } from 'node:net'

// The dev server forwards the brand-dependent files to the API (vite.config.ts). The other
// e2e tests intercept them in the browser, so they would pass with the proxy removed. This one
// starts its own stub API and its own Vite pointed at it (VITE_API_PROXY_TARGET), and sends a
// real request through the proxy, with no page.route.

const PATHS = ['/console/instance.webmanifest', '/console/site.webmanifest', '/console/browserconfig.xml']
const VITE_PORT = 5199

let stub: Server
let vite: ChildProcess
const received: string[] = []

test.describe.configure({ mode: 'serial' })

test.beforeAll(async () => {
  stub = createServer((req, res) => {
    received.push(req.url ?? '')
    res.setHeader('Content-Type', 'application/manifest+json; charset=utf-8')
    res.end(JSON.stringify({ name: 'Stub', path: req.url }))
  })
  await new Promise<void>((resolveListen) => stub.listen(0, '127.0.0.1', resolveListen))
  const { port } = stub.address() as AddressInfo

  vite = spawn('npx', ['vite', '--port', String(VITE_PORT), '--strictPort'], {
    cwd: process.cwd(),
    env: { ...process.env, VITE_API_PROXY_TARGET: `http://127.0.0.1:${port}` },
    stdio: 'pipe'
  })
  await new Promise<void>((resolveReady, rejectReady) => {
    const timer = setTimeout(() => rejectReady(new Error('vite did not start')), 60000)
    vite.stdout?.on('data', (chunk: Buffer) => {
      if (chunk.toString().includes(String(VITE_PORT))) {
        clearTimeout(timer)
        resolveReady()
      }
    })
    vite.on('exit', (code) => rejectReady(new Error(`vite exited with ${code}`)))
  })
})

test.afterAll(async () => {
  vite?.kill('SIGTERM')
  await new Promise<void>((resolveClose) => stub?.close(() => resolveClose()))
})

for (const path of PATHS) {
  test(`forwards ${path} with its /console prefix`, async ({ request }) => {
    const before = received.length
    const response = await request.get(`https://notifusedev.com:${VITE_PORT}${path}`, {
      ignoreHTTPSErrors: true,
      maxRedirects: 0
    })
    expect(response.status()).toBe(200)
    expect(response.headers()['content-type']).toContain('application/manifest+json')
    // The stub's JSON, not the SPA's index.html fallback.
    expect(await response.json()).toEqual({ name: 'Stub', path })
    expect(received.slice(before)).toEqual([path])
  })
}
