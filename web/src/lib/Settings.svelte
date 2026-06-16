<script>
  import { api } from './api.js'

  let apiKey = $state('')
  let llmUrl = $state('https://api.openai.com/v1')
  let llmKey = $state('')
  let llmModel = $state('gpt-4o-mini')
  let ttsName = $state('custom-tts')
  let ttsUrl = $state('')
  let ttsKey = $state('')
  let ttsModel = $state('tts-1')
  let ttsVoices = $state('alloy:alloy,echo:echo,fable:fable,nova:nova,shimmer:shimmer')
  let saved = $state(false)
  let isMimoSet = $state(false)
  let isLLMSet = $state(false)
  let importing = $state(false)
  let importResult = $state(null)

  let scriptPrompt = $state('')
  let voicePrompt = $state('')

  const defaultScriptPrompt = "你的任务是将给定小说内容拆分为台词和旁白，输出严格JSON数组。\n输出格式：\n[{type:dialogue,role_name:角色名,text_content:台词,emotion:情绪,intensity:强度,break_duration:0}]\n\n规则：\n- 旁白角色名统一为旁白\n- 情绪可选：开心,生气,伤心,害怕,厌恶,低落,惊喜,平静\n- 强度可选：微弱,稍弱,中等,较强,强烈\n- 必须完整保留原文，不得删改\n- 输出纯JSON数组，不含markdown标记"

  const defaultVoicePrompt = "请简要描述角色${charName}的音色特征。要求：必须要带上性别，对音色的描述文本非常精炼，控制在30字以内。重点描述声音的物理质感（声线粗细、年龄感、沙哑/清脆等），不要包含过多性格或情绪描写。直接输出描述。"

  function loadPrompts() {
    try {
      const saved = localStorage.getItem('prompts')
      if (saved) {
        const p = JSON.parse(saved)
        scriptPrompt = p.script || defaultScriptPrompt
        voicePrompt = p.voice || defaultVoicePrompt
        return
      }
    } catch(e) {}
    scriptPrompt = defaultScriptPrompt
    voicePrompt = defaultVoicePrompt
  }
  loadPrompts()

  function savePrompts() {
    localStorage.setItem('prompts', JSON.stringify({ script: scriptPrompt, voice: voicePrompt }))
    saved = true
    setTimeout(() => saved = false, 2000)
  }

  function resetPrompts() {
    scriptPrompt = defaultScriptPrompt
    voicePrompt = defaultVoicePrompt
    savePrompts()
  }

  async function loadSettings() {
    try {
      const s = await api.get('/api/v1/settings')
      isMimoSet = s.mimo_api_key_set
      isLLMSet = s.llm_api_key_set

      // Pre-fill LLM config from server
      if (s.llm_base_url) llmUrl = s.llm_base_url
      if (s.llm_model) llmModel = s.llm_model
      if (s.llm_api_key_mask) {
        // Show masked key as placeholder to indicate it's already configured
        llmKey = '' // don't reveal the key, but user knows it's set
      }

      // Pre-fill Custom TTS config from server
      if (s.custom_tts && s.custom_tts.length > 0) {
        const ct = s.custom_tts[0]
        ttsName = ct.name || 'custom-tts'
        if (ct.url) ttsUrl = ct.url
        if (ct.model) ttsModel = ct.model
        if (ct.voices) ttsVoices = ct.voices
        if (ct.key_mask) ttsKey = '' // don't reveal key
      }
    } catch(e) {
      console.warn('Failed to load settings:', e)
    }
  }
  loadSettings()

  async function saveKeys() {
    try {
      const payload = {
        api_key: apiKey, llm_url: llmUrl, llm_key: llmKey, llm_model: llmModel,
        tts_name: ttsName, tts_url: ttsUrl, tts_key: ttsKey, tts_model: ttsModel, tts_voices: ttsVoices,
      }
      await api.put('/api/v1/settings', payload)
      saved = true
      if (apiKey) isMimoSet = true
      if (llmKey) isLLMSet = true
      // Reload settings to reflect server state
      await loadSettings()
      setTimeout(() => saved = false, 3000)
    } catch(e) {
      alert('保存失败: ' + e.message)
    }
  }

  async function quickImport() {
    importing = true
    try {
      const res = await fetch('/api/v1/settings?import=1')
      importResult = await res.json()
    } catch(err) {
      importResult = { errors_count: 1, errors: [err.message] }
    } finally { importing = false }
  }
</script>

