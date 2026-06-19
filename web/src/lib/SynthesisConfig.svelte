<script>
  import { api } from './api.js'

  let { book, selectedVoice, isActive = true } = $props()

  let books = $state([])
  let voices = $state([])
  let selectedBookId = $state(book?.id || '')
  let selectedVoiceId = $state(selectedVoice?.id || 'mimo_default')
  let outputFormat = $state('mp3')
  let chapterGap = $state(1.5)
  let styleDirective = $state('')
  let submitting = $state(false)
  let error = $state('')
  let success = $state('')

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
      selectedBookId = book.id
    }
    if (selectedVoice?.id) selectedVoiceId = selectedVoice.id
  })

  const selectedBook = $derived(books.find(b => b.id === selectedBookId))

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
</script>

<div class="synthesis-config">
  <h2>全自动批量合成</h2>

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
    {/if}

    <label>
      <span>全局默认音色</span>
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
      <span>全局风格指令（可选，自然语言描述）</span>
      <textarea bind:value={styleDirective}
        rows="3"
        placeholder="例如：温柔舒缓的年轻女声，用播音员风格朗读，中速偏慢，情绪平和如同夜读..."></textarea>
    </label>

    <button class="submit" disabled={submitting} onclick={startSynthesis}>
      {submitting ? '正在创建任务...' : '🚀 开始一键合成全书'}
    </button>
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
