<script>
  import { api } from './api.js'

  let { onSelect } = $props()

  let presets = $state([])
  let customVoices = $state([])
  let previewText = $state('你好，这是音色预览测试。')
  let previewing = $state(false)
  let showCreate = $state(false)
  let createForm = $state({ name: '', source: 'preset', engine: 'mimo', voice_id: '', description: '', design_prompt: '' })
  let voiceFile = $state(null)
  let error = $state('')

  async function loadVoices() {
    try {
      presets = await api.get('/api/v1/voices/presets')
      customVoices = await api.get('/api/v1/voices')
    } catch(e) { /* ignore */ }
  }

  loadVoices()

  function getVoiceLabel(v) {
    return `${v.name} (${v.language || 'zh-CN'} ${v.gender || ''})`
  }

  async function previewVoice(voice) {
    previewing = true
    try {
      const res = await fetch(`/api/v1/voices/${voice.id}/preview`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: previewText }),
      })
      if (!res.ok) throw new Error('Preview failed')
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = new Audio(url)
      a.play()
    } catch(err) {
      error = err.message
    } finally {
      previewing = false
    }
  }

  async function createVoice() {
    try {
      const vp = await api.uploadVoice('/api/v1/voices', createForm, voiceFile)
      customVoices = [vp, ...customVoices]
      showCreate = false
      voiceFile = null
    } catch(err) {
      error = err.message
    }
  }

  async function deleteVoice(id) {
    await api.del(`/api/v1/voices/${id}`)
    customVoices = customVoices.filter(v => v.id !== id)
  }
</script>

<div class="voice-studio">
  <h2>音色工作室</h2>

  <div class="preview-bar">
    <input bind:value={previewText} placeholder="输入试听文本..." />
  </div>

  {#if error}
    <div class="error">{error}</div>
  {/if}

  <h3 style="margin-bottom: 12px">🎯 预置音色</h3>
  <div class="voice-grid">
    {#each presets as voice}
      <div class="voice-card">
        <div class="voice-head">
          <span class="voice-name">{voice.name}</span>
          <span class="voice-lang">{voice.language}</span>
        </div>
        <span class="voice-detail">{voice.gender} · {voice.source}</span>
        <div class="voice-actions">
          <button disabled={previewing} onclick={() => previewVoice(voice)}>🎧 试听</button>
          <button onclick={() => onSelect?.(voice)}>✅ 选用</button>
        </div>
      </div>
    {/each}
  </div>

  <div style="display: flex; justify-content: space-between; align-items: center; margin: 24px 0 12px">
    <h3>🎨 自定义音色</h3>
    <button class="btn" onclick={() => showCreate = !showCreate}>
      {showCreate ? '取消' : '+ 创建音色'}
    </button>
  </div>

  {#if showCreate}
    <div class="create-form">
      <input bind:value={createForm.name} placeholder="音色名称" />
      <select bind:value={createForm.source}>
        <option value="preset">预置</option>
        <option value="clone">音色复刻</option>
        <option value="design">文本设计</option>
      </select>
      {#if createForm.source === 'preset'}
        <select bind:value={createForm.voice_id}>
          <option value="">选择预置音色...</option>
          {#each presets as p}
            <option value={p.voice_id}>{p.name}</option>
          {/each}
        </select>
      {/if}
      {#if createForm.source === 'clone'}
        <label class="file-label">
          上传参考音频 (MP3/WAV, &lt;10MB)
          <input type="file" accept=".mp3,.wav,audio/mpeg,audio/wav"
            onchange={(e) => voiceFile = e.target.files?.[0] || null}
          />
        </label>
        {#if voiceFile}
          <span class="file-name">✅ {voiceFile.name}</span>
        {/if}
      {/if}
      {#if createForm.source === 'design'}
        <textarea bind:value={createForm.design_prompt}
          placeholder="描述你想要的声音——年龄、性别、音色质感、情绪、语速..." rows="3"></textarea>
      {/if}
      <button onclick={createVoice}>保存音色</button>
    </div>
  {/if}

  {#each customVoices as voice}
    <div class="voice-card">
      <div class="voice-head">
        <span class="voice-name">{voice.name}</span>
        <span class="voice-lang">{voice.language}</span>
      </div>
      <span class="voice-detail">{voice.source} · {voice.engine} · {voice.gender}</span>
      <div class="voice-actions">
        <button disabled={previewing} onclick={() => previewVoice(voice)}>🎧 试听</button>
        <button onclick={() => onSelect?.(voice)}>✅ 选用</button>
        <button class="danger" onclick={() => deleteVoice(voice.id)}>🗑️</button>
      </div>
    </div>
  {/each}
</div>

<style>
  .preview-bar {
    margin-bottom: 16px;
  }
  .preview-bar input {
    width: 100%;
    padding: 10px 14px;
    background: #252540;
    border: 1px solid #3a3a5a;
    border-radius: 8px;
    color: #e0e0e0;
    font-size: 0.9rem;
  }
  .voice-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 10px;
    margin-bottom: 12px;
  }
  .voice-card {
    padding: 14px;
    background: #252540;
    border-radius: 10px;
    border: 1px solid #2a2a3a;
  }
  .voice-head { display: flex; justify-content: space-between; align-items: center; }
  .voice-name { font-weight: 600; }
  .voice-lang { font-size: 0.75rem; color: #888; }
  .voice-detail { color: #888; font-size: 0.8rem; display: block; margin: 4px 0 8px; }
  .voice-actions { display: flex; gap: 6px; }
  .voice-actions button {
    padding: 5px 10px;
    border: 1px solid #3a3a5a;
    background: transparent;
    color: #aaa;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.8rem;
  }
  .voice-actions button:hover { border-color: #667eea; color: #fff; }
  .voice-actions button.danger:hover { border-color: #f55; color: #f55; }

  .create-form {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 16px;
    background: #252540;
    border-radius: 10px;
    margin-bottom: 16px;
  }
  .create-form input, .create-form select, .create-form textarea {
    padding: 10px 14px;
    background: #1a1a2e;
    border: 1px solid #3a3a5a;
    border-radius: 8px;
    color: #e0e0e0;
    font-size: 0.9rem;
  }
  .create-form textarea { resize: vertical; }
  .create-form button {
    padding: 10px;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: #fff;
    border: none;
    border-radius: 8px;
    cursor: pointer;
  }
  .file-label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    color: #aaa;
    font-size: 0.85rem;
    cursor: pointer;
  }
  .file-label input { cursor: pointer; }
  .file-name { color: #4a4; font-size: 0.8rem; }
  .btn {
    padding: 8px 16px;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: #fff;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85rem;
  }
  .error {
    padding: 10px;
    background: rgba(255,100,100,0.1);
    border: 1px solid #f55;
    border-radius: 8px;
    color: #f88;
    margin-bottom: 12px;
  }
</style>
