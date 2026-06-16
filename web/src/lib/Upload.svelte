<script>
  import { api } from './api.js'

  let { onSelect } = $props()

  let uploading = $state(false)
  let books = $state([])
  let error = $state('')
  let dragOver = $state(false)

  async function loadBooks() {
    try {
      books = await api.get('/api/v1/books')
    } catch(e) { /* ignore */ }
  }

  loadBooks()

  async function handleUpload(e) {
    const file = e.target?.files?.[0] || (e.dataTransfer?.files?.[0])
    if (!file) return

    uploading = true
    error = ''
    try {
      const book = await api.upload('/api/v1/books/upload', file)
      books = [book, ...books]
      onSelect?.(book)
    } catch(err) {
      error = err.message || 'Upload failed'
    } finally {
      uploading = false
    }
  }

  function handleDragOver(e) {
    e.preventDefault()
    dragOver = true
  }

  function handleDragLeave() {
    dragOver = false
  }

  async function deleteBook(id) {
    await api.del(`/api/v1/books/${id}`)
    books = books.filter(b => b.id !== id)
  }
</script>

<div class="upload">
  <h2>上传电子书</h2>

  <div
    class="dropzone"
    class:dragover={dragOver}
    ondragover={handleDragOver}
    ondragleave={handleDragLeave}
    ondrop={(e) => { e.preventDefault(); dragOver = false; handleUpload(e) }}
  >
    <div class="drop-icon">📖</div>
    <p>拖拽电子书到此处</p>
    <p class="hint">支持 EPUB / PDF / TXT / MOBI / Markdown</p>
    <label class="btn">
      选择文件
      <input type="file" hidden accept=".epub,.pdf,.txt,.mobi,.azw,.azw3,.md,.markdown" onchange={handleUpload} />
    </label>
  </div>

  {#if uploading}
    <div class="loading">正在解析电子书...</div>
  {/if}

  {#if error}
    <div class="error">{error}</div>
  {/if}

  {#if books.length > 0}
    <h3 style="margin-top: 24px">已上传的书籍</h3>
    <div class="book-list">
      {#each books as book}
        <div class="book-card">
          <div class="book-info">
            <strong>{book.title}</strong>
            <span class="meta">{book.author} · {book.format} · {book.chapters?.length || 0} 章</span>
          </div>
          <div class="book-actions">
            <button onclick={() => onSelect?.(book)}>合成</button>
            <button class="danger" onclick={() => deleteBook(book.id)}>删除</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .dropzone {
    border: 2px dashed #3a3a5a;
    border-radius: 16px;
    padding: 48px 24px;
    text-align: center;
    transition: all 0.2s;
    cursor: pointer;
  }
  .dropzone.dragover {
    border-color: #667eea;
    background: rgba(102, 126, 234, 0.1);
  }
  .drop-icon { font-size: 3rem; margin-bottom: 12px; }
  .hint { color: #666; font-size: 0.85rem; margin: 8px 0 16px; }
  .btn {
    display: inline-block;
    padding: 10px 24px;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: #fff;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
  }
  .loading {
    text-align: center;
    padding: 20px;
    color: #667eea;
  }
  .error {
    padding: 12px;
    background: rgba(255,100,100,0.1);
    border: 1px solid #f55;
    border-radius: 8px;
    color: #f88;
    margin-top: 12px;
  }
  .book-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .book-card {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 16px;
    background: #252540;
    border-radius: 10px;
  }
  .book-info strong { display: block; }
  .meta { color: #888; font-size: 0.8rem; }
  .book-actions { display: flex; gap: 8px; }
  .book-actions button {
    padding: 6px 14px;
    border: 1px solid #667eea;
    background: transparent;
    color: #667eea;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.85rem;
  }
  .book-actions button:hover { background: #667eea; color: #fff; }
  .book-actions button.danger { border-color: #f55; color: #f55; }
  .book-actions button.danger:hover { background: #f55; color: #fff; }
</style>
