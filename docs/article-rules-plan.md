# 文章规则功能开发计划

## Context

当前文章条目 `...` 菜单里的「新增规则」只是占位项。目标是把它做成可用的规则系统：

- 规则可绑定一个或多个订阅源。
- 条件只支持参考图中红框列出的字段、操作符和「并且/或者」。
- 命中规则后，对新入库文章执行动作。
- 「设置 → 规则」新增一个标签页，展示全部规则，支持新增、修改、删除、启用/禁用。
- 条目 `...` 菜单能直接打开规则编辑器，并预选当前文章的订阅源。

## 范围与假设

- **条件字段**（红框）：`标题或内容`、`标题`、`内容`、`作者`、`URL`、`URL（无域名）`。
- **操作符**（红框）：`包含`、`不包含`、`是`、`不是`、`开始于`、`结束于`、`匹配正则表达式`、`不匹配正则表达式`。
- **条件组合**（红框）：每行条件保存 `并且/或者`，从第二行开始生效，按从左到右顺序求值。
- **动作**：V1 只实现「标记为已读」。参考图中的「分配标签」依赖目前不存在的标签模型，暂不实现；数据模型预留 `action_type` 以便后续扩展。
- **触发时机**：
  - 订阅刷新时，新条目插入前执行规则；命中则直接以已读状态入库。
  - 编辑器里的「运行规则」对规则覆盖的**已有未读条目**执行一次，保存当前表单后运行。
- 规则启用状态只影响自动触发；手动「运行规则」不受禁用状态限制。
- 规则至少绑定一个订阅源。
- 文本字面量匹配默认不区分大小写；正则按用户输入执行并区分大小写。
- `标题或内容` = 标题 + 空格 + `sanitizer.StripTags(content)` 的纯文本。
- `仅完整单词` 是规则级开关，作用于字面量包含类匹配；CJK 无词边界时退化为子串匹配。
- `URL（无域名）` 取 `path + ?query`，解析失败时退化为去掉 `scheme://host`。

## 数据模型与迁移

新增 `backend/internal/model/article_rule.go`：

```go
type ArticleRule struct {
    ID         int64
    Name       string
    Enabled    bool
    FullWord   bool
    ActionType string // 目前仅 mark_read
    Conditions []RuleCondition
    FeedIDs    []int64
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type RuleCondition struct {
    ID       int64
    RuleID   int64
    Position int
    JoinType string // and / or，第一行忽略
    Field    string // title_content/title/content/author/url/url_no_domain
    Operator string // contains/not_contains/is/is_not/starts_with/ends_with/regex/not_regex
    Value    string
}
```

在 `backend/internal/db/migrations.go` 的顺序迁移末尾新增：

- `article_rules(id, name, enabled, full_word, action_type, created_at, updated_at)`
- `article_rule_conditions(id, rule_id, position, join_type, field, operator, value)`
- `article_rule_feeds(rule_id, feed_id, PRIMARY KEY(rule_id, feed_id))`

约束与索引：

- conditions/feeds 对 `article_rules(id)` 使用 `ON DELETE CASCADE`。
- `article_rule_feeds.feed_id` 对 `feeds(id)` 使用 `ON DELETE CASCADE`，订阅删除后自动清理绑定。
- 索引：`idx_article_rule_conditions_rule(rule_id, position)`、`idx_article_rule_feeds_feed(feed_id)`。
- 主表保存 `action_type` 单列，避免 V1 引入动作 JSON；后续加标签时再增加 `action_params` 或独立动作表。

## 后端实现

### Repository

新增 `backend/internal/repository/article_rule_repository.go`（带 `//go:generate mockgen`）：

- `List(ctx) ([]model.ArticleRule, error)`：查询全部规则，再批量查 conditions/feeds 并在 Go 中组装，避免逐条 N+1。
- `GetByID(ctx, id) (model.ArticleRule, error)`
- `Create(ctx, rule) (model.ArticleRule, error)`
- `Update(ctx, rule) error`
- `Delete(ctx, id) error`
- `SetEnabled(ctx, id, enabled) error`
- `ListEnabledForFeed(ctx, feedID) ([]model.ArticleRule, error)`：给刷新流程使用，只返回启用且绑定该 feed 的规则。

Create/Update 使用 `*sql.DB` 事务，主表与 conditions/feeds 的替换必须原子完成；Update 采用「删除旧 conditions/feeds → 插入新集合」。

### 规则引擎

新增 `backend/internal/service/rule_engine.go`，保持纯函数：

- `MatchArticleRule(rule model.ArticleRule, entry model.Entry) bool`
- `MatchConditions(rule, entry) bool`
- `fieldValue(field string, entry model.Entry) string`
- `matchOperator(operator, value, haystack string, fullWord bool) bool`
- 正则编译/校验辅助函数；同一规则批量匹配时先用 `CompileArticleRule` 预编译正则，避免逐条编译。

求值细节：

