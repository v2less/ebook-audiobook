<script>
  import { api } from './api.js'

  let { isActive = true } = $props()

  let books = $state([])
  let selectedBookId = $state('')
  let editingBook = $state(null)
  
  let selectedChapterIdx = $state(0)
  let saving = $state(false)
  let statusMsg = $state('')

  async function loadBooks() {
    try {
      books = await api.get('/api/v1/books')
    } catch(e) {
      statusMsg = '加载书籍失败'
    }
  }

  let wasActive = $state(false)
  $effect(() => {
    if (isActive && !wasActive) {
      loadBooks()
    }
    wasActive = isActive
  })

  // Load books initially
  loadBooks()

  $effect(() => {
    if (selectedBookId) {
      const book = books.find(b => b.id === selectedBookId)
      if (book) {
        // Deep copy so we can edit without affecting original until saved
        editingBook = JSON.parse(JSON.stringify(book))
        selectedChapterIdx = 0
      }
    } else {
      editingBook = null
    }
  })

  function mergeNext() {
    if (!editingBook || selectedChapterIdx >= editingBook.chapters.length - 1) return
    const current = editingBook.chapters[selectedChapterIdx]
    const next = editingBook.chapters[selectedChapterIdx + 1]
    
    current.content = (current.content + '\n\n' + next.content).trim()
    
    editingBook.chapters.splice(selectedChapterIdx + 1, 1)
    
    // Reindex
    editingBook.chapters.forEach((c, i) => c.index = i)
    editingBook.chapters = [...editingBook.chapters]
    statusMsg = `成功将第 ${selectedChapterIdx+2} 章合并到当前章`
  }

  function splitChapter() {
    if (!editingBook || !editText) return
    
    // Use a naive prompt for user or just split at halfway? Let's split at cursor or middle.
    // Since we don't have textarea cursor selection binding easily in Svelte without bind:this, 
    // let's just split in half for simplicity, or ask user for split point string.
    const splitPointStr = prompt("输入要拆分的界限文字（包含在第二章）：")
    if (!splitPointStr) return

    const current = editingBook.chapters[selectedChapterIdx]
    const currentContent = current.content

    const idx = currentContent.indexOf(splitPointStr)
    if (idx < 0) {
      alert("未找到该文字，无法拆分")
      return
    }

    const firstHalf = currentContent.slice(0, idx).trim()
    const secondHalf = currentContent.slice(idx).trim()

    current.content = firstHalf

    const newChapter = {
      index: selectedChapterIdx + 1,
      title: current.title + ' (续)',
      content: secondHalf,
    }

    editingBook.chapters.splice(selectedChapterIdx + 1, 0, newChapter)
    
    // Reindex
    editingBook.chapters.forEach((c, i) => c.index = i)
    editingBook.chapters = [...editingBook.chapters]
    
    statusMsg = `已拆分出新章节`
  }

  function deleteChapter() {
    if (!editingBook || editingBook.chapters.length <= 1) {
      alert("无法删除最后一章")
      return
    }
    if (!confirm(`确定要删除《${editingBook.chapters[selectedChapterIdx].title}》吗？`)) return

    editingBook.chapters.splice(selectedChapterIdx, 1)
    editingBook.chapters.forEach((c, i) => c.index = i)
    editingBook.chapters = [...editingBook.chapters]
    
    if (selectedChapterIdx >= editingBook.chapters.length) {
      selectedChapterIdx = editingBook.chapters.length - 1
    }
    statusMsg = `章节已删除`
  }

  async function saveChanges() {
    if (!editingBook) return
    saving = true
    statusMsg = '保存中...'
    try {
      const res = await api.put(`/api/v1/books/${editingBook.id}/chapters`, {
        chapters: editingBook.chapters
      })
      statusMsg = '✅ 保存成功'
      
      // Update local books array
      const idx = books.findIndex(b => b.id === editingBook.id)
      if (idx >= 0) {
        books[idx] = res
        books = [...books]
      }
    } catch(err) {
      statusMsg = '❌ 保存失败: ' + err.message
    } finally {
      saving = false
    }
  }

  function resetEdits() {
    const book = books.find(b => b.id === selectedBookId)
    if (book) {
      editingBook = JSON.parse(JSON.stringify(book))
      if (selectedChapterIdx >= editingBook.chapters.length) {
        selectedChapterIdx = 0
      }
      statusMsg = '已重置为最后一次保存的状态'
    }
  }
