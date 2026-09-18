# combined_fields 查询的映射与跨索引执行

> **Workspace:** F:\swe\091801\project-03

---

**User**

检索日志时，用户把 title、summary 和 tags 等多个文本字段放进同一条 combined_fields 请求，希望一组词能按统一的跨字段文本语义命中文档；请把这条查询接入现有链路，使字段集合、字段类型和 analyzer 先经过映射校验，再在兼容字段上完成匹配。operator 与 minimum_should_match 要对所有参与字段产生确定结果，单索引、含多个 shard 的索引和跨索引查询在字段兼容时应返回一致的命中与排序基础；某个索引缺少字段或字段类型不兼容时要按稳定的错误或空结果语义处理，而不是让 reader 合并出不确定结果。格式错误、非法 operator 或 minimum_should_match 只能拒绝当前请求，之后的有效 combined_fields、match、multi_match、bool 和已有 query_string 查询仍按原有入口和响应格式工作。

**Assistant**

我将首先查看相关经验，然后探索代码库以了解现有的查询链路。现在让我探索代码库，了解现有的查询 DSL 结构。

toolName: view_folder

status: success

f:\swe\091801\project-03\pkg\uquery

已有 `combined_fields.go` 文件。让我并行读取关键文件了解现状。

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\combined_fields.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\multi_match.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\query.go

让我继续查看 match 查询、meta 类型定义和 mappings 结构。

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\match.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\mappings\mappings.go

让我查看 meta 中的查询类型定义、Property 结构以及 analyzer 辅助函数。

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\analysis\analyzer.go

让我查看 `CombinedFieldsQuery` 结构体、Mappings 类型以及搜索链路如何跨索引/shard 执行查询。

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\meta\mappings.go

现在让我追踪搜索链路，理解查询如何跨索引/shard 执行，以及映射从哪里来。

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query_dsl_parser.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\search.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\bluge\search\search.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\multi_search.go

现在让我理解 reader 与索引/shard 的关系，以及 `GetReaders` 的结构。

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\index.go

让我查看 riot 库提供的查询类型以及 `CalculateMin` 工具函数。

toolName: search_by_regex

status: success

toolName: Shell

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\zutils\min_should.go

toolName: search_by_regex

status: success

toolName: file_search

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: c:\Users\czxy4\go\pkg\mod\github.com\vcaesar\riot@v1.23.1\query.go

现在让我查看文档索引时 text 字段如何选择 analyzer，以及现有查询测试的风格。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\index_shards_document.go

现在查看现有查询测试，了解测试约定。

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\match_test.go

toolName: search_by_regex

status: success

让我查看错误类型定义和 API 集成测试的结构。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\errors\error.go

toolName: file_search

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\test\api\search_v2_test.go

让我查看搜索 handler 如何分发单索引/跨索引查询，以及 ES mapping 创建接口。

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\handlers\search\search_v2.go

现在我已经理解了整体架构。让我检查 riot 默认分词器（无显式 analyzer 时）与 zinc standard analyzer 的差异，这对分词一致性很关键。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: c:\Users\czxy4\go\pkg\mod\github.com\vcaesar\riot@v1.23.1\analysis\tokenizer\single.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\analysis\analyzer\standard.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

关键链路已清楚。现在创建任务计划并开始实施。

toolName: todo_write

status: success

Todos updated: 6 items

先检查 `meta.CombinedFieldsQuery` 的使用点以及 riot BooleanQuery 的 Validate 逻辑。

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: c:\Users\czxy4\go\pkg\mod\github.com\vcaesar\riot@v1.23.1\query.go

`query_test.go` 第 217 行引用了 `CombinedFieldsQuery`，签名变更会影响它。让我查看这个测试。

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\query_test.go

现在让我快速查看 `query_string.go` 与 `term.go`，确认逐索引校验不会给其他查询类型带来意外的新错误。

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\query_string.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\term.go

现在开始实现。先确认 `zutils.ToString/ToFloat64` 的签名。

toolName: search_by_regex

status: success

现在实现 `combined_fields.go`：

toolName: Write

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\combined_fields.go

changes: undefined

现在接入 query.go 分发链路并更新 meta 结构体：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\query.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\meta\query_dsl.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\meta\query_dsl.go

changes: undefined

现在从 `TestNotImplementedQueries` 中移除 combined_fields：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\query_test.go

changes: undefined

任务1、2 完成。现在改造执行层，支持按索引分组的 mappings/analyzers：

toolName: todo_write

status: success

Todos updated: 6 items

现在重构 `pkg/bluge/search/search.go`：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\bluge\search\search.go

changes: undefined

现在更新 `pkg/core/multi_search.go`，逐索引校验并构建分组：

toolName: Write

status: success

