<script>
  import { api } from './api.js'
  import ScriptEditor from './ScriptEditor.svelte'

  let { book, selectedVoice, isActive = true } = $props()

  let books = $state([])
  let voices = $state([])
  let selectedBookId = $state(book?.id || '')
  let selectedChapterIdx = $state(0)
  let selectedVoiceId = $state(selectedVoice?.id || 'mimo_default')
  let outputFormat = $state('mp3')
  let chapterGap = $state(1.5)
  let styleDirective = $state('')
  let submitting = $state(false)
  let analyzing = $state(false)
  let error = $state('')
  let success = $state('')
  let analysis = $state(null)

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
      if (selectedBookId !== book.id) selectedChapterIdx = 0
      selectedBookId = book.id
    }
    if (selectedVoice?.id) selectedVoiceId = selectedVoice.id
  })

  const selectedBook = $derived(books.find(b => b.id === selectedBookId))

  $effect(() => {
    // Reset chapter index when book changes in dropdown
    if (selectedBookId) {
       // Keep reactivity clean, just ensure selectedChapterIdx is valid
       if (selectedBook && selectedBook.chapters && selectedChapterIdx >= selectedBook.chapters.length) {
         selectedChapterIdx = 0
       }
    }
  })

  let wasActive = $state(false)
  $effect(() => {
    if (isActive && !wasActive) {
      loadData()
    }
    wasActive = isActive
  })

  async function startSynthesis() {
    if (!selectedBookId) { error = '请选择一本书'; return }
    submitting = true
    error = ''
    success = ''

    try {
      const job = await api.post('/api/v1/jobs', {
        book_id: selectedBookId,
        config: {
          default_voice_id: selectedVoiceId,
          output_format: outputFormat,
          chapter_gap: chapterGap,
          merge_chapters: true,
          tts_options: {
            voice_id: selectedVoiceId,
            format: 'wav',
            style_directive: styleDirective,
          },
        },
      })
      success = `任务已创建！ID: ${job.id.slice(0, 8)}...`
    } catch(err) {
      error = err.message
    } finally {
      submitting = false
    }
  }

  async function aiAnalyze() {
    if (!selectedBookId) { error = '请选择一本书'; return }
    analyzing = true
    error = ''
    analysis = null
    try {
      const rawText = (selectedBook?.chapters?.[selectedChapterIdx]?.content || '').slice(0, 15000)
      if (!rawText.trim()) throw new Error('No text content in book. The parser may have failed to extract text.')

      // Warn if text looks garbled
      if (rawText.length < 50) {
        throw new Error('Text too short (' + rawText.length + ' chars). The parser may have failed.')
      }

      // Load prompt from localStorage or use default
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
	        throw new Error('LLM returned empty response. Please check: 1) API Key is correct 2) Model is available 3) Prompt is appropriate')
	      }
	      text = text.replace(/```json\n?/gi, '').replace(/```/g, '').trim()

      // Try to parse as JSON directly first
      let arr = null

      function fixJson(s) {
        // Remove trailing commas before ] or }
        s = s.replace(/,\s*([}\]])/g, '$1')
        // Replace \ not followed by valid JSON escape chars with \\
        s = s.replace(/\\(?!["\\\/bfnrtu0-9])/g, '\\\\')
        return s
      }

      try { arr = JSON.parse(fixJson(text)) }
      catch (e1) { /* try regex below */ }

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
            // Skip escaped characters inside strings (e.g. \", \\, \n)
            if (ch === '\\') { i++; continue }
            if (ch === '"') inStr = false
            continue
          }
          if (ch === '"') { inStr = true; continue }
          if (ch === '[') depth++
          else if (ch === ']') { depth--; if (depth === 0) { end = i; break } }
        }
        if (end < 0) throw new Error('Unclosed bracket. Raw: ' + text.slice(0, 300))
        try { arr = JSON.parse(fixJson(text.slice(start, end + 1))) }
        catch(e2) { throw new Error('JSON parse error: ' + e2.message + ' | Near: ' + text.slice(start, start + 200)) }
      }
      if (!Array.isArray(arr)) throw new Error('Not an array')

      // Build characters from unique role_name
      const chars = []
      const seen = new Set()
      arr.forEach(item => {
        const name = item.role_name
        if (name && name !== '旁白' && !seen.has(name)) {
          seen.add(name)
          chars.push({ name, role: 'supporting', voice_design: '', gender: '', age: '', personality: '', voice_id: '' })
        }
      })

      analysis = {
        characters: chars,
        script: arr.map((item, i) => ({
          index: i,
          type: item.type === 'bgm' ? 'bgm' : 'dialogue',
          speaker: item.role_name || '',
          text: item.text_content || item.name || '',
          emotion: item.emotion || '',
          emotion_hint: '',
          scene: 'normal',
          sfx: item.sfx || [],
        })),
        bgm_timeline: [],
      }
    } catch(err) {
      error = '分析失败: ' + err.message
    } finally {
      analyzing = false
    }
  }