</script>

<div class="book-editor">
  <div class="header">
    <h2>📖 书籍目录与校对</h2>
    <div class="controls">
      <select bind:value={selectedBookId}>
        <option value="">-- 选择要校对的书籍 --</option>
        {#each books as b}
          <option value={b.id}>{b.title} ({b.chapters?.length || 0} 章)</option>
        {/each}
      </select>

      {#if editingBook}
        <button class="save-btn" disabled={saving} onclick={saveChanges}>
          {saving ? '保存中...' : '💾 保存全部修改'}
        </button>
        <button class="reset-btn" disabled={saving} onclick={resetEdits}>
          ↩️ 重置放弃
        </button>
      {/if}
      {#if statusMsg}
        <span class="status-msg">{statusMsg}</span>
      {/if}
    </div>
  </div>

  {#if editingBook && editingBook.chapters}
    <div class="workspace">
      <!-- Left Sidebar: TOC -->
      <div class="sidebar">
        <h3>目录树 ({editingBook.chapters.length} 章)</h3>
        <div class="toc-list">
          {#each editingBook.chapters as ch, i}
            <div 
              class="toc-item" 
              class:active={selectedChapterIdx === i}
              onclick={() => selectedChapterIdx = i}
            >
              <div class="toc-idx">{i}</div>
              <div class="toc-title" title={ch.title}>{ch.title}</div>
              <div class="toc-count">{ch.content?.length || 0} 字</div>
            </div>
          {/each}
        </div>
      </div>

      <!-- Right Area: Editor -->
      <div class="editor">
        <div class="editor-toolbar">
          <input type="text" class="title-input" bind:value={editingBook.chapters[selectedChapterIdx].title} placeholder="章节标题" />
          <div class="editor-actions">
            <button onclick={mergeNext} disabled={selectedChapterIdx >= editingBook.chapters.length - 1} title="将下一章的文字拼接到本章末尾">🔗 合并下一章</button>
            <button onclick={splitChapter} title="根据指定关键字切分为两章">✂️ 拆分本章</button>
            <button class="danger" onclick={deleteChapter} title="删除这一章的内容">🗑️ 删除此章</button>
          </div>
        </div>
        
        <textarea 
          class="content-input" 
          bind:value={editingBook.chapters[selectedChapterIdx].content} 
          placeholder="正文内容..."
        ></textarea>
        <div class="hint">
          💡 修改此处的文字，然后点击上方“保存全部修改”即可生效。这不会影响原 PDF 文件，只会改变参与合成的文本。
        </div>
      </div>
    </div>
  {:else if selectedBookId}
    <div class="empty-state">书籍加载中或格式不支持...</div>
  {:else}
    <div class="empty-state">请先在上方选择一本已上传的书籍</div>
  {/if}
</div>

<style>
  .book-editor {
    display: flex;
    flex-direction: column;
    height: calc(100vh - 140px); /* Fill available space */
    background: var(--bg-card);
    border-radius: 12px;
    box-shadow: 0 4px 6px rgba(0,0,0,0.05);
    overflow: hidden;
  }

  .header {
    padding: 16px 20px;
    border-bottom: 1px solid var(--border-color);
    display: flex;
    align-items: center;
    gap: 20px;
    background: rgba(0,0,0,0.02);
  }
  .header h2 { margin: 0; font-size: 1.2rem; white-space: nowrap; }
  
  .controls { display: flex; align-items: center; gap: 12px; flex: 1; }
  .controls select {
    padding: 8px 12px; border-radius: 8px;
    border: 1px solid var(--border-color); background: var(--bg-secondary);
    color: var(--text-primary); font-size: 0.9rem; flex: 1; max-width: 300px;
  }
  
  .controls button {
    padding: 8px 16px; border: none; border-radius: 8px; cursor: pointer;
    font-weight: 500; font-size: 0.9rem; transition: background 0.2s;
  }
  .save-btn { background: var(--accent-start); color: #fff; }
  .save-btn:hover:not(:disabled) { background: var(--accent-end); }
  .reset-btn { background: var(--bg-secondary); color: var(--text-secondary); border: 1px solid var(--border-color) !important; }
  .reset-btn:hover:not(:disabled) { background: var(--border-color); color: var(--text-primary); }
  
  .status-msg { font-size: 0.85rem; color: var(--success); }

  .workspace {
    display: flex;
    flex: 1;
    overflow: hidden;
  }

  /* Sidebar */
  .sidebar {
    width: 280px;
    border-right: 1px solid var(--border-color);
    display: flex;
    flex-direction: column;
    background: var(--bg-secondary);
  }
  .sidebar h3 {
    margin: 0; padding: 12px 16px; font-size: 0.95rem; font-weight: 600;
    border-bottom: 1px solid var(--border-color);
  }
  .toc-list {
    flex: 1; overflow-y: auto;
  }
  .toc-item {
    display: flex; align-items: center; gap: 8px;
    padding: 10px 16px; cursor: pointer; border-bottom: 1px solid rgba(0,0,0,0.03);
    transition: background 0.1s;
  }
  .toc-item:hover { background: rgba(0,0,0,0.05); }
  .toc-item.active { background: rgba(102,126,234,0.1); border-left: 3px solid var(--accent-start); }
  
  .toc-idx { font-size: 0.75rem; color: var(--text-muted); min-width: 20px; }
  .toc-title { flex: 1; font-size: 0.85rem; font-weight: 500; color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .toc-count { font-size: 0.7rem; color: var(--text-secondary); background: rgba(0,0,0,0.05); padding: 2px 6px; border-radius: 10px; }

  /* Editor */
  .editor {
    flex: 1; display: flex; flex-direction: column; padding: 16px;
    background: var(--bg-card);
  }
  
  .editor-toolbar {
    display: flex; align-items: center; gap: 16px; margin-bottom: 16px;
  }
  .title-input {
    flex: 1; padding: 10px 14px; font-size: 1.1rem; font-weight: 600;
    border: 1px solid var(--border-color); border-radius: 8px; background: var(--bg-secondary);
    color: var(--text-primary);
  }
  
  .editor-actions { display: flex; gap: 8px; }
  .editor-actions button {
    padding: 6px 12px; border: 1px solid var(--border-color); border-radius: 6px;
    background: var(--bg-secondary); color: var(--text-secondary);
    font-size: 0.85rem; cursor: pointer; transition: all 0.2s;
  }
  .editor-actions button:hover:not(:disabled) { border-color: var(--accent-start); color: var(--accent-start); }
  .editor-actions button:disabled { opacity: 0.5; cursor: not-allowed; }
  .editor-actions .danger:hover:not(:disabled) { border-color: var(--error); color: var(--error); }

  .content-input {
    flex: 1; padding: 16px; font-size: 1rem; line-height: 1.6;
    border: 1px solid var(--border-color); border-radius: 8px;
    background: var(--bg-secondary); color: var(--text-primary);
    resize: none; font-family: inherit;
  }
  .content-input:focus { outline: none; border-color: var(--accent-start); }

  .hint { margin-top: 12px; font-size: 0.8rem; color: var(--text-secondary); text-align: center; }
  
  .empty-state {
    padding: 60px; text-align: center; color: var(--text-secondary); font-size: 1.1rem;
    display: flex; align-items: center; justify-content: center; flex: 1;
  }
</style>