- 逐条件从左到右：第一行直接取结果；后续按该行的 `and/or` 与累计结果组合。
- `title_or_content` 使用标题 + 纯文本正文。
- `url_no_domain` 用 `net/url` 解析后取 `EscapedPath()`/`Path` + `RawQuery`。
- 字面量操作符统一转小写比较；正则使用 `regexp.Compile`，保存时先校验。
- 完整单词使用 `(^|[^\p{L}\p{N}_])value([^\p{L}\p{N}_]|$)` 的边界检查；CJK 退化为子串。
- 空条件、空规则值、未知枚举一律不匹配，并在保存时拒绝。

### Service

新增 `backend/internal/service/rule_service.go`：

- `List/Create/Update/Delete/SetEnabled`
- `Run(ctx, ruleID) (RunResult, error)`，返回 `{Matched, Updated int}`
- `ListEnabledForFeed(ctx, feedID) ([]model.ArticleRule, error)`

校验：

- 名称非空、长度上限 100。
- 至少一个 feed，且 feed 存在。
- 1–50 个条件；value 非空；field/operator/join 合法；正则可编译。
- action_type 只接受 `mark_read`。
- 校验失败返回 `service.ErrInvalid`；规则不存在返回 `service.ErrNotFound`。

Run 执行：

- 为规则选择的 feed 查询未读条目，使用 `id > afterID ORDER BY id LIMIT 500` 键集分页，避免一次加载全部。
- 为此在 `EntryRepository` 增加 `ListUnreadForRule(ctx, feedIDs []int64, afterID int64, limit int) ([]model.Entry, error)`，只查 `read = 0`，并补 repository mock 与测试。
- 对每页条目执行 `MatchArticleRule`，收集命中 ID。
- 每 500 个 ID 调用现有 `EntryRepository.UpdateManyReadStatus(ctx, ids, true)`。
- 不包大事务，按批提交；HTTP 层使用 60 秒 context timeout。
- 返回 `matched`（条件命中数）和 `updated`（实际更新数）。

### 刷新流程集成

`RefreshService` 新增窄接口依赖，避免把完整 RuleService 塞进刷新服务：

```go
type RuleProvider interface {
    ListEnabledForFeed(ctx context.Context, feedID int64) ([]model.ArticleRule, error)
}
```

`saveEntries` 中：

- 每次 saveEntries 开始时按 feed 加载一次启用规则，并预编译规则条件。
- 仅对 `exists == false` 的新条目、在 `CreateOrUpdate` 前执行匹配。
- 命中则设置 `entry.Read = true`。
- 更新已有条目时不执行规则，避免覆盖用户已读状态。

同时修正 `entry_repository.go` 的 INSERT：

- 当前 `INSERT INTO entries (..., read, ...) VALUES (..., 0, ...)` 硬编码未读。
- 改为使用 `entry.Read`。
- `ON CONFLICT ... DO UPDATE` 部分保持不更新 `read`。
- 增加 repository 测试，确认传入 `Read=true` 的新条目落库为已读、冲突更新不会覆盖已有已读状态。

### API 与依赖注入

新增 `backend/internal/handler/rule_handler.go`：

- `GET /api/rules`
- `POST /api/rules`
- `PUT /api/rules/:id`
- `DELETE /api/rules/:id`
- `PATCH /api/rules/:id/enabled`
- `POST /api/rules/:id/run`

DTO 中所有 Snowflake ID 使用字符串返回/接收，避免 JS 精度问题。请求体：

```json
{
  "name": "规则名",
  "enabled": true,
  "fullWord": false,
  "feedIds": ["1", "2"],
  "action": { "type": "mark_read" },
  "conditions": [
    { "join": "and", "field": "title", "operator": "contains", "value": "AI" }
  ]
}
```

Run 响应：`{ "matched": 12, "updated": 12 }`。

`cmd/server/main.go` 构造 rule repo/service/handler，并传给 `transport.NewRouter`。`RefreshService` 构造参数增加 `RuleProvider`。同步更新：

- `internal/http/router.go` 与 `router_test.go`
- `internal/handler/routes_test.go`
- `refresh_service_test.go` 中所有 `NewRefreshService` 调用
- `service/mock`、`repository/mock` 生成产物（`make gen`）

## 前端实现

### 类型、API、Hooks

- `frontend/src/types/api.ts`：`ArticleRule`、`RuleCondition`、`RuleField`、`RuleOperator`、`RuleJoin`、`RuleRunResult`。
- `frontend/src/api/index.ts`：`listRules`、`createRule`、`updateRule`、`deleteRule`、`setRuleEnabled`、`runRule`。
- `frontend/src/hooks/useRules.ts`：
  - `useRules()` → `["rules"]`
  - `useCreateRule/useUpdateRule/useDeleteRule/useSetRuleEnabled/useRunRule`
  - Run/保存/删除后失效 `["rules"]`；Run 同时失效 `["entries"]`、`["unreadCounts"]`、`["starredCount"]`。

### 设置页「规则」标签

修改：

