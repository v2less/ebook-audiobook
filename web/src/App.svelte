<script>
  import Upload from './lib/Upload.svelte'
  import VoiceStudio from './lib/VoiceStudio.svelte'
  import SynthesisConfig from './lib/SynthesisConfig.svelte'
  import JobDashboard from './lib/JobDashboard.svelte'
  import Settings from './lib/Settings.svelte'
  import ScriptEditor from './lib/ScriptEditor.svelte'

  let tab = $state('upload')
  let selectedBook = $state(null)
  let selectedVoice = $state(null)
  let theme = $state(getInitialTheme())

  function getInitialTheme() {
    if (typeof localStorage !== 'undefined') {
      const saved = localStorage.getItem('theme')
      if (saved) return saved
    }
    if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: light)').matches) {
      return 'light'
    }
    return 'dark'
  }

  function toggleTheme() {
    theme = theme === 'dark' ? 'light' : 'dark'
    localStorage.setItem('theme', theme)
    applyTheme(theme)
  }

  function applyTheme(t) {
    document.documentElement.setAttribute('data-theme', t)
  }

  $effect(() => {
    applyTheme(theme)
  })

  const tabs = [
    { id: 'upload', label: '📚 上传', icon: '📚' },
    { id: 'voices', label: '🎤 音色', icon: '🎤' },
    { id: 'synthesize', label: '⚙️ 合成', icon: '⚙️' },
    { id: 'jobs', label: '📋 任务', icon: '📋' },
    { id: 'settings', label: '🔧 设置', icon: '🔧' },
  ]

  function onBookSelected(book) {
    selectedBook = book
    tab = 'synthesize'
  }

  function onVoiceSelected(voice) {
    selectedVoice = voice
  }
</script>

<div class="app">
  <header>
    <div class="header-row">
      <h1>🎧 有声书工厂</h1>
      <button class="theme-btn" onclick={toggleTheme} title="切换主题">
        {theme === 'dark' ? '☀️' : '🌙'}
      </button>
    </div>
    <p>上传电子书 → 选音色 → 生成有声书</p>
  </header>

  <nav class="desktop-nav">
    {#each tabs as t}
      <button
        class="tab"
        class:active={tab === t.id}
        onclick={() => tab = t.id}
      >
        {t.label}
      </button>
    {/each}
  </nav>

  <main>
    {#if tab === 'upload'}
      <Upload onSelect={onBookSelected} />
    {/if}
    {#if tab === 'voices'}
      <VoiceStudio onSelect={onVoiceSelected} />
    {/if}
    {#if tab === 'synthesize'}
      <SynthesisConfig book={selectedBook} selectedVoice={selectedVoice} />
    {/if}
    {#if tab === 'jobs'}
      <JobDashboard />
    {/if}
    {#if tab === 'settings'}
      <Settings />
    {/if}
  </main>

  <!-- Mobile bottom tab bar -->
  <nav class="mobile-nav">
    {#each tabs as t}
      <button
        class="mobile-tab"
        class:active={tab === t.id}
        onclick={() => tab = t.id}
      >
        <span class="mobile-tab-icon">{t.icon}</span>
        <span class="mobile-tab-label">{t.label.replace(/[^\u4e00-\u9fa5]/g, '')}</span>
      </button>
    {/each}
  </nav>
</div>

<style>
  /* ===== CSS Theme Variables ===== */
  :global(:root) {
    --bg-primary: #0f0f1a;
    --bg-secondary: #1a1a2e;
    --bg-card: #1e1e32;
    --border-color: #2a2a3a;
    --text-primary: #e0e0e0;
    --text-secondary: #888;
    --text-muted: #666;
    --accent-start: #667eea;
    --accent-end: #764ba2;
    --accent-gradient: linear-gradient(135deg, var(--accent-start), var(--accent-end));
    --danger: #ff4757;
    --success: #2ed573;
    --warning: #ffa502;
    --shadow: 0 4px 20px rgba(0,0,0,0.3);
  }

  :global([data-theme="light"]) {
    --bg-primary: #f5f5f7;
    --bg-secondary: #ffffff;
    --bg-card: #fafafa;
    --border-color: #e0e0e0;
    --text-primary: #1a1a2e;
    --text-secondary: #666;
    --text-muted: #999;
    --shadow: 0 4px 20px rgba(0,0,0,0.08);
  }

  :global(*) {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    min-height: 100vh;
    transition: background 0.3s, color 0.3s;
  }

  .app {
    max-width: 900px;
    margin: 0 auto;
    padding: 20px;
    padding-bottom: 80px; /* space for mobile nav */
  }

  header {
    text-align: center;
    padding: 30px 0 20px;
  }

  .header-row {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 16px;
  }

  header h1 {
    font-size: 2rem;
    background: var(--accent-gradient);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
  }

  header p {
    color: var(--text-secondary);
    margin-top: 6px;
    font-size: 0.95rem;
  }

  .theme-btn {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    padding: 6px 10px;
    cursor: pointer;
    font-size: 1.2rem;
    transition: all 0.2s;
  }
  .theme-btn:hover {
    transform: scale(1.1);
  }

  /* Desktop tab navigation */
  .desktop-nav {
    display: flex;
    gap: 8px;
    margin-bottom: 24px;
    flex-wrap: wrap;
    justify-content: center;
  }

  .tab {
    padding: 10px 20px;
    border: 1px solid var(--border-color);
    border-radius: 10px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
    cursor: pointer;
    font-size: 0.9rem;
    transition: all 0.2s;
  }

  .tab:hover {
    border-color: var(--accent-start);
    color: var(--text-primary);
  }

  .tab.active {
    background: var(--accent-gradient);
    color: #fff;
    border-color: transparent;
  }

  main {
    background: var(--bg-secondary);
    border-radius: 16px;
    padding: 24px;
    border: 1px solid var(--border-color);
    box-shadow: var(--shadow);
    transition: background 0.3s;
  }

  /* Mobile bottom tab bar (hidden on desktop) */
  .mobile-nav {
    display: none;
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    background: var(--bg-secondary);
    border-top: 1px solid var(--border-color);
    padding: 8px 0;
    padding-bottom: max(8px, env(safe-area-inset-bottom));
    z-index: 100;
    justify-content: space-around;
  }

  .mobile-tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px 12px;
    font-size: 0.7rem;
    transition: color 0.2s;
  }

  .mobile-tab.active {
    color: var(--accent-start);
  }

  .mobile-tab-icon {
    font-size: 1.3rem;
  }

  /* ===== Responsive Design ===== */
  @media (max-width: 768px) {
    .app {
      padding: 12px;
      padding-bottom: 90px;
    }

    header {
      padding: 16px 0 12px;
    }

    header h1 {
      font-size: 1.5rem;
    }

    main {
      padding: 16px;
      border-radius: 12px;
    }

    .desktop-nav {
      display: none;
    }

    .mobile-nav {
      display: flex;
    }

    .tab {
      padding: 8px 14px;
      font-size: 0.8rem;
    }
  }

  @media (min-width: 769px) and (max-width: 1024px) {
    .app {
      max-width: 95%;
    }

    .desktop-nav {
      justify-content: flex-start;
    }
  }
</style>
