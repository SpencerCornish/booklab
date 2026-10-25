// Waits for the BookLab app (started by tests/scripts/run-app.sh) to answer
// /api/health before the E2E specs run.
import { exit } from 'node:process'

const baseURL = process.env.E2E_BASE_URL ?? `http://localhost:${process.env.FUNC_APP_PORT ?? '8090'}`

export default async function globalSetup() {
  const deadline = Date.now() + 60_000
  for (;;) {
    try {
      const resp = await fetch(`${baseURL}/api/health`)
      if (resp.ok) {
        console.log(`e2e: app healthy at ${baseURL}`)
        return
      }
    } catch {
      // not up yet
    }
    if (Date.now() > deadline) {
      console.error(`e2e: app at ${baseURL} never became healthy — is the stack running? (make test-e2e)`)
      exit(1)
    }
    await new Promise((r) => setTimeout(r, 500))
  }
}