<div class="settings">
  <h2>⚙️ 设置</h2>

  <div class="card">
    <h3>🎤 MiMo TTS API</h3>
    <p class="desc">语音合成引擎，在 <a href="https://mimo.mi.com" target="_blank">mimo.mi.com</a> 获取</p>
    <input type="password" bind:value={apiKey} placeholder={isMimoSet ? '已配置 (重新输入以覆盖)' : 'MiMo API Key...'} class="field" />
    <div class="status">状态: {isMimoSet ? '🟢 已配置' : '🔴 未配置'}</div>
  </div>

  <div class="card">
    <h3>🧠 LLM 大模型 (OpenAI 兼容)</h3>
    <p class="desc">AI 分析引擎，支持 OpenAI / Gemini / DeepSeek 等兼容接口</p>
    <label>Base URL</label>
    <input bind:value={llmUrl} placeholder="https://api.openai.com/v1" class="field" />
    <label>API Key</label>
    <input type="password" bind:value={llmKey} placeholder={isLLMSet ? '已配置 (重新输入以覆盖)' : 'LLM API Key...'} class="field" />
    <label>Model</label>
    <input bind:value={llmModel} placeholder="gpt-4o-mini" class="field" />
    <div class="status">状态: {isLLMSet ? '🟢 已配置' : '🔴 未配置'}</div>
  </div>

  <div class="card">
    <h3>🔊 自定义 TTS (OpenAI 兼容)</h3>
    <p class="desc">支持任何 OpenAI /audio/speech 兼容的 TTS 服务</p>
    <label>引擎名称</label>
    <input bind:value={ttsName} placeholder="custom-tts" class="field" />
    <label>Base URL</label>
    <input bind:value={ttsUrl} placeholder="https://api.openai.com/v1" class="field" />
    <label>API Key</label>
    <input type="password" bind:value={ttsKey} placeholder="TTS API Key..." class="field" />
    <label>Model</label>
    <input bind:value={ttsModel} placeholder="tts-1" class="field" />
    <label>音色列表 (名称:ID, 逗号分隔)</label>
    <input bind:value={ttsVoices} placeholder="Alloy:alloy,Echo:echo" class="field" />
  </div>

  <div class="card">
    <h3>📦 导入音效资源库</h3>
    <p class="desc">导入 Unitale 工程文件，自动提取 SFX/BGM/音色到本地素材库</p>
    <button class="quick" onclick={quickImport} disabled={importing}>
      {importing ? '导入中...' : '📥 一键导入内置资源库'}
    </button>
    {#if importResult}
      <div class="import-result">
        ✅ SFX {importResult.sfx_count} 个、BGM {importResult.bgm_count} 个、音色 {importResult.voice_count} 个
        {#if importResult.errors_count > 0}<span class="warn">（{importResult.errors_count} 个失败）</span>{/if}
      </div>
    {/if}
  </div>

  <div class="card">
    <h3>📝 Prompt 管理</h3>
    <p class="desc">自定义 AI 分析 Prompt 模板</p>
    <label>脚本分析 Prompt</label>
    <textarea bind:value={scriptPrompt} rows="8" class="prompt-ta"></textarea>
    <label>音色分析 Prompt</label>
    <textarea bind:value={voicePrompt} rows="4" class="prompt-ta"></textarea>
    <div class="prompt-actions">
      <button onclick={savePrompts}>💾 保存 Prompt</button>
      <button class="reset" onclick={resetPrompts}>🔄 恢复默认</button>
    </div>
  </div>

  {#if saved}
    <div class="success">✅ 设置已保存</div>
  {/if}

  <button class="save-btn" onclick={saveKeys}>💾 保存所有设置</button>
</div>

<style>
  .card {
    padding: 20px; background: #252540; border-radius: 12px;
    border: 1px solid #2a2a3a; margin-top: 12px;
  }
  .card h3 { margin-bottom: 6px; }
  .desc { color: #888; font-size: 0.85rem; margin-bottom: 12px; }
  .desc a { color: #667eea; }
  label { display: block; font-size: 0.8rem; color: #aaa; margin: 10px 0 4px; }
  .field {
    width: 100%; padding: 10px 14px; background: #1a1a2e;
    border: 1px solid #3a3a5a; border-radius: 8px; color: #e0e0e0; font-size: 0.9rem;
  }
  .status { color: #aaa; font-size: 0.85rem; margin-top: 8px; }
  .success { color: #4a4; font-size: 0.85rem; margin-top: 12px; }
  .save-btn {
    margin-top: 16px; width: 100%; padding: 14px;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: #fff; border: none; border-radius: 10px; cursor: pointer;
    font-size: 1rem; font-weight: 600;
  }
  .import-result {
    margin-top: 10px; padding: 10px; background: rgba(100,255,100,0.1);
    border-radius: 8px; color: #8f8; font-size: 0.85rem;
  }
  .warn { color: #f90; }
  .quick {
    padding: 10px 24px;
    background: linear-gradient(135deg, #f093fb, #f5576c);
    color: #fff; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem;
  }
  .prompt-ta {
    width: 100%; padding: 10px 14px; background: #1a1a2e;
    border: 1px solid #3a3a5a; border-radius: 8px; color: #e0e0e0;
    font-size: 0.8rem; font-family: monospace; resize: vertical;
  }
  .prompt-actions { display: flex; gap: 8px; margin-top: 10px; }
  .prompt-actions button {
    padding: 6px 14px; background: #667eea; color: #fff;
    border: none; border-radius: 6px; cursor: pointer; font-size: 0.8rem;
  }
  .prompt-actions button.reset { background: transparent; border: 1px solid #666; color: #aaa; }
</style>
