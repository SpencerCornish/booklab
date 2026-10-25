// Minimal Mailpit REST client for asserting on delivered email.

export const MAILPIT_URL =
  process.env.FUNCTEST_MAILPIT_URL ?? 'http://localhost:8027'

export interface MailpitSummary {
  ID: string
  Subject: string
}

export async function waitForMessage(
  query: string,
  { timeoutMs = 20_000, expectCount = 1 }: { timeoutMs?: number; expectCount?: number } = {},
): Promise<MailpitSummary> {
  const deadline = Date.now() + timeoutMs
  let lastErr = 'no response'
  while (Date.now() < deadline) {
    const resp = await fetch(`${MAILPIT_URL}/api/v1/search?query=${encodeURIComponent(query)}`)
    if (resp.ok) {
      const data = (await resp.json()) as { total: number; messages: MailpitSummary[] }
      if (data.total >= expectCount && data.messages.length > 0) {
        return data.messages[0]
      }
      lastErr = `total=${data.total}`
    } else {
      lastErr = `HTTP ${resp.status}`
    }
    await new Promise((r) => setTimeout(r, 300))
  }
  throw new Error(`Mailpit: no message matching ${JSON.stringify(query)} within ${timeoutMs}ms (${lastErr})`)
}

export async function messageHtml(id: string): Promise<string> {
  const resp = await fetch(`${MAILPIT_URL}/api/v1/message/${id}`)
  if (!resp.ok) throw new Error(`Mailpit: message ${id} → HTTP ${resp.status}`)
  const data = (await resp.json()) as { HTML?: string; Text?: string }
  return data.HTML ?? data.Text ?? ''
}