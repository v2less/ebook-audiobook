<script>
  import { api } from './api.js'
  import { untrack } from 'svelte'

  let { 
    script = [], 
    characters = [], 
    voices = [], 
    defaultVoiceId = 'mimo_default',
    audioUrls = {},
    onAudioUrlsChange = () => {}
  } = $props()

  let generating = $state(false)
  let currentGenerating = $state(-1)
  let playing = $state(-1)
  let audioElements = $state({})
  let exportProgress = $state('')
  let editingLine = $state(-1)        // which line is being edited (text)
  let editText = $state('')            // temporary edit buffer
  let editingSpeaker = $state(-1)      // which line's speaker is being edited
  let editSpeaker = $state('')

  // Generate TTS for one line
  async function generateLine(index) {
    const line = script[index]
    if (!line) return

    const vp = characters.find(c => c.name === line.speaker)
    const voiceId = vp?.voice_id || defaultVoiceId

    currentGenerating = index
    try {
      const blob = await api.postBlob('/api/v1/synthesis/single', {
        text: line.text,
        voice_id: voiceId,
        emotion_hint: line.emotion_hint || `${line.emotion || '平静'} 语气`,
        format: 'wav',
      })
      const url = URL.createObjectURL(blob)
      onAudioUrlsChange({ ...audioUrls, [index]: url })
    } catch (e) {
      alert('合成失败: ' + e.message)
    } finally {
      currentGenerating = -1
    }
  }

  // Generate all lines
  async function generateAll() {
    generating = true
    for (let i = 0; i < script.length; i++) {
      if (script[i].type === 'dialogue' && !audioUrls[i]) {
        await generateLine(i)
      }
    }
    generating = false
  }

  // Play one line
  function playLine(index) {
    const url = audioUrls[index]
    if (!url) return generateLine(index).then(() => playLine(index))
    if (playing === index) {
      audioElements[index]?.pause()
      playing = -1
      return
    }
    if (audioElements[playing]) {
      audioElements[playing].pause()
    }
    const a = new Audio(url)
    a.onended = () => { playing = -1 }
    a.play()
    audioElements[index] = a
    playing = index
  }

  // Stop all
  function stopAll() {
    Object.values(audioElements).forEach(a => {
      try { a.pause() } catch(e) {}
    })
    playing = -1
  }

  // Stop playing if script changes (e.g. switching chapter)
  $effect(() => {
    if (script) {
      untrack(() => stopAll())
    }
  })

  // ---- Inline Edit: Text ----
  function startEditText(index) {
    editingLine = index
    editText = script[index].text
  }

  function saveEditText(index) {
    if (editText.trim() && editText !== script[index].text) {
      script[index].text = editText.trim()
      // Invalidate cached audio so user regenerates with new text
      if (audioUrls[index]) {
        URL.revokeObjectURL(audioUrls[index])
        const newUrls = { ...audioUrls }
        delete newUrls[index]
        onAudioUrlsChange(newUrls)
      }
    }
    editingLine = -1
    editText = ''
  }

  function cancelEditText() {
    editingLine = -1
    editText = ''
  }

  function handleTextKeydown(e, index) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      saveEditText(index)
    } else if (e.key === 'Escape') {
      cancelEditText()
    }
  }

  // ---- Inline Edit: Speaker ----
  function startEditSpeaker(index) {
    editingSpeaker = index
    editSpeaker = script[index].speaker || '旁白'
  }

  function saveEditSpeaker(index) {
    const newName = editSpeaker.trim()
    if (newName && newName !== script[index].speaker) {
      script[index].speaker = newName
      // Add to characters list if not existing
      if (!characters.find(c => c.name === newName)) {
        characters.push({ name: newName, role: 'supporting', voice_design: '', gender: '', age: '', personality: '' })
      }
      // Invalidate cached audio
      if (audioUrls[index]) {
        URL.revokeObjectURL(audioUrls[index])
        const newUrls = { ...audioUrls }
        delete newUrls[index]
        onAudioUrlsChange(newUrls)
      }
    }
    editingSpeaker = -1
    editSpeaker = ''
  }

  function cancelEditSpeaker() {
    editingSpeaker = -1
    editSpeaker = ''
  }

  function handleSpeakerKeydown(e, index) {
    if (e.key === 'Enter') { e.preventDefault(); saveEditSpeaker(index) }
    else if (e.key === 'Escape') { cancelEditSpeaker() }
  }

  // ---- Inline Edit: Emotion ----
  function updateEmotion(index, emotion) {
    script[index].emotion = emotion
    script[index].emotion_hint = emotion + ' 语气'
    // Invalidate cached audio
    if (audioUrls[index]) {
      URL.revokeObjectURL(audioUrls[index])
      const newUrls = { ...audioUrls }
      delete newUrls[index]
      onAudioUrlsChange(newUrls)
    }
  }

  // Export WAV
  async function exportWAV() {
    exportProgress = '正在合成未生成的音频...'
    generating = true
    for (let i = 0; i < script.length; i++) {
      if (script[i].type === 'dialogue' && !audioUrls[i]) {
        await generateLine(i)
      }
    }
    generating = false

    exportProgress = '正在准备音频数据...'
    // Convert blob URLs to base64 for server-side mixing
    const scriptData = []
    for (let i = 0; i < script.length; i++) {
      const s = script[i]
      const entry = {
        index: s.index || i,
        type: s.type,
        speaker: s.speaker,
        text: s.text,
        emotion: s.emotion,
        sfx: s.sfx || [],
      }
      if (s.type === 'dialogue' && audioUrls[i]) {
        try {
          const resp = await fetch(audioUrls[i])
          const blob = await resp.blob()
          entry.audio_data = await blobToBase64(blob)
        } catch(e) {
          console.warn(`Failed to read audio for line ${i}:`, e)
        }
      }
      scriptData.push(entry)
    }

    exportProgress = '正在混合音频...'
    try {
      const blob = await api.postMix('/api/v1/synthesis/mix', {
        script: scriptData,
        format: 'wav',
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `audiobook_${Date.now()}.wav`
      a.click()
      URL.revokeObjectURL(url)
      exportProgress = '✅ 导出完成'
    } catch (e) {
      exportProgress = '导出失败: ' + e.message
    }
  }

  // Convert Blob to base64 string
  function blobToBase64(blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onloadend = () => resolve(reader.result.split(',')[1])
      reader.onerror = reject
      reader.readAsDataURL(blob)
    })
  }

  const emotions = ['平静', '开心', '生气', '伤心', '害怕', '厌恶', '低落', '惊喜']
