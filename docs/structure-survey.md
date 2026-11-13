# Gist 仓库结构速览

> 自托管 RSS 阅读器：Go + Echo + SQLite 后端，React + Vite + TypeScript 前端，单二进制/Docker 交付。

## 1. 顶层布局

```
Gist/
├── backend/            Go 服务端（cmd + internal + pkg）
├── frontend/           React SPA（Vite + wouter + TanStack Query）
├── docker/             Dockerfile、entrypoint.sh
├── docs/               截图等文档资源
├── .github/workflows/  后端/前端测试、Docker 构建、Release
├── docker-compose.yml  部署入口
├── flake.nix/.envrc    Nix 开发环境
└── .rules              项目规范（CLAUDE.md/AGENTS.md/GEMINI.md 均指向它）
```

## 2. 后端（backend）

### 2.1 目录与分层

| 目录 | 职责 |
| --- | --- |
| `cmd/server/main.go` | 唯一入口：加载配置、初始化日志/Snowflake/DB、手工组装依赖、启动 HTTP 与定时任务、优雅关闭 |
| `internal/config` | 环境变量配置（`GIST_ADDR`、`GIST_DATA_DIR`、`GIST_STATIC_DIR`、`GIST_LOG_LEVEL`、`GIST_SWAGGER`、`GIST_PPROF_ADDR`）及 UA 常量 |
| `internal/http` | Echo 路由装配、JWT/请求日志中间件、SPA 静态文件与回退 |
| `internal/handler` | HTTP 适配层：参数校验、请求/响应 DTO、swag 注解、错误到状态码映射 |
| `internal/service` | 业务编排：Feed/Entry/Folder/OPML/Refresh/Readability/Icon/Proxy/AI/Auth/Settings/限流/导入任务 |
| `internal/service/ai` | AI Provider 抽象与实现（OpenAI/Anthropic/兼容接口）、提示词、HTML 分块、限流器 |
| `internal/service/anubis` | Anubis 反爬挑战的 Cookie 存储与求解 |
| `internal/repository` | SQLite 数据访问；接口与实现同文件，`mock/` 为 mockgen 产物，`testutil/` 为测试 DB |
| `internal/model` | 领域模型（Folder/Feed/Entry/Settings/AI 缓存/域名限流） |
| `internal/db` | 连接与 DSN pragma（WAL、外键、busy_timeout）、内嵌式增量迁移 |
| `internal/scheduler` | 15 分钟定时刷新，启动即执行一次，支持取消与优雅停止 |
| `internal/hashutil` / `urlutil` | 文章去重哈希、URL 规范化/主机提取 |
| `pkg/logger` | 基于 slog 的全局日志封装 |
| `pkg/network` | `ClientFactory`：标准 HTTP 与 azuretls 客户端，统一代理/IP 栈 |
| `pkg/opml` | OPML XML 解析/编码 |
| `pkg/sanitizer` | Author/HTML 文本清洗 |
| `pkg/snowflake` | Snowflake ID 生成（全局 node） |
| `docs/` | `swag init` 生成的 Swagger 文档 |

### 2.2 启动与请求链路

```
main()
 ├─ config.Load() → logger.Init() → snowflake.Init(1)
 ├─ db.Open() ──► db.Migrate()（基础表 + 增量 ALTER/建表）
 ├─ 构造 8 个 Repository
 ├─ 构造 SettingsService → network.ClientFactory → anubis.Solver
 ├─ 构造 12 个业务 Service + ImportTaskService
 │    └─ 后台 goroutine：IconService.BackfillIcons()
 ├─ 构造 10 个 Handler
 ├─ transport.NewRouter(...)（13 个参数）
 ├─ scheduler.New(refreshService, 15m).Start()
 └─ signal.Notify(SIGINT/SIGTERM) → 取消后台任务、关 DB 连接、Shutdown HTTP

一次请求：
Echo
 ├─ Recover → RequestLogger
 ├─ /api/auth/{status,register,login} 公开
 ├─ /api/* → JWTAuthMiddleware → handler.Xxx
 │      handler 绑定/校验 → service.Xxx → repository.Xxx → SQLite
 │      错误经 handler/response.go 统一映射
 ├─ /api/icons/* 与 /icons/:filename → IconHandler
 └─ /* → 静态文件；未命中且非 assets 时回退 index.html（SPA）
```

关键点：