filePath: f:\swe\091801\project-03\pkg\core\multi_search.go

changes: undefined

让我确认 `source.Request` 对重复解析是否幂等（现在验证阶段会按索引多次解析）：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\source\source.go

执行层改造完成。现在编译检查修改过的包：

toolName: Shell

status: success

需要给 meta.CombinedFieldsQuery 增加 Boost 字段：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\meta\query_dsl.go

changes: undefined

toolName: Shell

status: success

编译通过。现在编写单元测试：

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Write

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\combined_fields_test.go

changes: undefined

运行单元测试：

toolName: Shell

status: success

让我查看失败的具体行：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\combined_fields_test.go

问题在于 `zutils.ToString` 会把数字强制转成字符串。按 ES 严格语义，这些字段必须是 string。让我确认 ToString 的行为：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\zutils\strconv.go

`ToString` 永不报错且强转数字。改为严格的 string 类型断言：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\uquery\query\combined_fields.go

changes: undefined

toolName: Shell

status: success

单元测试全部通过。现在编写 API 集成测试。先了解 test/api 的初始化和 index 创建方式：

toolName: todo_write

status: success

Todos updated: 6 items

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\test\api\init.go

让我查看现有创建带 mapping 的索引测试的写法：

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\test\api\analyze_test.go

让我确认创建索引时 mappings 的请求体结构：

toolName: search_by_regex

status: success

现在编写 API 集成测试。先检查多 shard 索引在现有测试中如何构造：

toolName: search_by_regex

status: success

让我查看 core 包测试如何创建索引，以便编写多 shard 测试：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\index_test.go

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\index_test.go

让我查看 core 包中文档索引的公开方法：

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

core 包已有 search_test.go，让我查看其测试模式：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\search_test.go

让我查看 ES 搜索 handler 如何解析 target（逗号分隔多索引）：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\handlers\search\search_v2.go

现在编写 core 包的多 shard 集成测试：

toolName: Write

status: success

filePath: f:\swe\091801\project-03\pkg\core\combined_fields_test.go

changes: undefined

运行 core 多 shard 测试：

toolName: Shell

status: success

`index.GetMappings()` 返回 nil。让我检查 NewIndex 的初始化：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\newindex.go

`NewIndex` 不初始化 Mappings。现有测试先调用了 `StoreIndex`。让我检查 StoreIndex 是否初始化 Mappings：

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\pkg\core\newindex.go

需要先调用 `StoreIndex` 初始化 Mappings。修复测试：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\pkg\core\combined_fields_test.go

changes: undefined

toolName: Shell

status: success

多 shard 测试通过（文档分布在 3 个 shard）。现在编写 HTTP API 集成测试。先确认指定 ID 的文档路由：

toolName: search_by_regex

status: success

现在编写完整的 HTTP API 集成测试：

toolName: Write

status: success

filePath: f:\swe\091801\project-03\test\api\combined_fields_test.go

changes: undefined

运行 API 集成测试：

toolName: Shell

status: success

test/api 依赖根包的 `//go:embed frontend/dist`，需要先构建前端。检查现状：

toolName: view_folder

status: success

f:\swe\091801\project-03\frontend

需要构建前端。执行 `npm ci && npm run build`（可能需要几分钟）：

toolName: Shell

status: success

前端构建完成。重新运行 API 集成测试：

toolName: Shell

status: success

唯一失败是我的测试断言写错了：a1 的 title 含 "quick"、summary 含 "guide"，AND 确实命中。修正断言：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\091801\project-03\test\api\combined_fields_test.go

changes: undefined

现在运行完整测试套件验证回归。先跑 test/api 全部测试：

toolName: todo_write

status: success

Todos updated: 6 items

toolName: Shell

status: success

test/api 全部通过。现在运行 pkg 全部测试：

toolName: Shell

status: success

有几个失败看起来与 WAL 时序有关（"id not found"），可能是预先存在的 flaky 测试。让我先单独重跑这些失败的测试：

toolName: Shell

status: success

toolName: Shell

status: success

需要确认这些失败在我改动前的基线代码上是否也存在。先看 git 状态：

toolName: Shell

status: success

暂存全部改动，在基线代码上运行这两个测试：

toolName: Shell

status: success

确认这两个失败在基线代码上同样存在——是预先存在的问题（Windows 上 WAL 时序）。再验证 TestDeleteByQuery：

toolName: Shell

status: success

TestDeleteByQuery 同样在基线上失败，均为预先存在的问题。auth 包需要环境变量。让我查看 sh/test.sh 设置的环境变量并用相同方式运行：

toolName: view_files

status: success

filePath: f:\swe\091801\project-03\sh\test.sh

让我按 CI 方式设置环境变量并清理 data 目录，运行 auth 相关包验证：