- `SettingsModal.tsx`：`SettingsTab` 加 `"rules"`；补 `renderContent`、`getTitle`、`tabs`。
- `SettingsSidebar.tsx`：`navItems` 增加「规则」项和图标。

新增 `frontend/src/components/settings/tabs/RulesSettings.tsx`：

- 顶部「规则」标题 + 「新增规则」按钮。
- 规则列表：名称、订阅源数量、条件摘要、启用 Switch、运行/编辑/删除操作。
- 空状态、加载状态、错误提示。
- 删除使用轻量确认（`Dialog` 或 `window.confirm`，与仓库现有删除习惯保持一致）。
- 运行结果用行内状态显示「已标记 N 条」。

### 规则编辑器

新增 `frontend/src/components/rules/RuleEditorDialog.tsx`，设置页和条目菜单共用：

- 规则名称输入。
- 「当 订阅源中有新文章」+ 多订阅源选择器：
  - 使用 `useFeeds()` 获取订阅源。
  - 用 `DropdownMenu` 承载复选列表，菜单项 `onSelect` 阻止关闭；触发器显示「N 个订阅源」或单个源名称。
- 「如果」条件区：
  - 第一行：字段 select、操作符 select、值 input、删除按钮。
  - 第二行起：并且/或者 select + 上述字段。
  - 「+ 新增条件」。
  - 「仅完整单词」checkbox（规则级）。
- 「然后」动作区：V1 只有一个「标记为已读」选项；保留 select 结构，便于未来扩展。
- 底部：`运行规则`、`保存规则`、`取消`。
- 保存前做客户端校验：名称、feed、条件值、正则。
- 「运行规则」先保存（新建或更新），再调用 run 接口；显示运行中、成功数量和失败信息。

### 条目 `...` 菜单联动

- `EntryListItem.tsx`：`entry.add_rule` 菜单项改为 `onSelect={() => onAddRule?.(entry.feedId)}`，新增 `onAddRule?: (feedId: string) => void` prop。
- `EntryList.tsx`：
  - 新增 `onAddRule` 回调所需状态：`ruleEditorOpen`、`ruleEditorFeedIds`。
  - 渲染 `RuleEditorDialog`，传入 `initialFeedIds`。
  - 将 `onAddRule` 传给 `EntryListItem`。
- 不从设置页路由跳转，也不引入全局 store；编辑器由 EntryList 本地状态控制即可。

### i18n

`frontend/public/locales/zh/common.json` 和 `en/common.json` 增加：

- `settings.rules`：规则
- `rules.*`：标题、名称、订阅源、条件、操作符、并且/或者、仅完整单词、动作、运行、保存、删除、空状态、校验错误、运行结果等
- 复用已有 `entry.add_rule`

## 分阶段实施

1. **迁移与模型**：新增三张表、model、迁移测试，确认 `foreign_keys(ON)` 下删除行为。
2. **规则引擎**：纯函数 + 单测，覆盖全部字段/操作符/组合/正则/完整单词/URL 无域名。
3. **规则仓储**：CRUD、事务、ListEnabledForFeed、mock，repository 测试。
4. **规则服务与 API**：校验、Run、handler、路由、DTO、错误映射，service/handler/routes 测试。
5. **刷新集成**：RuleProvider 注入、saveEntries 命中新条目、INSERT read 修复，更新现有测试。
6. **前端基础**：types、api、hooks。
7. **设置页规则标签**：RulesSettings 列表 + RuleEditorDialog 编辑器。
8. **条目菜单联动**：`... → 新增规则` 预选 feed，补测试与 i18n。
9. **验证与收尾**：Swagger 重新生成、全量测试、lint、build、手工回归。

## 验证方式

- 后端：`go test ./internal/db/... ./internal/repository/... ./internal/service/... ./internal/handler/... ./internal/http/...`。
- 前端：`bun run tsc -b`、`bun run lint`、`bun run test -- --run`、`bun run build`。
- 手工：
  1. 设置页新建规则，绑定两个订阅源，条件用「标题 包含 xxx」。
  2. 保存后在列表看到规则，切换启用/禁用，编辑、删除正常。
  3. 点「运行规则」，确认已有未读文章按条件变为已读，未读数同步下降。
  4. 触发订阅刷新，新文章中命中条件的条目直接以已读状态出现。
  5. 测试「并且/或者」、正则、完整单词、URL 无域名。
  6. 从条目 `... → 新增规则` 打开编辑器，确认当前订阅源已预选。
- 已知既有失败：`frontend/src/app-shell.test.ts` 的换行符断言与本功能无关，验证时单独标注。

## 需要确认的取舍

1. V1 动作只做「标记为已读」，不实现「分配标签」；如需标签，需要另建 tags/entry_tags 模型和列表展示/筛选 UI。
2. 「运行规则」同步处理当前所有未读条目，默认 60 秒超时；数据量很大时后续应改为异步任务。
3. 条件按从左到右顺序求值；第一行的「并且/或者」不生效。
4. 规则不支持「全部订阅源」或按文件夹选择，必须明确勾选一个或多个订阅源。