- 路由不是集中声明，而是各 Handler 的 `RegisterRoutes`/`RegisterPublicRoutes`/`RegisterAPIRoutes` 注册，`internal/http/router.go` 只负责分组与中间件。
- 鉴权同时接受 `Authorization: Bearer` 与 `gist_auth` Cookie（便于 `<img>` 等资源请求）。
- 数据库为 `modernc.org/sqlite`（无 CGO），表：`folders`、`feeds`、`entries`、`entries_fts`（FTS5 + 触发器）、`settings`、`ai_summaries`、`ai_translations`、`ai_list_translations`、`domain_rate_limits`。
- 刷新流程有全局并发上限 8、同主机串行、域名级最小间隔；Feed 支持 ETag/Last-Modified 条件请求。

## 3. 前端（frontend）

### 3.1 目录组织

| 目录 | 职责 |
| --- | --- |
| `src/main.tsx` | React 入口：QueryClientProvider、I18nProvider、启动埋点、SW 状态、BFCache 失效 |
| `src/App.tsx` | 路由容器 + 鉴权门禁 + 移动/桌面布局分支（约 560 行） |
| `src/lib/router.ts` | URL ↔ 路由状态解析/构建（`/all`、`/feed/:id`、`/folder/:id`、`/starred`、`/add-feed`，可选 entryId、`unread`、`type`） |
| `src/components/` | 按功能分目录：`sidebar`、`entry-list`、`entry-content`、`picture-masonry`、`settings`、`add-feed`、`auth`、`layout`，`ui/` 为通用基础组件 |
| `src/hooks/` | 数据查询/变更（`useEntries`、`useFeeds`、`useFolders`）、AI/Readability、滚动/手势/移动端/主题等交互逻辑 |
| `src/api/index.ts` | 统一 API 客户端（约 984 行）：fetch 封装、Token、错误、NDJSON/SSE 流解析、全部领域接口 |
| `src/stores/` | Zustand 客户端状态：auth、image-preview、lightbox、image-dimensions、translation |
| `src/types/` | `api.ts`（Feed/Entry/Folder 等）与 `settings.ts`（AI/网络/外观等） |
| `src/services/translation-service.ts` | 翻译请求去重与批量任务协调 |
| `src/i18n/` + `public/locales/{zh,en}/common.json` | i18next 初始化与中英文资源 |

### 3.2 入口、路由与数据链路

```
index.html
 └─ main.tsx
     ├─ QueryClientProvider (staleTime 30s, retry 1)
     ├─ I18nProvider
     └─ App.tsx / wouter Router
          └─ AppContent（auth 状态机：loading / network-error / 注册 / 登录 / 已登录）
               └─ AuthenticatedApp
                    ├─ useSelection ──► lib/router.ts（URL 即选择状态）
                    ├─ useFeeds / useFolders / useEntriesInfinite / useEntry
                    │      └─► @/api ── fetch ──► /api/*
                    ├─ 移动端：Sheet 侧栏 + 列表/详情叠层；桌面：三栏布局
                    ├─ EntryList / PictureMasonry（滚动已读、无限分页）
                    └─ EntryContent
                         ├─ useReadability  → POST /entries/:id/fetch-readable
                         ├─ useAISummary    → POST /ai/summarize（流式）
                         └─ useAITranslation→ POST /ai/translate(/batch)（流式）
```

状态分工：

- **服务端状态**：TanStack Query。查询键以 `entries`、`entry`、`feeds`、`folders`、`unreadCounts`、`refreshStatus` 及各 settings 为主；已读操作为乐观更新 + 回滚；`useRefreshStatus` 每 15s 轮询并在刷新完成时失效相关缓存。
- **客户端状态**：Zustand（auth、图片预览、灯箱、翻译、图片尺寸）；UI 偏好另由 `useUISettings` 写入本地存储；页面选择状态放在 URL。
- **分页**：`EntryList` 请求 `limit`，后端用多取一条或 `hasMore` 返回，前端 `useInfiniteQuery` 以已加载数量作为 offset。
- **流式**：AI 摘要/翻译走 NDJSON 增量行；OPML 导入状态有轮询/SSE 封装（`watchImportStatus`）。

## 4. 构建、运行、测试与生成

| 项目 | 位置/命令 |
| --- | --- |
| 后端运行 | `cd backend && go run ./cmd/server/main.go` |
| 前端运行 | `cd frontend && bun install && bun run dev`（Vite 将 `/api` 代理到 `localhost:8080`） |
| 后端构建 | `cd backend && make build` → `gist-server` |
| 后端测试 | `make test`（`go test ./... -race -v`） |
| 后端 lint | `make lint`（`golangci-lint`，配置 `backend/.golangci.yml`） |
| Mock 生成 | `make gen` → `go generate ./...`，`//go:generate mockgen ... -destination=mock/$GOFILE` |
| Swagger | `swag init -g cmd/server/main.go --parseDependency --parseInternal`，产物在 `backend/docs/` |
| 前端构建 | `bun run build`（`tsc -b && vite build`，含 PWA/workbox 与手动分包） |
| 前端测试 | `bun run test`（Vitest + jsdom，`frontend/vitest.config.ts`） |
| 前端 lint | `bun run lint`（ESLint flat config） |
| 类型检查 | `bun run tsc -b` |
| CI | `backend-test.yml`、`frontend-test.yml`、`docker-build.yml`、`release.yml` |
| 镜像 | `docker/Dockerfile` 三阶段：bun 构建前端 → Go 静态编译 → Alpine 运行；`docker-compose.yml` 挂载 `./data` |

