// worker.ts
import { Todo, ReducerAction } from './types'

async function httpPostJson(url: string, body: any, opts?: RequestInit) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    ...opts,
  })
  if (!res.ok) {
    const text = await res.text().catch(() => '<no body>')
    const err = new Error(`HTTP ${res.status}: ${text}`)
    throw err
  }
  return res
}

export async function sendEvent(event: { type: string; eventId?: string; payload?: any }) {
  try {
    await httpPostJson('/api/events', event)
  } catch (e) {
    console.error('worker sendEvent failed', e)
    throw e
  }
}

/**
 * Send an event to server and update reducer state accordingly.
 * - If `pending` is provided, it's dispatched as `ADD_PENDING`.
 * - On failure, pending is removed.
 */
export async function sendAndHandle(
  event: { type: string; eventId?: string; payload?: any },
  dispatch: (action: ReducerAction) => void,
  pending?: Todo
) {
  if (pending) {
    dispatch({ type: 'ADD_PENDING', todo: pending })
  }

  try {
    await sendEvent(event)
  } catch (e) {
    // on failure remove pending if provided
    if (pending) {
      dispatch({ type: 'REMOVE', id: pending.id })
    }
    throw e
  }
}
