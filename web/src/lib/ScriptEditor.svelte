<script>
  import { api } from './api.js'

  let { script = [], characters = [], voices = [] } = $props()

  let generating = $state(false)
  let currentGenerating = $state(-1)
  let audioUrls = $state({})
  let playing = $state(-1)
  let audioElements = $state({})
  let exportProgress = $state('')

  // Generate TTS for one line
  async function generateLine(index) {
    const line = script[index]
    if (!line) return

    const vp = characters.find(c => c.name === line.speaker)
    const voiceId = vp?.voiceFile || 'mimo_default'

    currentGenerating = index
    try {
      const blob = await api.postBlob('/api/v1/synthesis/single', {
        text: line.text,
        voice_id: voiceId,
        emotion_hint: line.emotion_hint || `${line.emotion || '平静'} 语气`,
        format: 'wav',
      })
      const url = URL.createObjectURL(blob)
      audioUrls[index] = url
      audioUrls = { ...audioUrls }
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

    exportProgress = '正在混合音频...'
    try {
      const res = await api.post('/api/v1/synthesis/mix', {
        script: script.map((s, i) => ({
          index: s.index,
          type: s.type,
          speaker: s.speaker,
          text: s.text,
          audio_url: audioUrls[i] || '',
          emotion: s.emotion,
          sfx: s.sfx || [],
        })),
        format: 'wav',
      })
      const blob = await res.blob()
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

  const emotions = ['平静', '开心', '生气', '伤心', '害怕', '厌恶', '低落', '惊喜']
</script>

<div class="script-editor">
  <div class="toolbar">
    <h3>📜 脚本台词 ({script.length} 行)</h3>
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
      <div class="line" class:playing={playing === i} class:has-audio={!!audioUrls[i]}>
        <div class="line-header">
          <span class="line-idx">#{i + 1}</span>
          {#if line.type === 'bgm'}
            <span class="bgm-tag">🎵 BGM: {line.speaker}</span>
          {:else}
            <span class="role-name">{line.speaker || '旁白'}</span>
            <select class="emotion-select">
              {#each emotions as e}
                <option selected={line.emotion === e}>{e}</option>
              {/each}
            </select>
            {#if line.sfx?.length}
              <span class="sfx-tag">🔊 {line.sfx.map(s => s.keyword || s.name).join(', ')}</span>
            {/if}
          {/if}
        </div>
        <div class="line-text">{line.text}</div>
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
              <button onclick={() => generateLine(i)}>🎤 生成配音</button>
            {/if}
            <input type="range" min="0" max="200" value="100" class="vol-slider" title="音量" />
          </div>
        {/if}
      </div>
    {/each}
  </div>
</div>

<style>
  .script-editor {
    margin-top: 16px;
  }
  .toolbar {
    padding: 12px 16px;
    background: #252540;
    border-radius: 10px;
    margin-bottom: 12px;
  }
  .toolbar h3 { margin-bottom: 8px; }
  .tb-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .tb-actions button {
    padding: 8px 16px;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
    color: #fff;
  }
  .tb-actions button:nth-child(1) { background: linear-gradient(135deg, #667eea, #764ba2); }
  .tb-actions button:nth-child(2) { background: linear-gradient(135deg, #4a4, #2a2); }
  .tb-actions button:nth-child(3) { background: #444; }
  .tb-actions button:disabled { opacity: 0.5; cursor: not-allowed; }
  .export-status {
    margin-top: 8px;
    padding: 6px 10px;
    border-radius: 6px;
    font-size: 0.8rem;
    color: #aaa;
    background: #1a1a2e;
  }

  .lines {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .line {
    padding: 10px 14px;
    background: #1a1a2e;
    border-radius: 8px;
    border: 1px solid #2a2a3a;
    transition: all 0.2s;
  }
  .line.playing {
    border-color: #667eea;
    background: #1e1e38;
  }
  .line.has-audio {
    border-left: 3px solid #4a4;
  }

  .line-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
    flex-wrap: wrap;
  }
  .line-idx {
    color: #666;
    font-size: 0.75rem;
    min-width: 24px;
  }
  .role-name {
    font-weight: 600;
    font-size: 0.85rem;
    color: #667eea;
  }
  .bgm-tag {
    font-size: 0.85rem;
    color: #f90;
  }
  .sfx-tag {
    font-size: 0.7rem;
    color: #888;
  }
  .emotion-select {
    padding: 2px 6px;
    background: #252540;
    border: 1px solid #3a3a5a;
    border-radius: 4px;
    color: #aaa;
    font-size: 0.7rem;
  }
  .line-text {
    font-size: 0.9rem;
    color: #ccc;
    line-height: 1.5;
    margin-bottom: 6px;
  }
  .line-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .line-actions button {
    padding: 4px 10px;
    border: 1px solid #3a3a5a;
    background: transparent;
    color: #aaa;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.75rem;
  }
  .line-actions button:hover { border-color: #667eea; color: #fff; }
  .play-btn {
    border-color: #4a4 !important;
    color: #4a4 !important;
  }
  .gen-status {
    font-size: 0.75rem;
    color: #f90;
  }
  .vol-slider {
    width: 60px;
    accent-color: #667eea;
  }
</style>