## 5. 已观察到的耦合、重复与可疑点

1. **组合根集中且参数多**：`cmd/server/main.go` 手工组装 8 个 Repository、12 个 Service、10 个 Handler；`NewRouter` 接收 13 个参数。新增模块需要同时改 `main.go` 与 `router.go`。
2. **pkg 反向依赖规避**：`pkg/network` 自定义 `ProxyProvider`/`IPStackProvider` 接口以避免与 `service` 包循环引用；`main.go` 把同一个 `SettingsService` 同时作为两个 provider 传入。
3. **路由表分散**：路径定义分布在 10 个 Handler 文件中，`internal/http/router.go` 看不到完整路由清单；`/icons` 同时存在全局路由与 `/api/icons` 两套入口。
4. **已读接口重叠**：`PATCH /entries/read`（批量）、`PATCH /entries/:id/read`（单条）、`POST /entries/mark-read`（按 feed/folder/type 全部已读）三套语义并存，调用方需要自行选择。
5. **迁移脚本为运行时命令式**：`internal/db/migrations.go` 约 645 行，通过反复查询表结构后执行 `ALTER TABLE`/`CREATE TABLE`，没有 schema 版本表；表结构知识分散在基础 schema 与多段增量逻辑中。
6. **DTO 与 model 双份维护**：Handler 内各自定义 `entryResponse`、`feedResponse` 等并与 `model` 手工映射；字段变更需要同时改 model、handler DTO、前端 `types`。
7. **App.tsx 承担过多职责**：鉴权门禁、路由重定向、响应式布局、移动端列表/详情切换、查询组合都在同一文件；移动端与桌面端各自重复传入 `EntryList`/`PictureMasonry` 的整组 props。
8. **API 层单文件过大且调用方式不统一**：`src/api/index.ts` 同时包含传输层、Token 存储、错误码、流解析和全部领域函数；`settings/tabs/*` 直接 import API，而列表/Feed 等又通过 hooks 封装，存在两种数据访问风格。
9. **设置 hooks 重复模板**：`useAISettings`、`useGeneralSettings`、`useAppearanceSettings` 都是同构的 3 行 `useQuery` 包装，仅 queryKey/请求函数不同。
10. **模块级可变请求状态与规范冲突**：`src/services/translation-service.ts` 使用模块级 `inFlightRequests`、`inFlightBatchArticles`、`batchAbortController` 保存请求上下文，与 `.rules` 中“禁止使用模块级变量传递请求上下文”不一致。
11. **依赖/配置残留**：`frontend/package.json` 同时依赖 `motion` 与 `framer-motion`；`vite.config.ts` 的 `manualChunks` 判断了未在依赖中声明的 `@tanstack/react-virtual`。
12. **Makefile clean 与 Mock 产物不匹配**：`make clean` 删除的是 `mock_*.go`，而 mockgen 产物是 `mock/<source>.go`，该清理规则不会命中现有 Mock。
13. **测试规范存在例外**：`.rules` 要求测试使用 `package name_test`，但 `pkg/opml/opml_test.go`、`pkg/network/client_test.go`、`pkg/logger/logger_test.go` 等为同包测试；`internal/*/export_test.go` 也是同包导出钩子。

## 6. 快速索引

- 后端入口：[`backend/cmd/server/main.go`](../../../backend/cmd/server/main.go)
- 路由与中间件：[`backend/internal/http/router.go`](../../../backend/internal/http/router.go)、[`middleware.go`](../../../backend/internal/http/middleware.go)
- 领域模型：[`backend/internal/model/`](../../../backend/internal/model)
- 前端入口：[`frontend/src/main.tsx`](../../../frontend/src/main.tsx)、[`App.tsx`](../../../frontend/src/App.tsx)
- 前端路由解析：[`frontend/src/lib/router.ts`](../../../frontend/src/lib/router.ts)
- API 客户端：[`frontend/src/api/index.ts`](../../../frontend/src/api/index.ts)
- 类型定义：[`frontend/src/types/api.ts`](../../../frontend/src/types/api.ts)、[`settings.ts`](../../../frontend/src/types/settings.ts)
