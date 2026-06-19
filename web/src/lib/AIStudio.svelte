<script>
  import { api } from './api.js'
  import ScriptEditor from './ScriptEditor.svelte'

  let { book, selectedVoice, isActive = true } = $props()

  let books = $state([])
  let voices = $state([])
  let selectedBookId = $state(book?.id || '')
  let selectedVoiceId = $state(selectedVoice?.id || 'mimo_default')
  
  let selectedChapterIdxs = $state([]) // Array of selected chapter indices for analysis
  
  let analyzing = $state(false)
  let analyzeProgress = $state({ current: 0, total: 0, currentChapter: '' })
  let error = $state('')
  let success = $state('')
  
  let globalCharacters = $state([])
  // Map of chapter index to its script array
  let chapterScripts = $state({})
  // Map of chapter index to its generated audio URLs { chapterIdx: { lineIdx: url } }
  let allAudioUrls = $state({})
  
  let analyzedChapterIdxs = $state([]) // Keeps track of which chapters have been analyzed
  let activeAnalyzedChapterIdx = $state(null)
  
  let generatingVoices = $state(false)
  let genProgress = $state({ current: 0, total: 0, currentChapter: '' })

  async function loadData() {
    try {
      books = await api.get('/api/v1/books')
      voices = [
        ...(await api.get('/api/v1/voices/presets')),
        ...(await api.get('/api/v1/voices')),
      ]
      if (!selectedBookId && books.length > 0) selectedBookId = books[0].id
    } catch(e) { /* ignore */ }
  }

  loadData()

  $effect(() => {
    if (book?.id) {
      if (selectedBookId !== book.id) {
        selectedChapterIdxs = []
        globalCharacters = []
        chapterScripts = {}
        allAudioUrls = {}
        analyzedChapterIdxs = []
        activeAnalyzedChapterIdx = null
      }
      selectedBookId = book.id
    }
    if (selectedVoice?.id) selectedVoiceId = selectedVoice.id
  })

  const selectedBook = $derived(books.find(b => b.id === selectedBookId))

  $effect(() => {
    if (selectedBookId) {
       // Just resetting state when changing book directly
    }
  })

  let wasActive = $state(false)
  $effect(() => {
    if (isActive && !wasActive) {
      loadData()
    }
    wasActive = isActive
  })

  function toggleChapterSelection(idx) {
    if (selectedChapterIdxs.includes(idx)) {
      selectedChapterIdxs = selectedChapterIdxs.filter(i => i !== idx)
    } else {
      selectedChapterIdxs = [...selectedChapterIdxs, idx]
    }
  }

  function toggleSelectAll() {
    if (!selectedBook || !selectedBook.chapters) return
    if (selectedChapterIdxs.length === selectedBook.chapters.length) {
      selectedChapterIdxs = []
    } else {
      selectedChapterIdxs = selectedBook.chapters.map((_, i) => i)
    }
  }

  async function analyzeSingleChapter(chapter, idx) {
    const rawText = (chapter.content || '').slice(0, 15000)
    if (!rawText.trim()) throw new Error('No text content in book.')
    if (rawText.length < 50) {
      throw new Error('Text too short (' + rawText.length + ' chars). The parser may have failed.')
    }

    let sp = `你的任务是将给定小说内容拆分为台词和旁白，输出严格JSON数组。
输出格式：
[
  {"type":"dialogue","role_name":"角色名","text_content":"台词","emotion":"情绪","intensity":"强度","break_duration":0},
  {"type":"dialogue","role_name":"旁白","text_content":"旁白内容...","emotion":"平静","intensity":"中等","break_duration":0}
]
规则：旁白为"旁白"，输出纯JSON数组不含markdown，完整保留原文`
    try {
      const saved = localStorage.getItem('prompts')
      if (saved) {
        const p = JSON.parse(saved)
        if (p.script) sp = p.script
      }
    } catch(e) {}

    const res = await api.post('/api/v1/ai/llm-proxy', {
      system_prompt: sp,
      user_prompt: '请分析以下小说文本：\n\n' + rawText + '\n\n请输出JSON数组。',
    })
    
    let text = res.reply || ''
    if (!text.trim()) {
      throw new Error('LLM returned empty response.')
    }
    text = text.replace(/```json\n?/gi, '').replace(/```/g, '').trim()

    let arr = null
    function fixJson(s) {
      s = s.replace(/,\s*([}\]])/g, '$1')
      s = s.replace(/\\(?!["\\\/bfnrtu0-9])/g, '\\\\')
      return s
    }

    try { arr = JSON.parse(fixJson(text)) } catch (e1) {}

    if (!Array.isArray(arr)) {
      let match = text.match(/\[\s*\{[\s\S]*?\}\s*\]/)
      if (match) try { arr = JSON.parse(fixJson(match[0])) } catch(e) {}
    }
    
    if (!Array.isArray(arr)) {
      const start = text.indexOf('[')
      if (start < 0) throw new Error('No JSON array. Raw: ' + text.slice(0, 300))
      let depth = 0, end = -1, inStr = false
      for (let i = start; i < text.length; i++) {
        const ch = text[i]
        if (inStr) {
          if (ch === '\\') { i++; continue }
          if (ch === '"') inStr = false
          continue
        }
        if (ch === '"') { inStr = true; continue }
        if (ch === '[') depth++
        else if (ch === ']') { depth--; if (depth === 0) { end = i; break } }
      }
      if (end < 0) throw new Error('Unclosed bracket.')
      try { arr = JSON.parse(fixJson(text.slice(start, end + 1))) }
      catch(e2) { throw new Error('JSON parse error') }
    }
    
    if (!Array.isArray(arr)) throw new Error('Not an array')

    const chars = []
    const seen = new Set()
    arr.forEach(item => {
      const name = item.role_name
      if (name && name !== '旁白' && !seen.has(name)) {
        seen.add(name)
        chars.push({ name, role: 'supporting', voice_design: '', gender: '', age: '', personality: '', voice_id: '' })
      }
    })

    const script = arr.map((item, i) => ({
      index: i,
      type: item.type === 'bgm' ? 'bgm' : 'dialogue',
      speaker: item.role_name || '',
      text: item.text_content || item.name || '',
      emotion: item.emotion || '',
      emotion_hint: '',
      scene: 'normal',
      sfx: item.sfx || [],
    }))

    return { chars, script }
  }

  async function aiAnalyzeBatch() {
    if (!selectedBookId) { error = '请选择一本书'; return }
    if (selectedChapterIdxs.length === 0) { error = '请至少选择一章进行分析'; return }
    
    analyzing = true
    error = ''
    success = ''
    analyzeProgress = { current: 0, total: selectedChapterIdxs.length, currentChapter: '' }
    
    // Sort indices to process sequentially
    const sortedIdxs = [...selectedChapterIdxs].sort((a,b)=>a-b)
    
    let tempGlobalChars = [...globalCharacters]
    let tempChapterScripts = {...chapterScripts}
    let tempAnalyzedIdxs = [...analyzedChapterIdxs]

    try {
      for (let i = 0; i < sortedIdxs.length; i++) {
        const idx = sortedIdxs[i]
        const chapter = selectedBook.chapters[idx]
        analyzeProgress.current = i + 1
        analyzeProgress.currentChapter = chapter.title

        const { chars, script } = await analyzeSingleChapter(chapter, idx)
        
        // Merge characters
        chars.forEach(c => {
          if (!tempGlobalChars.find(tc => tc.name === c.name)) {
            tempGlobalChars.push(c)
          }
        })
        
        // Save script
        tempChapterScripts[idx] = script
        if (!tempAnalyzedIdxs.includes(idx)) {
          tempAnalyzedIdxs.push(idx)
        }
        
        // Update reactive state gradually
        globalCharacters = tempGlobalChars
        chapterScripts = tempChapterScripts
        analyzedChapterIdxs = tempAnalyzedIdxs.sort((a,b)=>a-b)
        
        if (activeAnalyzedChapterIdx === null) {
          activeAnalyzedChapterIdx = idx
        }
      }
      success = '批量分析完成！'
    } catch(err) {
      error = '分析过程中断: ' + err.message
    } finally {
      analyzing = false
    }
  }

  async function batchGenerateAllVoices() {
    generatingVoices = true
    error = ''
    success = ''
    
    // Calculate total lines to generate
    let totalLines = 0
    let toGenerate = []
    
    for (const idx of analyzedChapterIdxs) {
      const script = chapterScripts[idx] || []
      for (let i = 0; i < script.length; i++) {
        const line = script[i]
        if (line.type === 'dialogue' && !(allAudioUrls[idx] && allAudioUrls[idx][i])) {
          toGenerate.push({ chapterIdx: idx, lineIdx: i, line })
        }
      }
    }
    
    if (toGenerate.length === 0) {
      success = '所有台词均已生成！'
      generatingVoices = false
      return
    }

    genProgress = { current: 0, total: toGenerate.length, currentChapter: '' }
    
    try {
      for (let i = 0; i < toGenerate.length; i++) {
        const item = toGenerate[i]
        genProgress.current = i + 1
        genProgress.currentChapter = selectedBook.chapters[item.chapterIdx].title
        
        const vp = globalCharacters.find(c => c.name === item.line.speaker)
        const voiceId = vp?.voice_id || selectedVoiceId

        const blob = await api.postBlob('/api/v1/synthesis/single', {
          text: item.line.text,
          voice_id: voiceId,
          emotion_hint: item.line.emotion_hint || `${item.line.emotion || '平静'} 语气`,
          format: 'wav',
        })
        const url = URL.createObjectURL(blob)
        
        // Update allAudioUrls cleanly
        let newUrls = { ...allAudioUrls }
        if (!newUrls[item.chapterIdx]) newUrls[item.chapterIdx] = {}
        newUrls[item.chapterIdx][item.lineIdx] = url
        allAudioUrls = newUrls
      }
      success = '批量配音生成完成！'
    } catch (e) {
      error = '生成配音中断: ' + e.message
    } finally {
      generatingVoices = false
    }
  }

  function blobToBase64(blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onloadend = () => resolve(reader.result.split(',')[1])
      reader.onerror = reject
      reader.readAsDataURL(blob)
    })
  }

  async function exportAllWAV() {
    error = ''
    success = ''
    
    // First ensure all lines are generated
    let missingCount = 0
    for (const idx of analyzedChapterIdxs) {
      const script = chapterScripts[idx] || []
      for (let i = 0; i < script.length; i++) {
        const line = script[i]
        if (line.type === 'dialogue' && !(allAudioUrls[idx] && allAudioUrls[idx][i])) {
          missingCount++
        }
      }
    }
    
    if (missingCount > 0) {
       await batchGenerateAllVoices()
       if (error) { return } // Stop if batch generation failed
    }

    generatingVoices = true
    genProgress = { current: 0, total: 0, currentChapter: '正在读取本地音频并打包...' }
    const scriptData = []
    
    let globalIndex = 0
    for (const idx of analyzedChapterIdxs) {
      const script = chapterScripts[idx] || []
      for (let i = 0; i < script.length; i++) {
        const s = script[i]
        const entry = {
          index: globalIndex++,
          type: s.type,
          speaker: s.speaker,
          text: s.text,
          emotion: s.emotion,
          sfx: s.sfx || [],
        }
        if (s.type === 'dialogue' && allAudioUrls[idx] && allAudioUrls[idx][i]) {
          try {
            const resp = await fetch(allAudioUrls[idx][i])
            const blob = await resp.blob()
            entry.audio_data = await blobToBase64(blob)
          } catch(e) {
            console.warn(`Failed to read audio for chapter ${idx} line ${i}:`, e)
          }
        }
        scriptData.push(entry)
      }
    }

    genProgress = { current: 0, total: 0, currentChapter: '正在云端合成合并所有音频...' }
    try {
      const blob = await api.postMix('/api/v1/synthesis/mix', {
        script: scriptData,
        format: 'wav',
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `full_audiobook_${Date.now()}.wav`
      a.click()
      URL.revokeObjectURL(url)
      success = '✅ 全书合并音频导出成功！'
    } catch (e) {
      error = '导出失败: ' + e.message
    } finally {
      generatingVoices = false
    }
  }
</script>

<div class="ai-studio">
  <h2>智能编辑与分析</h2>

  {#if error}
    <div class="error">{error}</div>
  {/if}
  {#if success}
    <div class="success">{success}</div>
  {/if}

  <div class="form">
    <label>
      <span>选择书籍</span>
      <select bind:value={selectedBookId}>
        <option value="">-- 选择 --</option>
        {#each books as b}
          <option value={b.id}>{b.title} ({b.chapters?.length || 0} 章)</option>
        {/each}
      </select>
    </label>

    {#if selectedBook}
      <div class="book-summary">
        <strong>{selectedBook.title}</strong>
        <p>{selectedBook.author} · {selectedBook.format} · {selectedBook.chapters?.length || 0} 章</p>
      </div>

      <div class="chapter-selection">
        <div class="selection-header">
          <span>选择分析章节</span>
          <button class="btn-small" onclick={toggleSelectAll}>
            {selectedChapterIdxs.length === (selectedBook.chapters?.length || 0) ? '取消全选' : '全选'}
          </button>
        </div>
        <div class="chapter-list">
          {#each selectedBook.chapters || [] as c, i}
            <label class="chapter-item">
              <input type="checkbox" checked={selectedChapterIdxs.includes(i)} onchange={() => toggleChapterSelection(i)} />
              <span class="ch-idx">{i+1}.</span>
              <span class="ch-title">{c.title}</span>
            </label>
          {/each}
        </div>
      </div>
    {/if}

    <button class="ai-btn" disabled={analyzing || selectedChapterIdxs.length === 0} onclick={aiAnalyzeBatch}>
      {#if analyzing}
        🧠 AI 分析中... ({analyzeProgress.current}/{analyzeProgress.total}) - {analyzeProgress.currentChapter}
      {:else}
        🧠 开始批量 AI 分析
      {/if}
    </button>
  </div>

  {#if analyzedChapterIdxs.length > 0}
    <div class="global-characters">
      <h3>全局角色配置池 (共 {globalCharacters.length} 角色)</h3>
      <div class="chars">
        {#each globalCharacters as c}
          <div class="char-card">
            <div class="char-header">
              <strong>{c.name}</strong> <span class="char-role">({c.role})</span>
            </div>
            <select bind:value={c.voice_id}>
              <option value="">-- 跟随默认音色 --</option>
              {#each voices as v}
                <option value={v.id}>{v.name}</option>
              {/each}
            </select>
          </div>
        {/each}
      </div>
    </div>

    <div class="workspace-split">
      <div class="sidebar">
        <h3>已分析章节</h3>
        <ul>
          {#each analyzedChapterIdxs as idx}
            <li class:active={activeAnalyzedChapterIdx === idx} onclick={() => activeAnalyzedChapterIdx = idx}>
              {selectedBook.chapters[idx].title}
            </li>
          {/each}
        </ul>
        <div class="sidebar-actions">
          <button class="batch-gen-btn" disabled={generatingVoices} onclick={batchGenerateAllVoices}>
            {#if generatingVoices && genProgress.total > 0}
              ⏳ 生成中 ({genProgress.current}/{genProgress.total})
            {:else}
              🎙️ 生成所有章节配音
            {/if}
          </button>
          <button class="export-all-btn" disabled={generatingVoices} onclick={exportAllWAV}>
            {#if generatingVoices && genProgress.total === 0}
              ⏳ {genProgress.currentChapter}
            {:else}
              💾 导出全部合并 WAV
            {/if}
          </button>
        </div>
      </div>
      
      <div class="content-area">
        {#if activeAnalyzedChapterIdx !== null && chapterScripts[activeAnalyzedChapterIdx]}
          <div class="chapter-header-info">
            <h3>{selectedBook.chapters[activeAnalyzedChapterIdx].title}</h3>
            <span class="badge">{chapterScripts[activeAnalyzedChapterIdx].length} 段台词</span>
          </div>
          <ScriptEditor 
            script={chapterScripts[activeAnalyzedChapterIdx]} 
            characters={globalCharacters} 
            voices={voices} 
            defaultVoiceId={selectedVoiceId}
            audioUrls={allAudioUrls[activeAnalyzedChapterIdx] || {}}
            onAudioUrlsChange={(newUrls) => {
              allAudioUrls = { ...allAudioUrls, [activeAnalyzedChapterIdx]: newUrls }
            }}
          />
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: 18px;
    margin-bottom: 24px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  label span {
    font-size: 0.85rem;
    color: var(--text-secondary);
    font-weight: 500;
  }
  select, input {
    padding: 10px 14px;
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 0.9rem;
  }
  .book-summary {
    padding: 12px 16px;
    background: var(--bg-card);
    border-radius: 10px;
    border: 1px solid var(--border-color);
  }
  .book-summary strong { display: block; }
  .book-summary p { color: var(--text-secondary); font-size: 0.85rem; margin-top: 4px; }
  
  .chapter-selection {
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 10px;
    padding: 12px 16px;
  }
  .selection-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }
  .selection-header span {
    font-weight: 600;
    font-size: 0.95rem;
  }
  .btn-small {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    color: var(--text-primary);
    padding: 4px 10px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.8rem;
  }
  .chapter-list {
    max-height: 200px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .chapter-item {
    flex-direction: row;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
  }
  .chapter-item:hover {
    background: rgba(255,255,255,0.05);
  }
  .chapter-item input {
    margin: 0;
  }
  .ch-idx { color: var(--text-secondary); font-size: 0.85rem; }
  .ch-title { font-size: 0.9rem; }

  .ai-btn {
    padding: 14px;
    background: linear-gradient(135deg, #f093fb, #f5576c);
    color: #fff;
    border: none;
    border-radius: 10px;
    cursor: pointer;
    font-size: 0.95rem;
    font-weight: 600;
  }
  .ai-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .global-characters {
    margin-top: 20px;
    padding: 16px;
    background: var(--bg-card);
    border-radius: 10px;
    border: 1px solid var(--accent-start);
  }
  .global-characters h3 { color: var(--accent-start); margin-bottom: 12px; font-size: 1.1rem; }
  .chars { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; }
  .char-card {
    padding: 12px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .char-header strong { color: var(--text-primary); }
  .char-role { color: var(--text-secondary); font-size: 0.8rem; }
  .char-card select { padding: 6px; font-size: 0.85rem; }

  .workspace-split {
    display: flex;
    gap: 20px;
    margin-top: 24px;
    align-items: flex-start;
  }
  .sidebar {
    width: 250px;
    flex-shrink: 0;
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 10px;
    padding: 12px;
  }
  .sidebar h3 { font-size: 1rem; margin-bottom: 12px; color: var(--text-primary); }
  .sidebar ul { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 4px; }
  .sidebar li {
    padding: 8px 12px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.9rem;
    color: var(--text-secondary);
    transition: all 0.2s;
  }
  .sidebar li:hover { background: rgba(255,255,255,0.05); color: var(--text-primary); }
  .sidebar li.active { background: var(--accent-start); color: #fff; font-weight: 500; }
  .sidebar-actions { margin-top: 16px; border-top: 1px solid var(--border-color); padding-top: 12px; display: flex; flex-direction: column; gap: 8px; }
  .batch-gen-btn, .export-all-btn {
    width: 100%;
    padding: 10px;
    color: #fff;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85rem;
    font-weight: 600;
  }
  .batch-gen-btn { background: linear-gradient(135deg, #667eea, #764ba2); }
  .export-all-btn { background: linear-gradient(135deg, var(--success), #2a2); }
  .batch-gen-btn:disabled, .export-all-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .content-area {
    flex-grow: 1;
    min-width: 0;
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 10px;
    padding: 16px;
  }
  .chapter-header-info {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
    border-bottom: 1px solid var(--border-color);
    padding-bottom: 12px;
  }
  .chapter-header-info h3 { margin: 0; color: var(--text-primary); }
  .badge { background: var(--bg-secondary); padding: 4px 8px; border-radius: 12px; font-size: 0.8rem; color: var(--text-secondary); }

  .error {
    padding: 10px;
    background: rgba(255,71,87,0.1);
    border: 1px solid var(--danger);
    border-radius: 8px;
    color: var(--danger);
    margin-bottom: 16px;
  }
  .success {
    padding: 10px;
    background: rgba(46,213,115,0.1);
    border: 1px solid var(--success);
    border-radius: 8px;
    color: var(--success);
    margin-bottom: 16px;
  }

  @media (max-width: 768px) {
    .workspace-split { flex-direction: column; }
    .sidebar { width: 100%; max-height: 200px; overflow-y: auto; }
  }
</style>
