# 🎧 有声书工厂 (Audiobook Factory)

将常见电子书格式（EPUB/PDF/TXT/MOBI/DOCX/Markdown）通过 AI 大模型语音合成引擎转换为有声书，支持自定义音色、AI 智能分析、音效/背景音乐编排。

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.23 + chi (REST + SSE) |
| 数据库 | SQLite |
| 前端 | Svelte 5 + Vite (响应式, 深色/浅色主题) |
| TTS 引擎 | MiMo API v2.5（主）/ OpenAI 兼容 TTS（自定义）/ Real-Time-Voice-Cloning（本地备选）+ 故障转移链 |
| LLM 集成 | OpenAI 兼容 API（流式 + 函数调用 + 上下文窗口管理） |
| 电子书解析 | epub2MD + opendataloader-pdf + pandoc + Go 原生 |
| 音频处理 | FFmpeg（EBU R128 响度归一化 + M4B 章节标记 + 后处理滤镜链） |

## 快速开始

### 前置依赖

- Go 1.23+
- Node.js 20+
- FFmpeg
- [pdf-inspector](https://github.com/firecrawl/pdf-inspector)（推荐，PDF→Markdown，<200ms）
- Python 3（opendataloader-pdf，PDF 备选）
- [epub2md](https://github.com/uxiew/epub2MD)（npm 全局，EPUB 解析）
- [Calibre](https://calibre-ebook.com/)（可选，MOBI 转换）
- [pandoc](https://pandoc.org/)（可选，DOCX 转换）

安装工具：
```bash
# PDF 解析（三选一，按优先级）:
npm install -g @firecrawl/pdf-inspector   # 推荐：极速、表格/布局/连字符修复
pip install opendataloader-pdf            # 备选：结构化 JSON
apt install poppler-utils                 # 最后手段：pdftotext

# EPUB 解析
npm install -g epub2md
```

### 运行

```bash
# 1. 安装依赖
make deps

# 2. 开发模式
make dev          # 启动后端 (hot-reload)
make frontend-dev # 启动前端 (hot-reload)

# 3. 生产构建
make build
make frontend
./bin/audiobook-factory
```

### Docker

```bash
docker compose up -d
```

### 配置

`configs/config.yaml`:
```yaml
mimo:
  api_key: ""           # MiMo API Key (或环境变量 MIMO_API_KEY)
synthesis:
  max_concurrency: 2    # TTS 并发数
  default_format: "mp3" # mp3/wav/m4b/ogg/flac/opus
  chapter_gap: 1.5      # 章节间隔 (秒)
  sample_rate: 24000

library:
  project_path: ""      # Unitale 工程文件路径（SFX/BGM/音色导入）

llm:
  base_url: "https://api.openai.com/v1"
  model: "gpt-4o-mini"
  timeout: 120
```

## API 端点

| Method | Path | 说明 |
|---|---|---|
| `POST` | `/api/v1/books/upload` | 上传电子书 (自动格式检测) |
| `GET` | `/api/v1/books` | 书籍列表 |
| `DELETE` | `/api/v1/books/:id` | 删除书籍 |
| `POST` | `/api/v1/pdf/classify` | PDF 分类检测 (text/scanned/mixed) |
| `GET` | `/api/v1/voices/presets` | 预置音色列表 |
| `POST` | `/api/v1/voices` | 创建自定义音色 (预设/克隆/设计) |
| `POST` | `/api/v1/voices/:id/preview` | 音色试听 |
| `POST` | `/api/v1/voices/:id/evaluate` | 音色质量评估 |
| `POST` | `/api/v1/jobs` | 创建合成任务 |
| `GET` | `/api/v1/jobs/:id/download` | 下载有声书 |
| `GET` | `/api/v1/jobs/:id/progress` | SSE 进度推送 |
| `POST` | `/api/v1/synthesis/single` | 单句 TTS 合成 |
| `POST` | `/api/v1/synthesis/mix` | 多段混音 |
| `POST` | `/api/v1/ai/analyze/:id` | AI 分析章节 (角色/情绪/音效) |
| `POST` | `/api/v1/ai/produce/:id` | AI 全流程生产 |
| `POST` | `/api/v1/ai/llm-proxy` | LLM 代理 (流式兼容) |
| `GET` | `/api/v1/settings` | 获取/恢复运行时设置 |
| `PUT` | `/api/v1/settings` | 保存运行时设置 |
| `POST` | `/api/v1/library/import` | 导入 Unitale 工程文件 |
| `GET` | `/api/v1/books/:id/role-voices` | 获取书籍角色音色映射 |
| `PUT` | `/api/v1/books/:id/role-voices` | 更新书籍角色音色映射 |
| `POST` | `/api/v1/voices/generate-design` | 通过 voice_design 生成临时音色音频 |
| `GET` | `/health` | 健康检查 |

## 支持的格式

| 输入格式 | 解析方式 |
|---|---|
| EPUB | epub2MD CLI（主）/ 纯 Go 降级 |
| PDF | pdf-inspector pdf2md（主 · <200ms · 表格/布局/连字符修复）<br>opendataloader-pdf（备 · 结构化 JSON+Markdown）<br>pdftotext（最后手段） |
| TXT | Go 原生解析（按空行分章） |
| Markdown | Go 原生解析（按 `#` 标题分章） |
| MOBI/AZW/AZW3 | Calibre ebook-convert → EPUB / 纯 Go 降级 |
| DOCX | pandoc（主）/ 纯 Go ZIP+XML 降级 |
| DOC | LibreOffice headless → DOCX → 解析 |
| **输出格式** | MP3 / WAV / M4B / OGG / FLAC / OPUS |

> 支持 **magic bytes 格式自动检测**，不再仅依赖文件扩展名。

## 音色类型

- **预置音色**：冰糖、茉莉、苏打、白桦、Mia、Chloe、Milo、Dean（MiMo v2.5）
- **音色复刻**：上传参考音频 → Base64 → MiMo / OpenAI TTS API
- **音色设计**：文本描述音色特征 → MiMo API Voice Design
- **角色临时音色自动生成**：根据 LLM 设计的音色描述（voice design）自动调用 Voice Design 生成专属参考音频并持久化绑定，后续合成通过音色克隆（Voice Clone）确保音色一致。
- **自定义 TTS**：任意 OpenAI `/audio/speech` 兼容服务
- **音色质量评估**：信噪比、语速、综合评分

## 核心特性

### 🧠 AI 智能分析
- LLM 分析章节内容，提取角色、情绪、音效触发词
- 支持 **流式 (SSE)** 和 **非流式** LLM 调用
- **上下文窗口管理**：自动分块长文本，滑动窗口保留上下文
- **函数调用 (Function Calling)**：内置音效搜索、BGM 搜索、音色查询、场景滤镜建议
- 自动修复 LLM 常见 JSON 输出问题（转义引号、尾部逗号等）
- 兼容各种 LLM Router/Proxy 的 SSE 返回格式

### 🎤 TTS 引擎
- **故障转移链**：MiMo → 自定义 TTS → RTVC 自动切换
- **流式合成**：MiMo SSE 流式 + 自定义引擎 SSE 流式
- **角色音色持久化绑定**：支持在 AI Studio 中根据角色的音色描述自动生成临时音色并持久化绑定。对于历史未分析音色描述的角色，支持通过 LLM 自动补全设计。
- MiMo v2.5 全特性：Audio Tags、Style Prefixes、导演模式

### 🔊 音频生产
- **M4B 有声书格式**：AAC 编码 + 章节标记 + 元数据 + 封面图嵌入
- **EBU R128 响度归一化**：目标 -16 LUFS
- **后处理滤镜链**：去齿音、压缩器、均衡器

### 🎨 前端
- **深色/浅色主题**自动切换（跟随系统或手动）
- **响应式设计**：Mobile (<768px) → 底部 Tab / Tablet → 折叠侧栏 / Desktop
- 设置页面**自动回填**已保存的 API Provider、Model、TTS 配置

### 🛡️ 运维
- **优雅关闭**：SIGINT/SIGTERM → 30s 超时 → 释放资源
- **统一错误格式**：`{"error":{"code":"...","message":"..."}}`
- GitHub Actions CI/CD：lint → test → build → docker

## 项目结构

```
ebook-audiobook/
├── cmd/server/main.go              # 入口（优雅关闭）
├── internal/
│   ├── api/
│   │   ├── server.go               # Server 结构体 & 依赖注入
│   │   ├── router.go               # 路由注册
│   │   ├── response.go             # 统一响应（writeJSON/writeError）
│   │   ├── handler_books.go        # 书籍上传/列表/删除
│   │   ├── handler_voices.go       # 音色 CRUD/预览/评估
│   │   ├── handler_jobs.go         # 合成任务/进度/下载
│   │   ├── handler_synthesis.go    # 单句合成/混音
│   │   ├── handler_ai.go           # AI 分析/生产/LLM 代理
│   │   ├── handler_settings.go     # 运行时设置持久化
│   │   ├── handler_library.go      # Unitale 工程导入
│   │   └── middleware/
│   │       ├── timeout.go          # 请求超时
│   │       ├── ratelimit.go        # 令牌桶限流
│   │       └── auth.go             # Bearer 认证
│   ├── ai/
│   │   ├── analyzer.go             # LLM 章节分析
│   │   └── orchestrator.go         # AI 全流程编排
│   ├── config/config.go            # YAML 配置 (viper)
│   ├── llm/
│   │   ├── client.go               # OpenAI 兼容客户端 + JSON 修复
│   │   ├── stream.go               # SSE 流式聊天
│   │   ├── tools.go                # 函数调用 + 有声书工具集
│   │   └── context.go              # 上下文窗口 + 滑动窗口
│   ├── model/                      # 领域模型
│   ├── parser/
│   │   ├── parser.go               # Parser 接口 + 文本清洗
│   │   ├── registry.go             # 格式路由 + 自动检测
│   │   ├── epub.go                 # EPUB 解析
│   │   ├── pdf.go                  # PDF 解析 (pdf-inspector > odl > pdftotext)
│   │   ├── txt.go                  # TXT/Markdown 解析
│   │   ├── docx.go                 # DOCX/DOC 解析 (★ 新增)
│   │   └── parser_test.go          # 解析器测试
│   ├── storage/store.go            # SQLite 持久化
│   ├── synthesis/
│   │   ├── manager.go              # 合成任务调度
│   │   ├── assembler.go            # FFmpeg 组装 + M4B + 归一化
│   │   ├── retry.go                # 指数退避重试
│   │   └── assembler_test.go       # 组装器测试
│   └── tts/
│       ├── engine.go               # Engine 接口 + 文本分句
│       ├── mimo.go                 # MiMo API v2.5 引擎
│       ├── custom.go               # 自定义 OpenAI TTS + 流式
│       ├── rtvc.go                 # RTVC 本地引擎
│       ├── fallback.go             # 故障转移链 (★ 新增)
│       ├── evaluator.go            # 音色质量评估 (★ 新增)
│       └── engine_test.go          # TTS 测试
├── web/
│   └── src/
│       ├── App.svelte              # Shell（主题切换 + 响应式）
│       └── lib/
│           ├── api.js              # REST 客户端（统一错误提取）
│           ├── Upload.svelte
│           ├── VoiceStudio.svelte
│           ├── SynthesisConfig.svelte
│           ├── JobDashboard.svelte
│           ├── Settings.svelte     # 设置（自动回填）
│           └── ScriptEditor.svelte
├── configs/config.yaml
├── Dockerfile
├── docker-compose.yml
├── .dockerignore                   # (★ 新增)
├── .github/workflows/ci.yml        # CI/CD (★ 新增)
└── Makefile
```

## 注意事项

- **首次使用**：在 Web 设置页配置 MiMo API Key 和 LLM API Key
- **MOBI 格式**：需要安装 Calibre（`apt install calibre` 或 `brew install calibre`）
- **DOCX 格式**：推荐安装 pandoc（`apt install pandoc`），否则使用纯 Go 降级
- **LLM 路由器兼容**：已适配 SSE 流式返回的 Router/Proxy（如 OneAPI/NewAPI）
- **API Key 安全**：所有 Key 持久化到 `data/runtime_settings.json`，GET 接口返回掩码（`sk-****AbcD`）