</script>

<div class="script-editor">
  <div class="toolbar">
    <h3>📜 脚本台词 ({script.length} 行)</h3>
    <p class="hint">💡 点击台词文本编辑，点击角色名修改说话人，生成后可播放试听</p>
    <div class="tb-actions">
      <button onclick={generateAll} disabled={generating}>
        {generating ? '⏳ 生成中...' : '🎙️ 一键生成配音'}
      </button>
      <button onclick={exportWAV} disabled={generating}>
        💾 导出 WAV
      </button>
      <button onclick={stopAll}>⏹️ 停止播放</button>
    </div>
    {#if exportProgress}
      <div class="export-status">{exportProgress}</div>
    {/if}
  </div>

  <div class="lines">
    {#each script as line, i}
      <div class="line" class:playing={playing === i} class:has-audio={!!audioUrls[i]} class:editing={editingLine === i}>
        <div class="line-header">
          <span class="line-idx">#{i + 1}</span>
          {#if line.type === 'bgm'}
            <span class="bgm-tag">🎵 BGM: {line.speaker}</span>
          {:else}
            <!-- Editable speaker -->
            {#if editingSpeaker === i}
              <input
                class="edit-speaker-input"
                bind:value={editSpeaker}
                onkeydown={(e) => handleSpeakerKeydown(e, i)}
                onblur={() => saveEditSpeaker(i)}
                autofocus
              />
            {:else}
              <span class="role-name editable" onclick={() => startEditSpeaker(i)} title="点击修改说话人">
                {line.speaker || '旁白'} ✎
              </span>
            {/if}

            <!-- Editable emotion -->
            <select
              class="emotion-select"
              value={line.emotion || '平静'}
              onchange={(e) => updateEmotion(i, e.target.value)}
            >
              {#each emotions as e}
                <option value={e}>{e}</option>
              {/each}
            </select>

            {#if line.sfx?.length}
              <span class="sfx-tag">🔊 {line.sfx.map(s => s.keyword || s.name).join(', ')}</span>
            {/if}
          {/if}
        </div>

        <!-- Editable text -->
        {#if editingLine === i}
          <textarea
            class="edit-text-input"
            bind:value={editText}
            onkeydown={(e) => handleTextKeydown(e, i)}
            rows="3"
          ></textarea>
          <div class="edit-actions">
            <button class="save-btn" onclick={() => saveEditText(i)}>💾 保存</button>
            <button class="cancel-btn" onclick={cancelEditText}>取消</button>
          </div>
        {:else}
          <div class="line-text editable" onclick={() => startEditText(i)} title="点击编辑台词">
            {line.text}
          </div>
        {/if}

        {#if line.type === 'dialogue'}
          <div class="line-actions">
            {#if currentGenerating === i}
              <span class="gen-status">⏳ 合成中...</span>
            {:else if audioUrls[i]}
              <button class="play-btn" onclick={() => playLine(i)}>
                {playing === i ? '⏸️ 暂停' : '▶️ 播放'}
              </button>
              <button onclick={() => generateLine(i)}>🔄 重新生成</button>
            {:else}
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .script-editor { margin-top: 16px; }
  .toolbar {
    padding: 12px 16px;
    background: var(--bg-card);
    border-radius: 10px;
    margin-bottom: 12px;
  }
  .toolbar h3 { margin-bottom: 4px; }
  .hint { color: var(--text-secondary); font-size: 0.75rem; margin-bottom: 10px; }
  .tb-actions { display: flex; gap: 8px; flex-wrap: wrap; }
  .tb-actions button {
    padding: 8px 16px; border: none; border-radius: 8px;
    cursor: pointer; font-size: 0.85rem; font-weight: 600; color: #fff;
  }
  .tb-actions button:nth-child(1) { background: linear-gradient(135deg, var(--accent-start), var(--accent-end)); }
  .tb-actions button:nth-child(2) { background: linear-gradient(135deg, var(--success), #2a2); }
  .tb-actions button:nth-child(3) { background: var(--bg-card); border: 1px solid var(--border-color); color: var(--text-primary); }
  .tb-actions button:disabled { opacity: 0.5; cursor: not-allowed; }
  .export-status {
    margin-top: 8px; padding: 6px 10px; border-radius: 6px;
    font-size: 0.8rem; color: var(--text-secondary); background: var(--bg-secondary);
  }

  .lines { display: flex; flex-direction: column; gap: 6px; }
  .line {
    padding: 10px 14px; background: var(--bg-secondary); border-radius: 8px;
    border: 1px solid var(--border-color); transition: all 0.2s;
  }
  .line.playing { border-color: var(--accent-start); background: rgba(102,126,234,0.05); }
  .line.has-audio { border-left: 3px solid var(--success); }
  .line.editing { border-color: var(--warning); }

  .line-header { display: flex; align-items: center; gap: 8px; margin-bottom: 4px; flex-wrap: wrap; }
  .line-idx { color: var(--text-muted); font-size: 0.75rem; min-width: 24px; }
  .role-name {
    font-weight: 600; font-size: 0.85rem; color: var(--accent-start);
  }
  .role-name.editable { cursor: pointer; }
  .role-name.editable:hover { text-decoration: underline; opacity: 0.8; }
  .bgm-tag { font-size: 0.85rem; color: var(--warning); }
  .sfx-tag { font-size: 0.7rem; color: var(--text-secondary); }

  .edit-speaker-input {
    width: 100px; padding: 2px 6px; background: var(--bg-secondary);
    border: 1px solid var(--warning); border-radius: 4px;
    color: var(--text-primary); font-size: 0.85rem; font-weight: 600;
  }

  .emotion-select {
    padding: 2px 6px; background: var(--bg-card);
    border: 1px solid var(--border-color); border-radius: 4px;
    color: var(--text-secondary); font-size: 0.7rem; cursor: pointer;
  }
  .emotion-select:hover { border-color: var(--accent-start); }

  .line-text {
    font-size: 0.9rem; color: var(--text-primary); line-height: 1.5; margin-bottom: 4px;
    padding: 4px 6px; border-radius: 4px; cursor: text;
  }
  .line-text.editable:hover { background: rgba(102,126,234,0.05); }

  .edit-text-input {
    width: 100%; padding: 8px 10px; background: var(--bg-secondary);
    border: 1px solid var(--warning); border-radius: 6px;
    color: var(--text-primary); font-size: 0.9rem; line-height: 1.5;
    font-family: inherit; resize: vertical; margin-bottom: 6px;
  }

  .edit-actions { display: flex; gap: 6px; margin-bottom: 6px; }
  .edit-actions button {
    padding: 4px 12px; border: none; border-radius: 6px;
    cursor: pointer; font-size: 0.75rem;
  }
  .save-btn { background: var(--success); color: #fff; }
  .cancel-btn { background: var(--border-color); color: var(--text-secondary); }

  .line-actions { display: flex; align-items: center; gap: 8px; }
  .line-actions button {
    padding: 4px 10px; border: 1px solid var(--border-color); background: transparent;
    color: var(--text-secondary); border-radius: 6px; cursor: pointer; font-size: 0.75rem;
  }
  .line-actions button:hover { border-color: var(--accent-start); color: var(--text-primary); }
  .play-btn { border-color: var(--success) !important; color: var(--success) !important; }
  .gen-status { font-size: 0.75rem; color: var(--warning); }
</style>
