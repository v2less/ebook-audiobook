// Helper: extract error message from various error response formats
function extractError(errBody) {
  if (!errBody) return 'Unknown error'
  // New format: {"error":{"code":"...","message":"..."}}
  if (errBody.error && typeof errBody.error === 'object' && errBody.error.message) {
    return errBody.error.message
  }
  // Old format: {"error":"string"}
  if (typeof errBody.error === 'string') {
    return errBody.error
  }
  // Plain message field
  if (errBody.message) return errBody.message
  return JSON.stringify(errBody).slice(0, 200)
}

export const api = {
  async get(path) {
    const res = await fetch(path)
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    return res.json()
  },

  async post(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    return res.json()
  },

  async upload(path, file) {
    const form = new FormData()
    form.append('file', file)
    const res = await fetch(path, { method: 'POST', body: form })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    return res.json()
  },

  async del(path) {
    const res = await fetch(path, { method: 'DELETE' })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    return res.json()
  },

  async put(path, body) {
    const res = await fetch(path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    return res.json()
  },

  async uploadVoice(path, formData, file) {
    const form = new FormData()
    form.append('name', formData.name || '')
    form.append('source', formData.source || 'preset')
    form.append('engine', formData.engine || 'mimo')
    form.append('voice_id', formData.voice_id || '')
    form.append('design_prompt', formData.design_prompt || '')
    form.append('description', formData.description || '')
    if (file) {
      form.append('voice_file', file)
    }
    const res = await fetch(path, { method: 'POST', body: form })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    return res.json()
  },

  async postRaw(path, body, responseType) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    if (responseType === 'blob') return res.blob()
    return res.arrayBuffer()
  },

  async postBlob(path, body, timeoutMs = 600000) {
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), timeoutMs)
    try {
      const res = await fetch(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal: controller.signal,
      })
      if (!res.ok) {
        const errBody = await res.json().catch(() => ({}))
        throw new Error(extractError(errBody) || res.statusText)
      }
      return res.blob()
    } finally {
      clearTimeout(timer)
    }
  },

  async postMix(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) {
      const errBody = await res.json().catch(() => ({}))
      throw new Error(extractError(errBody) || res.statusText)
    }
    // Returns audio blob directly
    const contentType = res.headers.get('Content-Type') || ''
    if (contentType.includes('json')) {
      throw new Error((await res.json()).error?.message || 'Mix failed')
    }
    return res.blob()
  },
}
