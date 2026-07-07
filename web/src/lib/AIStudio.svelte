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

  // 自动生成音色相关状态
  let generatingDesignFor = $state({}) // { charName: true } 正在生成的角色
  let batchGeneratingDesign = $state(false)
  let playingVoiceId = $state(null)
  let audioPlayer = null

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
    if (voices.length > 0 && globalCharacters.length > 0) {
      globalCharacters.forEach(c => {
        if (!c.voice_id) {
          const matched = voices.find(v => v.name === c.name || v.id === c.name)
          if (matched) {
            c.voice_id = matched.id
          }
        }
      })
    }
  })

  let lastBookId = $state('')
  $effect(() => {
    if (book?.id) {
      selectedBookId = book.id
    }
    if (selectedVoice?.id) selectedVoiceId = selectedVoice.id
  })

  $effect(() => {
    if (selectedBookId !== lastBookId) {
      lastBookId = selectedBookId
      selectedChapterIdxs = []
      globalCharacters = [
        { name: '旁白', role: 'narrator', voice_design: '小说的主干旁白叙述', gender: 'neutral', age: 'adult', personality: 'calm', voice_id: '' }
      ]
      chapterScripts = {}
      allAudioUrls = {}
      analyzedChapterIdxs = []
      activeAnalyzedChapterIdx = null
      generatingDesignFor = {}
      // 加载已保存的角色-音色绑定
      if (selectedBookId) loadBookRoleVoices(selectedBookId)
    }
  })

  async function loadBookRoleVoices(bookId) {
    try {
      const roleVoices = await api.get(`/api/v1/books/${bookId}/role-voices`)
      if (roleVoices && typeof roleVoices === 'object') {
        globalCharacters.forEach(c => {
          if (roleVoices[c.name]) {
            c.voice_id = roleVoices[c.name]
          }
        })
      }
    } catch(e) { /* 首次使用可能没有绑定 */ }
  }

  async function saveBookRoleVoices() {
    if (!selectedBookId) return
    const roleVoices = {}
    globalCharacters.forEach(c => {
      if (c.voice_id) roleVoices[c.name] = c.voice_id
    })
    try {
      await api.put(`/api/v1/books/${selectedBookId}/role-voices`, roleVoices)
    } catch(e) { /* ignore */ }
  }

  const selectedBook = $derived(books.find(b => b.id === selectedBookId))

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

  // 将文本在句末标点处断开，每段不超过 maxLen 字符
  function splitTextAtSentence(text, maxLen) {
    if (text.length <= maxLen) return [text]
    const SENTENCE_ENDS = /[。！？!?\n]{1,3}/g
    const chunks = []
    let lastEnd = 0
    let match
    while ((match = SENTENCE_ENDS.exec(text)) !== null) {
      const end = match.index + match[0].length
      if (end - lastEnd >= maxLen) {
        chunks.push(text.slice(lastEnd, match.index + 1))
        lastEnd = match.index + 1
      }
    }
    if (lastEnd < text.length) {
      chunks.push(text.slice(lastEnd))
    }
    // Merge small trailing chunks
    if (chunks.length > 1 && chunks[chunks.length - 1].length < 200) {
      chunks[chunks.length - 2] += chunks.pop()
    }
    return chunks.length === 0 ? [text] : chunks
  }

  // 解析 LLM 返回的 JSON 数组（含截断恢复）
  function parseScriptArray(text) {
    const rawPreview = text.slice(0, 200).replace(/\n/g, '\\n')
    function fixJson(s) {
      s = s.replace(/,\s*([}\]])/g, '$1')
      s = s.replace(/\\(?!["\\\/bfnrtu0-9])/g, '\\\\')
      return s
    }

    let arr = null
    let parseErr = ''
    try { arr = JSON.parse(fixJson(text)) } catch (e1) { parseErr = e1.message }

    if (!Array.isArray(arr)) {
      let match = text.match(/\[\s*\{[\s\S]*?\}\s*\]/)
      if (match) try { arr = JSON.parse(fixJson(match[0])) } catch(e) {}
    }
    
    if (!Array.isArray(arr)) {
      const start = text.indexOf('[')
      if (start < 0) throw new Error('No JSON array found. LLM: ' + rawPreview)
      let depth = 0, end = -1, inStr = false
      for (let i = start; i < text.length; i++) {
        const ch = text[i]
        if (inStr) { if (ch === '\\') { i++; continue }; if (ch === '"') inStr = false; continue }
        if (ch === '"') { inStr = true; continue }
        if (ch === '[') depth++
        else if (ch === ']') { depth--; if (depth === 0) { end = i; break } }
      }
      if (end < 0) {
        const lastObj = text.lastIndexOf('}')
        if (lastObj > start) {
          let recovered = text.slice(start, lastObj + 1)
          if ((recovered.match(/"/g) || []).length % 2 !== 0) recovered += '"'
          recovered += ']'
          try { arr = JSON.parse(fixJson(recovered)) } catch(e) {}
        }
        if (!Array.isArray(arr)) throw new Error('Response truncated. LLM: ' + rawPreview)
      } else {
        try { arr = JSON.parse(fixJson(text.slice(start, end + 1))) }
        catch(e2) { throw new Error('JSON parse error' + (parseErr ? ' (' + parseErr + ')' : '') + '. LLM: ' + rawPreview) }
      }
    }
    
    if (!Array.isArray(arr)) throw new Error('Not an array')
    return arr
  }

  async function analyzeSingleChapter(chapter, idx) {
    const titlePrefix = chapter.title ? chapter.title + '\n\n' : ''
    const fullText = titlePrefix + (chapter.content || '')
    if (!fullText.trim()) throw new Error('No text content in book.')
    if (fullText.length < 50) {
      throw new Error('Text too short (' + fullText.length + ' chars). The parser may have failed.')
    }

    // 将长文本按 ~8000 字符分块（在句末标点处断开，避免截断句子）
    const MAX_CHUNK = 8000
    const chunks = splitTextAtSentence(fullText, MAX_CHUNK)

    let sp = `你的任务是将给定小说内容拆分为台词和旁白，输出严格JSON数组。
输出格式：
[
  {"type":"dialogue","role_name":"角色名","text_content":"台词","emotion":"情绪","intensity":"强度","break_duration":0},
  {"type":"dialogue","role_name":"旁白","text_content":"旁白内容...","emotion":"平静","intensity":"中等","break_duration":0}
]
规则：
1. 旁白的角色名固定为"旁白"
2. 直接输出纯JSON数组，不要用markdown代码块（不要\`\`\`json）
3. 完整保留原文内容，不要省略`
    try {
      const saved = localStorage.getItem('prompts')
      if (saved) {
        const p = JSON.parse(saved)
        if (p.script) sp = p.script
      }
    } catch(e) {}

    // Process each chunk and merge results
    const allScripts = []
    const allChars = []
    const seenCharNames = new Set()

    for (let ci = 0; ci < chunks.length; ci++) {
      const chunk = chunks[ci]
      if (!chunk.trim()) continue

      const res = await api.post('/api/v1/ai/llm-proxy', {
        system_prompt: sp,
        user_prompt: (chunks.length > 1 
          ? `（第${ci+1}/${chunks.length}段）请分析以下文本并输出JSON数组：\n\n` 
          : '请分析以下文本并输出JSON数组：\n\n') + chunk,
      })
      
      let text = res.reply || ''
      if (!text.trim()) {
        throw new Error('LLM returned empty response (chunk ' + (ci+1) + '/' + chunks.length + ').')
      }
      text = text.replace(/```json\n?/gi, '').replace(/```/g, '').trim()

      const arr = parseScriptArray(text)
      if (arr.length === 0) {
        throw new Error('LLM returned empty array (chunk ' + (ci+1) + '/' + chunks.length + ').')
      }

      // Merge characters
      arr.forEach(item => {
        const name = item.role_name
        if (name && !seenCharNames.has(name)) {
          seenCharNames.add(name)
          allChars.push({
            name,
            role: name === '旁白' ? 'narrator' : 'supporting',
            voice_design: item.voice_design || '',
            gender: item.gender || '',
            age: item.age || '',
            personality: item.personality || '',
            voice_id: ''
          })
        }
      })

      allScripts.push(...arr)
    }

    // Build final merged script with correct indexing
    const script = allScripts.map((item, i) => ({
      index: i,
      type: item.type === 'bgm' ? 'bgm' : 'dialogue',
      speaker: item.role_name || '',
      text: item.text_content || item.name || '',
      emotion: item.emotion || '',
      emotion_hint: '',
      scene: 'normal',
    }))

    return { chars: allChars, script }
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
      // 分析完成后，尝试加载已保存的角色-音色绑定
      if (selectedBookId) {
        try {
          const roleVoices = await api.get(`/api/v1/books/${selectedBookId}/role-voices`)
          if (roleVoices && typeof roleVoices === 'object') {
            globalCharacters.forEach(c => {
              if (roleVoices[c.name] && !c.voice_id) {
                c.voice_id = roleVoices[c.name]
              }
            })
          }
        } catch(e) { /* ignore */ }
      }
      
      // 为没有音色描述的角色生成描述
      analyzeProgress.currentChapter = '正在为角色生成音色描述...'
      await generateVoiceDesignsForCharacters()

      success = '批量分析完成！'
    } catch(err) {
      error = '分析过程中断: ' + err.message
    } finally {
      analyzing = false
    }
  }

  async function generateVoiceDesignsForCharacters() {
    const charsWithoutDesign = globalCharacters.filter(c => c.name !== '旁白' && !c.voice_design)
    if (charsWithoutDesign.length === 0) return

    // 收集前几个章节的文本作为上下文
    let contextText = ''
    for (const idx of analyzedChapterIdxs.slice(0, 3)) {
      contextText += (selectedBook.chapters[idx].content || '').slice(0, 3000) + '\n\n'
    }

    const charNames = charsWithoutDesign.map(c => c.name).join('、')
    const sp = `你是一个专业的有声书导演。请根据给出的小说内容，为以下角色设计音色（voice_design）。
要求输出JSON数组，格式如下：
[
  {
    "name": "角色名",
    "gender": "male/female/neutral",
    "age": "young/middle-aged/elderly",
    "personality": "性格特点",
    "voice_design": "用于TTS合成的音色提示词，描述该角色的音色特点，如：20多岁年轻男性，声音清澈阳光，语速适中，带有学生气。"
  }
]
确保只输出严格的JSON数组，不要任何Markdown和其他解释。
需要设计音色的角色：${charNames}`

    try {
      const res = await api.post('/api/v1/ai/llm-proxy', {
        system_prompt: sp,
        user_prompt: '小说内容节选：\n' + contextText.slice(0, 8000),
      })
      let text = res.reply || ''
      text = text.replace(/```json\n?/gi, '').replace(/```/g, '').trim()
      
      let arr = null
      function fixJson(s) {
        s = s.replace(/,\s*([}\]])/g, '$1')
        s = s.replace(/\\(?!["\\\/bfnrtu0-9])/g, '\\\\')
        return s
      }
      try { arr = JSON.parse(fixJson(text)) } catch(e1) {
        let match = text.match(/\[\s*\{[\s\S]*?\}\s*\]/)
        if (match) try { arr = JSON.parse(fixJson(match[0])) } catch(e) {}
      }

      if (Array.isArray(arr)) {
        globalCharacters = globalCharacters.map(c => {
          const design = arr.find(d => d.name === c.name)
          if (design) {
            return {
              ...c,
              voice_design: design.voice_design || c.voice_design,
              gender: design.gender || c.gender,
              age: design.age || c.age,
              personality: design.personality || c.personality,
            }
          }
          return c
        })
      }
    } catch(e) {
      console.warn('生成角色音色描述失败:', e)
    }
  }

  // ---- 自动生成音色 ----

  async function generateVoiceForChar(c) {
    if (!c.voice_design) {
      error = `角色 ${c.name} 没有 voice_design 描述，无法自动生成音色`
      return
    }
    generatingDesignFor = { ...generatingDesignFor, [c.name]: true }
    error = ''
    try {
      const res = await api.post('/api/v1/voices/generate-design', {
        design_prompt: c.voice_design,
        preview_text: '你好，这是为角色' + c.name + '自动生成的音色测试。',
        save_as_name: c.name,
        gender: c.gender || '',
      })
      // 绑定到角色
      c.voice_id = res.voice_profile.id
      // 将新生成的音色加入 voices 列表
      voices = [...voices, res.voice_profile]
      // 持久化绑定
      await saveBookRoleVoices()
      success = `角色 ${c.name} 的音色已生成并固定！`
    } catch(e) {
      error = `生成 ${c.name} 音色失败: ${e.message}`
    } finally {
      generatingDesignFor = { ...generatingDesignFor, [c.name]: false }
    }
  }

  async function batchGenerateDesignVoices() {
    // 检查是否有角色需要音色但缺少 voice_design
    const missingDesign = globalCharacters.filter(c => !c.voice_id && !c.voice_design && c.name !== '旁白')
    if (missingDesign.length > 0) {
      batchGeneratingDesign = true
      genProgress = { current: 0, total: 1, currentChapter: '正在让AI构思角色声音特点...' }
      await generateVoiceDesignsForCharacters()
      batchGeneratingDesign = false
    }

    const needGen = globalCharacters.filter(c => !c.voice_id && c.voice_design && c.name !== '旁白')
    if (needGen.length === 0) {
      success = '所有角色都已分配音色，或无法生成音色描述！'
      return
    }
    batchGeneratingDesign = true
    error = ''
    genProgress = { current: 0, total: needGen.length, currentChapter: '' }
    try {
      for (let i = 0; i < needGen.length; i++) {
        genProgress.current = i + 1
        genProgress.currentChapter = needGen[i].name
        await generateVoiceForChar(needGen[i])
        if (error) break
      }
      if (!error) success = `已为 ${needGen.length} 个角色生成音色！`
    } finally {
      batchGeneratingDesign = false
    }
  }

  // 当用户手动修改角色音色选择时，保存绑定
  function onVoiceChange(c, voiceId) {
    if (voiceId === '__auto_generate__') {
      generateVoiceForChar(c)
      return
    }
    c.voice_id = voiceId
    saveBookRoleVoices()
  }

  async function previewVoice(c) {
    if (!c.voice_id) return
    if (playingVoiceId === c.voice_id && audioPlayer) {
      audioPlayer.pause()
      playingVoiceId = null
      return
    }
    
    try {
      if (audioPlayer) {
        audioPlayer.pause()
      }
      
      const blob = await api.postBlob('/api/v1/synthesis/single', {
        text: '你好，这是我为角色' + c.name + '生成的音色。',
        voice_id: c.voice_id,
        emotion_hint: '平静语气',
        format: 'wav',
      })
      
      const url = URL.createObjectURL(blob)
      audioPlayer = new Audio(url)
      playingVoiceId = c.voice_id
      
      audioPlayer.onended = () => {
        playingVoiceId = null
        URL.revokeObjectURL(url)
      }
      audioPlayer.play()
    } catch(e) {
      error = '试听失败: ' + e.message
      playingVoiceId = null
    }
  }

  function regenerateVoice(c) {
    if (confirm(`确定要为角色 ${c.name} 重新生成音色吗？这会覆盖之前的音色。`)) {
      generateVoiceForChar(c)
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
      <div class="global-chars-header">
        <h3>全局角色配置池 (共 {globalCharacters.length} 角色)</h3>
        <button class="batch-design-btn" disabled={batchGeneratingDesign} onclick={batchGenerateDesignVoices}>
          {#if batchGeneratingDesign}
            ⏳ 生成中 ({genProgress.current}/{genProgress.total}) - {genProgress.currentChapter}
          {:else}
            🎲 一键为未分配角色生成音色
          {/if}
        </button>
      </div>
      <div class="chars">
        {#each globalCharacters as c}
          <div class="char-card" class:narrator-card={c.name === '旁白'} class:generated-card={voices.find(v => v.id === c.voice_id && v.source === 'generated')}>
            <div class="char-header">
              {#if c.name === '旁白'}
                <span class="narrator-badge">📖 旁白</span>
              {/if}
              <strong>{c.name}</strong> 
              {#if c.name !== '旁白'}
                <span class="char-role">({c.role})</span>
              {/if}
              {#if voices.find(v => v.id === c.voice_id && v.source === 'generated')}
                <span class="generated-badge">🔒 已生成</span>
              {/if}
              {#if generatingDesignFor[c.name]}
                <span class="generating-badge">⏳ 生成中...</span>
              {/if}
            </div>
            {#if c.voice_design}
              <div class="voice-design-hint" title={c.voice_design}>🎭 {c.voice_design.slice(0, 40)}{c.voice_design.length > 40 ? '...' : ''}</div>
            {/if}
            <div class="voice-controls">
              <select value={c.voice_id} onchange={(e) => onVoiceChange(c, e.target.value)}>
                <option value="">-- 跟随默认音色 --</option>
                <option value="__auto_generate__">🎲 自动生成音色</option>
                {#each voices as v}
                  <option value={v.id}>{v.source === 'generated' ? '🔒 ' : ''}{v.name}</option>
                {/each}
              </select>
              
              {#if c.voice_id}
                <button class="icon-btn" title={playingVoiceId === c.voice_id ? "停止试听" : "试听音色"} onclick={() => previewVoice(c)}>
                  {playingVoiceId === c.voice_id ? '⏹️' : '▶️'}
                </button>
              {/if}
              
              {#if voices.find(v => v.id === c.voice_id && v.source === 'generated')}
                <button class="icon-btn" title="重新生成" onclick={() => regenerateVoice(c)}>🔄</button>
              {/if}
            </div>
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
  .global-chars-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
    flex-wrap: wrap;
    gap: 8px;
  }
  .global-chars-header h3 { color: var(--accent-start); margin: 0; font-size: 1.1rem; }
  .batch-design-btn {
    padding: 6px 14px;
    background: linear-gradient(135deg, #f093fb, #764ba2);
    color: #fff;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.8rem;
    font-weight: 600;
    transition: opacity 0.2s;
  }
  .batch-design-btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .chars { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
  .char-card {
    padding: 14px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 10px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    transition: all 0.25s ease;
  }
  .char-card:hover {
    transform: translateY(-2px);
    box-shadow: 0 6px 16px rgba(0,0,0,0.12);
    border-color: var(--accent-start);
  }
  .char-card.narrator-card {
    border: 1px solid transparent;
    background: linear-gradient(var(--bg-secondary), var(--bg-secondary)) padding-box,
                linear-gradient(135deg, #f093fb, #f5576c) border-box;
    box-shadow: 0 4px 12px rgba(245, 87, 108, 0.15);
  }
  .char-card.narrator-card:hover {
    box-shadow: 0 8px 20px rgba(245, 87, 108, 0.25);
  }
  .char-card.generated-card {
    border: 1px solid transparent;
    background: linear-gradient(var(--bg-secondary), var(--bg-secondary)) padding-box,
                linear-gradient(135deg, #667eea, #764ba2) border-box;
    box-shadow: 0 4px 12px rgba(118, 75, 162, 0.15);
  }
  .char-card.generated-card:hover {
    box-shadow: 0 8px 20px rgba(118, 75, 162, 0.25);
  }
  .narrator-badge {
    background: linear-gradient(135deg, #f093fb, #f5576c);
    color: #fff;
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 600;
    margin-right: 6px;
  }
  .generated-badge {
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: #fff;
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 0.65rem;
    font-weight: 600;
    margin-left: 6px;
  }
  .generating-badge {
    color: var(--accent-start);
    font-size: 0.7rem;
    margin-left: 6px;
    animation: pulse 1.5s ease-in-out infinite;
  }
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
  .voice-design-hint {
    font-size: 0.75rem;
    color: var(--text-secondary);
    background: rgba(255,255,255,0.04);
    padding: 4px 8px;
    border-radius: 4px;
    line-height: 1.3;
    cursor: help;
  }
  .char-header {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
  }
  .char-header strong { color: var(--text-primary); }
  .char-role { color: var(--text-secondary); font-size: 0.8rem; margin-left: 4px; }
  
  .voice-controls {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .voice-controls select { 
    flex-grow: 1;
    padding: 6px; 
    font-size: 0.85rem; 
  }
  .icon-btn {
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    padding: 6px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.2s;
  }
  .icon-btn:hover {
    background: var(--bg-secondary);
    border-color: var(--accent-start);
  }

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
