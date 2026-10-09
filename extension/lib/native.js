// Messaging with the GoIDM native host (com.goidm.host).

import { ext } from './api.js'

export const HOST = 'com.goidm.host'

/** Maps a native messaging error message (Chrome or Firefox) to a stable code. */
export function classifyError(message = '') {
  if (/not found|no such native application/i.test(message)) return 'host_not_installed'
  if (/forbidden|does not have permission/i.test(message)) return 'host_forbidden'
  if (/exited|disconnected/i.test(message)) return 'host_exited'
  return message || 'unknown_error'
}

/**
 * Sends one message and resolves with the host's response. Never rejects:
 * failures come back as { ok: false, error: <code> }.
 */
export async function send(message) {
  try {
    const response = await ext.runtime.sendNativeMessage(HOST, message)
    return response ?? { ok: false, error: 'empty_response' }
  } catch (e) {
    return { ok: false, error: classifyError(String(e?.message ?? e)) }
  }
}

/** Human-readable explanation for an error code from send(). */
export function describeError(code) {
  switch (code) {
    case 'host_not_installed':
      return 'The GoIDM native host is not installed. In GoIDM open Settings, Browser integration, then Install.'
    case 'host_forbidden':
      return `GoIDM's native host does not allow this extension (ID ${ext.runtime.id}). Reinstall the integration from GoIDM Settings.`
    case 'host_exited':
      return 'The GoIDM native host stopped unexpectedly.'
    case 'app_not_running':
      return "GoIDM isn't running."
    case 'app_launch_failed':
      return 'GoIDM could not be started. Open it manually.'
    default:
      return code
  }
}