toolName: Shell

status: success

auth 包通过。现在用完整 CI 环境运行全部测试：

toolName: Shell

status: success

全部测试通过。现在运行 `go vet` 并清理测试数据目录：

toolName: Shell

status: success

尝试运行 golangci-lint（CI 使用）：

toolName: Shell

status: success

golangci-lint 未安装，但 go vet 已通过。最后检查编辑器诊断：

toolName: GetDiagnostics

status: success

toolName: todo_write

status: success

Todos updated: 6 items

`combined_fields` 查询已完整接入现有链路。全部实现与验证如下：

## 实现内容

### 1. 查询构建与映射校验
[combined_fields.go](file:///f:/swe/091801/project-03/pkg/uquery/query/combined_fields.go) 中的 `CombinedFieldsQuery` 现在按以下顺序工作：

1. **参数解析**：`query`、`fields`（支持 `field^boost` 记法）、`analyzer`、`operator`、`minimum_should_match`、`boost`、`zero_terms_query`；已知字段类型错误（如 query 非字符串、fields 非数组）直接返回 x_content_parse 错误。
2. **映射校验**（针对所属索引）：
   - 已映射字段必须是 `text`，否则报 `only text fields are supported`；
   - 未映射字段按 ES 语义忽略（对任何类型兼容）；
   - 不显式指定 analyzer 时，所有参与字段的 search analyzer（`search_analyzer` → `analyzer` → 默认）必须一致，否则稳定报错；
   - 该索引一个字段都没有时返回 `match_none`，而不是让 term 查询在 reader 上产生不确定行为。
3. **确定性匹配**：查询文本用统一 analyzer 只分词一次（无 analyzer 时用 standard，与索引侧默认一致），每个 term 构造「在任意参与字段命中」的子句；
   - `AND`：每个 term 必须出现在至少一个字段中（如 fox 在 title、nature 在 tags 也能命中，这是与 per-field 方式的核心区别）；
   - `OR`/默认：外层 should，`minimum_should_match` 经 `CalculateMin` 解析（数字、百分比、负数、组合表达式），对所有参与字段产生确定阈值。

### 2. 链路接入
- [query.go](file:///f:/swe/091801/project-03/pkg/uquery/query/query.go#L101-L104) 分发改为传入 mappings/analyzers；
- [query_dsl.go](file:///f:/swe/091801/project-03/pkg/meta/query_dsl.go#L163-L170) 的结构体补充 `Boost` 字段、`MinimumShouldMatch` 改为 `interface{}` 以支持百分比等写法。

### 3. 跨索引确定性
- [search.go](file:///f:/swe/091801/project-03/pkg/bluge/search/search.go#L34-L186) 新增 `ReaderGroup` 与 `MultiSearchGroups`：每个索引的 shard reader 绑定该索引自己的 mappings/analyzers；原 `MultiSearch` 签名不变、内部委托到分组实现，单索引入口完全保持原样。
- [multi_search.go](file:///f:/swe/091801/project-03/pkg/core/multi_search.go#L33-L131) 先逐索引校验（类型/analyzer 不兼容立即拒绝整个请求），再按索引分组执行：兼容字段上命中与排序基础一致；缺字段的索引在剩余字段上匹配、全部缺失时贡献空结果，不会再用首个索引的映射合并出不确定结果。

### 4. 测试
- 单元测试 [combined_fields_test.go](file:///f:/swe/091801/project-03/pkg/uquery/query/combined_fields_test.go) 覆盖全部合法/非法分支；
- 多 shard 测试 [pkg/core/combined_fields_test.go](file:///f:/swe/091801/project-03/pkg/core/combined_fields_test.go)（3 shard，验证跨字段 AND、msm、重复查询排序一致）；
- HTTP 集成测试 [test/api/combined_fields_test.go](file:///f:/swe/091801/project-03/test/api/combined_fields_test.go) 覆盖单索引、跨索引兼容、缺字段、类型/analyzer 不兼容、9 种畸形请求拒绝，并回归验证之后的 combined_fields、match、multi_match、bool、query_string 入口与响应格式均不变。

**验证结果**：`go test ./...`（CI 环境变量）全部通过，`go vet` 无问题。过程中发现的 3 个 "id not found"/delete-by_query 失败经 stash 对比确认是基线代码在 Windows 上的既有 WAL 时序问题，与本次改动无关（CI 的 `ZINC_WAL_SYNC_INTERVAL=10ms` 下同样通过）。

本次工作中同类「新查询类型接入」流程（解析→映射校验→分组执行→分层测试）已第三次出现，可沉淀为该工作区专属 skill，自动套用 ZincSearch 的查询链路约定；如需要，我可以帮你创建。
