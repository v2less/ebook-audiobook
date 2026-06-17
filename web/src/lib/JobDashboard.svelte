<script>
  import { api } from './api.js'

  let jobs = $state([])
  let polling = $state(false)

  async function loadJobs() {
    try {
      jobs = await api.get('/api/v1/jobs')
    } catch(e) { /* ignore */ }
  }

  loadJobs()

  function statusLabel(s) {
    const map = {
      pending: '⏳ 等待中',
      running: '🔄 合成中',
      merging: '🔧 合并中',
      completed: '✅ 已完成',
      failed: '❌ 失败',
      cancelled: '🚫 已取消',
    }
    return map[s] || s
  }

  function statusClass(s) {
    return `status-${s}`
  }

  function progressPercent(job) {
    if (job.progress.total_segments === 0) return 0
    return Math.round(job.progress.completed_segments / job.progress.total_segments * 100)
  }

  async function cancelJob(id) {
    await api.del(`/api/v1/jobs/${id}`)
    loadJobs()
  }

  function downloadJob(job) {
    window.open(`/api/v1/jobs/${job.id}/download`, '_blank')
  }

  // Auto-poll for running jobs
  $effect(() => {
    const hasRunning = jobs.some(j => j.status === 'running' || j.status === 'merging' || j.status === 'pending')
    if (!hasRunning) return

    polling = true
    const interval = setInterval(loadJobs, 3000)
    return () => { clearInterval(interval); polling = false }
  })
</script>

<div class="dashboard">
  <div style="display: flex; justify-content: space-between; align-items: center">
    <h2>任务看板</h2>
    <button class="refresh" onclick={loadJobs}>🔄 刷新</button>
  </div>

  {#if polling}
    <div class="polling-hint">自动刷新中...</div>
  {/if}

  {#if jobs.length === 0}
    <div class="empty">
      <p>暂无合成任务</p>
      <span>上传电子书并配置后，在此查看进度</span>
    </div>
  {/if}

  {#each jobs as job}
    <div class="job-card">
      <div class="job-header">
        <span class={statusClass(job.status)}>{statusLabel(job.status)}</span>
        <span class="job-id">ID: {job.id.slice(0, 8)}...</span>
      </div>

      <div class="job-meta">
        <span>格式: {job.output_format || 'mp3'}</span>
        <span>创建: {new Date(job.created_at).toLocaleString()}</span>
      </div>

      {#if job.status === 'running' || job.status === 'merging'}
        <div class="progress-container">
          <div class="progress-bar">
            <div class="progress-fill" style="width: {progressPercent(job)}%"></div>
          </div>
          <div class="progress-text">
            {job.progress.completed_segments} / {job.progress.total_segments} 片段
            ({job.progress.completed_chapters} / {job.progress.total_chapters} 章)
            — {progressPercent(job)}%
          </div>
        </div>
      {/if}

      {#if job.error}
        <div class="job-error">{job.error}</div>
      {/if}
      {#if job.status === 'failed'}
        <div class="job-hint">💡 请检查 ⚙️ 设置中的 MiMo API Key 是否正确，然后重新创建任务</div>
      {/if}

      <div class="job-actions">
        {#if job.status === 'completed'}
          <button class="download-btn" onclick={() => downloadJob(job)}>💾 下载</button>
        {/if}
        {#if job.status === 'running' || job.status === 'pending' || job.status === 'merging'}
          <button class="cancel-btn" onclick={() => cancelJob(job.id)}>取消</button>
        {/if}
      </div>
    </div>
  {/each}
</div>

<style>
  .refresh {
    padding: 6px 14px;
    background: transparent;
    border: 1px solid var(--border-color);
    color: var(--text-secondary);
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85rem;
  }
  .polling-hint {
    font-size: 0.75rem;
    color: var(--accent-start);
    margin-bottom: 12px;
  }
  .empty {
    text-align: center;
    padding: 48px 24px;
    color: var(--text-secondary);
  }
  .empty p { font-size: 1.1rem; margin-bottom: 6px; }
  .job-card {
    padding: 16px;
    background: var(--bg-card);
    border-radius: 10px;
    border: 1px solid var(--border-color);
    margin-bottom: 12px;
  }
  .job-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }
  .job-id { font-size: 0.75rem; color: var(--text-muted); font-family: monospace; }
  .job-meta {
    display: flex;
    gap: 16px;
    color: var(--text-secondary);
    font-size: 0.8rem;
    margin-bottom: 12px;
  }

  [class^="status-"] {
    font-weight: 600;
    font-size: 0.9rem;
  }
  .status-pending { color: var(--text-secondary); }
  .status-running { color: var(--accent-start); }
  .status-merging { color: var(--accent-end); }
  .status-completed { color: var(--success); }
  .status-failed { color: var(--danger); }
  .status-cancelled { color: var(--warning); }

  .progress-container { margin-bottom: 12px; }
  .progress-bar {
    height: 8px;
    background: var(--bg-secondary);
    border-radius: 4px;
    overflow: hidden;
  }
  .progress-fill {
    height: 100%;
    background: var(--accent-gradient);
    border-radius: 4px;
    transition: width 0.5s;
  }
  .progress-text {
    margin-top: 4px;
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .job-error {
    padding: 8px 12px;
    background: rgba(255,71,87,0.1);
    border-radius: 6px;
    color: var(--danger);
    font-size: 0.85rem;
    margin-bottom: 10px;
  }

  .job-actions {
    display: flex;
    gap: 8px;
  }
  .download-btn, .cancel-btn {
    padding: 8px 18px;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.85rem;
  }
  .download-btn {
    background: var(--accent-gradient);
    color: #fff;
  }
  .cancel-btn {
    background: transparent;
    border: 1px solid var(--warning);
    color: var(--warning);
  }
  .cancel-btn:hover { background: var(--warning); color: #fff; }
</style>