</script>

<div class="synthesis-config">
  <h2>合成配置</h2>

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
      <label>
        <span>选择分析章节</span>
        <select bind:value={selectedChapterIdx}>
          {#each selectedBook.chapters || [] as c, i}
            <option value={i}>{c.title} ({c.content?.length || 0} 字)</option>
          {/each}
        </select>
      </label>

      <div class="book-summary">
        <strong>{selectedBook.title}</strong>
        <p>{selectedBook.author} · {selectedBook.format} · {selectedBook.chapters?.length || 0} 章</p>
      </div>
    {/if}

    <label>
      <span>默认音色</span>
      <select bind:value={selectedVoiceId}>
        {#each voices as v}
          <option value={v.id}>{v.name} ({v.language || 'zh'})</option>
        {/each}
      </select>
    </label>

    <label>
      <span>输出格式</span>
      <select bind:value={outputFormat}>
        <option value="mp3">MP3</option>
        <option value="wav">WAV</option>
        <option value="m4b">M4B (有声书格式)</option>
      </select>
    </label>

    <label>
      <span>章节间隔 (秒)</span>
      <input type="number" bind:value={chapterGap} min="0" max="10" step="0.5" />
    </label>

    <label>
      <span>风格指令（可选，自然语言描述）</span>
      <textarea bind:value={styleDirective}
        rows="3"
        placeholder="例如：温柔舒缓的年轻女声，用播音员风格朗读，中速偏慢，情绪平和如同夜读..."></textarea>
    </label>

    <button class="ai-btn" disabled={analyzing} onclick={aiAnalyze}>
      {analyzing ? '🧠 AI 分析中...' : '🧠 AI 智能分析 (角色/情绪/音效)'}
    </button>

    <button class="submit" disabled={submitting} onclick={startSynthesis}>
      {submitting ? '正在创建任务...' : '🚀 开始合成'}
    </button>

    {#if analysis}
      <div class="analysis-result">
        <h3>AI 分析结果</h3>
        <p>角色: {analysis.characters?.length || 0} 个 | 台词段: {analysis.script?.length || 0} 段</p>
        {#if analysis.characters?.length}
          <div class="chars">
            {#each analysis.characters as c}
              <div class="char-card">
                <div class="char-header">
                  <strong>{c.name}</strong> <span class="char-role">({c.role})</span>
                </div>
                <div class="char-desc" title={c.voice_design}>{c.voice_design?.slice(0, 40)}...</div>
                <select bind:value={c.voice_id}>
                  <option value="">-- 跟随默认音色 --</option>
                  {#each voices as v}
                    <option value={v.id}>{v.name}</option>
                  {/each}
                </select>
              </div>
            {/each}
          </div>
        {/if}
      </div>
      <ScriptEditor script={analysis.script || []} characters={analysis.characters || []} voices={voices} defaultVoiceId={selectedVoiceId} />
    {/if}
  </div>
</div>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: 18px;
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
  select, input, textarea {
    padding: 10px 14px;
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    color: var(--text-primary);
    font-size: 0.9rem;
  }
  textarea { resize: vertical; }
  .book-summary {
    padding: 12px 16px;
    background: var(--bg-card);
    border-radius: 10px;
    border: 1px solid var(--border-color);
  }
  .book-summary strong { display: block; }
  .book-summary p { color: var(--text-secondary); font-size: 0.85rem; margin-top: 4px; }
  .submit {
    padding: 14px;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: #fff;
    border: none;
    border-radius: 10px;
    cursor: pointer;
    font-size: 1rem;
    font-weight: 600;
    transition: opacity 0.2s;
  }
  .submit:disabled { opacity: 0.5; cursor: not-allowed; }
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
  .analysis-result {
    margin-top: 16px;
    padding: 16px;
    background: var(--bg-card);
    border-radius: 10px;
    border: 1px solid var(--accent-start);
  }
  .analysis-result h3 { color: var(--accent-start); margin-bottom: 8px; }
  .chars { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; margin-top: 10px; }
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
  .char-desc { font-size: 0.8rem; color: var(--text-muted); }
  .char-card select { padding: 6px; font-size: 0.85rem; }
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
</style>
