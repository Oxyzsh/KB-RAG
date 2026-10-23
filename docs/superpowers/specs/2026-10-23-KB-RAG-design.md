# KB-RAG 企业知识库 RAG 设计文档

- 日期：2026-10-23
- 状态：已评审，待实施
- 技术栈：Go 1.26.5
- 第一期形态：命令行工具（CLI）

## 1. 背景与目标

企业内部知识分散在 Markdown、Word、PDF 等文档中，需要一套可本地部署、可溯源的知识库问答系统。第一期先做 CLI，用真实问题验证检索与生成效果，再决定是否扩展为服务或界面。

核心目标：

1. 文档解析 → 切分 → 本地嵌入 → 检索 → 云端生成，全链路可运行。
2. 答案必须附带引用的原文片段与来源文件，方便人工核对。
3. 数据本地存储；嵌入模型本地运行；生成调用云端 OpenAI 兼容 API。
4. 用 20-30 个真实问题做人工评测，达到验收标准。

## 2. 第一期范围

### 2.1 做

- CLI 命令：`ingest`、`ask`、`stats`
- 文档格式：Markdown、TXT、Word（.docx）、文本型 PDF
- 本地嵌入：Windows 上通过 Ollama 运行 `bge-m3`（1024 维，CPU 推理）
- 向量存储：SQLite（纯 Go 驱动），内存暴力余弦检索
- 生成：任意 OpenAI 兼容 `/chat/completions` 服务
- 答案输出：答案正文 + `[n]` 引用标注 + 来源列表
- 增量入库：按文件 SHA-256 跳过未变文件
- 日志落盘与轮转
- 单元测试、基于假模型服务的集成测试、人工评测

### 2.2 不做（第一期明确排除）

- 扫描件 PDF 的 OCR
- Web 界面、登录、权限、多用户、多租户
- IM 机器人（企业微信/钉钉/Slack）
- 文件系统监听与自动同步
- 删除源文件后的自动清理（`prune`）
- 重排模型（rerank）、关键词/向量混合检索、问题改写（query rewrite）
- 多轮对话与流式输出
- 分布式向量库或 Docker 化部署
- 自动化评测框架

这些能力在后续版本按需引入，第一期不为它们预留配置或抽象。

## 3. 需求汇总

| 维度 | 结论 |
|------|------|
| 场景 | 企业/团队知识库问答，中文为主 |
| 使用者 | 第一期：开发者本机 CLI |
| 文档来源 | 多种来源混合，第一期统一为本地目录 |
| 文档规模 | 不确定；按小规模设计，存储层保留替换空间 |
| 嵌入 | 本地 Ollama + bge-m3，CPU（无 NVIDIA GPU，16GB 内存） |
| 生成 | 云端 OpenAI 兼容 API，Key 走环境变量 |
| 输出 | 答案 + 引用原文 |
| 验收 | 人工评测真实问题集 |

## 4. 方案选择

### 4.1 候选方案

- **方案 A：手写轻量管道**（采用）
  - CLI + 本地 Ollama 嵌入 + SQLite 向量存储 + 云端 LLM。
  - 依赖少、链路透明，中文切分与引用格式完全可控。
  - 10 万片段以内，内存暴力检索每次查询约几十毫秒，满足第一期。
- **方案 B：Qdrant 向量库**
  - 适合万级以上、需要增量过滤的场景，但需要 Docker/常驻服务，第一期用不到，提前引入复杂度。
- **方案 C：langchaingo 框架**
  - 减少胶水代码，但抽象重、版本迭代快；中文切分、docx/PDF 解析、引用格式仍需自写，排查链路长。

### 4.2 采用 A 的理由

第一期目标是验证效果而非上生产。管道代码量约 1000-1500 行，透明可控；存储层保留接口，后续换 Qdrant 只替换实现。

## 5. 总体架构

```text
cmd/KB-RAG/              CLI 入口：ingest / ask / stats
internal/config/          YAML 配置 + 环境变量覆盖
internal/ingest/
  loader/                 md、txt、docx、pdf → Document{标题, 正文, 结构}
  chunker/                结构感知切分
  embedder/               Ollama /api/embed 客户端
internal/store/           SQLite 持久化 + 余弦检索
internal/query/           检索 → 拼上下文 → 生成 → 引用
internal/llm/             OpenAI 兼容 chat/completions 客户端
docs/                     设计与评测文档
data/                     运行时生成：SQLite 库与日志（不提交）
```

### 5.1 核心接口

| 接口 | 职责 | 替换点 |
|------|------|--------|
| `Loader.Load(path) (Document, error)` | 单文件解析为纯文本 + 标题结构 | 新增格式只加实现 |
| `Chunker.Split(doc) []Chunk` | 切分并保留标题路径 | 参数可调 |
| `Embedder.Embed(ctx, []string) ([][]float32, error)` | 文本转向量 | 本地 Ollama，可换云嵌入 |
| `Store`：`UpsertDocument` / `DeleteDocument` / `Search` / `Stats` | 持久化与检索 | SQLite，可换 Qdrant |
| `LLM.Chat(ctx, messages) (string, error)` | 生成答案 | 任意 OpenAI 兼容服务 |

### 5.2 依赖

- `modernc.org/sqlite`：纯 Go SQLite 驱动，无 CGO
- `github.com/yuin/goldmark`：Markdown 解析
- `gopkg.in/yaml.v3`：配置解析
- `gopkg.in/natefinch/lumberjack.v2`：日志轮转
- PDF 解析库：候选为 `github.com/ledongthuc/pdf` 或 `github.com/dslipak/pdf`。**实现第一步用真实 PDF 验证中文提取效果后定型**；若乱码严重，增加调用外部 `pdftotext` 的 loader 实现。

DOCX 使用标准库 `archive/zip` + `encoding/xml` 解析，不引入第三方库。

## 6. CLI 设计

```bash
KB-RAG ingest ./docs      # 扫描目录，增量入库
KB-RAG ask "年假怎么请"    # 检索 + 生成答案 + 引用
KB-RAG stats              # 文档/片段数量、失败文档、嵌入模型、库大小
```

- 子命令用标准库 `flag.NewFlagSet` 实现，不引入 CLI 框架。
- `ask` 支持 `--top-k`（默认 8）和 `--config`；`ingest` 支持 `--config`。
- 路径显示时去掉当前工作目录前缀，输出为相对路径。

## 7. 配置

默认读取 `./config.yaml`，可用 `--config` 指定。

```yaml
embedding:
  base_url: http://localhost:11434
  model: bge-m3
  batch_size: 16

llm:
  base_url: https://api.deepseek.com/v1
  model: deepseek-chat

chunk:
  size: 500        # 字符数
  overlap: 80

store:
  path: ./data/KB-RAG.db

log:
  file: ./data/logs/KB-RAG.log
```

- API Key 只从环境变量 `KB_RAG_LLM_API_KEY` 读取，不写入配置、不打印。
- 校验按命令区分：`ingest`/`stats` 只校验 embedding、chunk、store、log 配置；`ask` 额外校验 llm 配置与 API Key。
- 校验内容：必填项非空、URL 格式、`chunk.size > chunk.overlap > 0`、库目录可创建。校验失败退出码 2。
- 超时为固定值：嵌入单批 120 秒，LLM 请求 90 秒。

## 8. 数据模型与存储

### 8.1 表结构

```sql
documents(
  id INTEGER PRIMARY KEY,
  path TEXT UNIQUE,          -- 绝对路径
  title TEXT,
  content_hash TEXT,         -- SHA-256，用于增量判断
  status TEXT,               -- ok / failed
  error TEXT,
  updated_at TEXT
)

chunks(
  id INTEGER PRIMARY KEY,
  doc_id INTEGER REFERENCES documents(id),
  seq INTEGER,               -- 文档内顺序
  heading TEXT,              -- 标题路径，如 "人事制度 > 休假 > 年假"
  text TEXT,
  vector BLOB,               -- float32 小端，写入时已归一化
  dim INTEGER
)

meta(key TEXT PRIMARY KEY, value TEXT)
-- schema_version, embedding_model, dim, chunk_size
```

### 8.2 存储决定

1. 向量归一化后以 float32 小端 BLOB 存储，检索用点积。
2. `ask` 时将全部向量读入内存计算；CLI 进程短命，10 万片段约 400MB 向量，读取耗时几百毫秒，可接受。
3. SQLite 开启 WAL 与 `busy_timeout=5000`。
4. 事务粒度为单文档：中途中断不会留下半个文档的片段。
5. `meta` 记录嵌入模型名与维度；配置与库内记录不一致时，`ingest` 直接报错，要求重建库，禁止新旧向量混存。
6. `meta.schema_version` 第一期固定为 v1，不实现迁移；库内版本与程序不一致时直接报错，提示删除库文件后重建。
7. `stats` 显示片段数；超过 20 万片段时提示建议迁移 Qdrant，但不阻止。
8. 规模上限参考：16GB 内存下，20 万片段约 800MB 向量 + 原文，仍在可行范围。

## 9. 入库流程

```text
扫描目录 → 按扩展名选 Loader → 统一 Document
        → Chunker 切分 → 批量嵌入 → 事务写入
```

### 9.1 扫描

- 递归遍历；跳过隐藏目录、`.git`、`node_modules`。
- 扩展名不区分大小写：`.md` `.markdown` `.txt` `.docx` `.pdf`。
- 单文件失败记录日志并继续；结束时汇总成功/跳过/失败数量；有失败则退出码 1。

### 9.2 解析

| 格式 | 做法 | 已知限制 |
|------|------|---------|
| TXT/MD | 先校验 UTF-8，失败回退 GB18030；MD 用 goldmark 提取标题层级与正文 | 基本无 |
| DOCX | 标准库解压读取 `word/document.xml`，提取段落、标题样式；表格按行拼文本 | 不读页眉页脚、脚注、批注 |
| PDF | 纯 Go 库逐页提取文本，页码作为结构标记 | 无标题结构；表格/双栏会乱；中文 CID 字体可能乱码，需真实文件验证 |

PDF 策略：实现早期用真实 PDF 做提取测试。若纯 Go 库效果不可接受，新增基于外部 `pdftotext`（poppler）子进程的 loader 兜底。该兜底为设计内预案，使用它时需额外安装 poppler。

### 9.3 切分

- 先按标题边界切，片段不跨标题；每片段携带标题路径。
- 标题下按段落打包，达到约 500 字符封顶。
- 超长段落按中文句末标点（`。！？；`）切分，片段间重叠 80 字符，避免断句。
- 丢弃纯空白、导航文本、少于 10 字的碎片。
- PDF 无标题，使用 `第 N 页` 作为结构标记，按段落/长度切分。

### 9.4 嵌入

- 调用 Ollama `/api/embed`，批量提交（默认 16 条/批）。
- 失败指数退避重试 3 次。
- 校验返回维度与 `meta.dim` 一致，不一致直接失败。
- 文件串行处理（CPU 推理为瓶颈），每个文件打印片段数与耗时。
- 增量语义：
  - hash 未变 → 跳过。
  - hash 变化 → 先解析并完成全部嵌入，再在单事务中删旧片段、插新片段、更新文档状态；嵌入中途失败则保留旧版本，文档标 `failed`，下次 ingest 重试。
  - 源文件被删除 → 库内记录不自动清理，避免误传目录导致误删。

## 10. 查询与生成

```text
ask "问题"
  1. 校验嵌入模型与库内 meta 一致
  2. 问题走 Ollama 嵌入 → 1024 维向量
  3. 内存点积，取 top-k（默认 8）
  4. 按编号拼装上下文（文件路径 + 标题路径 + 原文）
  5. 调用云端 OpenAI 兼容接口生成
  6. 输出答案 + 引用来源
```

### 10.1 上下文格式

```text
资料：
[1] 来源：docs/人事制度/休假管理办法.docx
    标题：人事制度 > 休假 > 年假
    内容：员工累计工作满1年不满10年的，年休假5天……

[2] ...
```

### 10.2 Prompt 约束

- 只依据资料回答；资料中没有时明确回答"知识库中未找到相关依据"，禁止编造。
- 每个结论后用 `[1]` `[2]` 标注依据。
- 中文回答；温度 0.2；`max_tokens` 1024。

### 10.3 输出格式

```text
根据《休假管理办法》，累计工作满1年不满10年的员工享5天年假[1]；
满10年不满20年为10天[2]。

引用来源：
[1] docs/人事制度/休假管理办法.docx > 人事制度 > 休假 > 年假
[2] ...
```

### 10.4 第一期取舍

- 不接重排、不做混合检索、不做问题改写。
- 不做相似度阈值过滤；检索恒返回 top-k，由 prompt 约束拒答，依赖人工评测观察可靠性。
- 不流式输出；拿到完整答案后打印。
- 不做多轮对话，每次 `ask` 独立。
- 生成失败时仍打印检索片段并给出警告，用于区分检索问题与生成问题。

## 11. 日志与错误处理

### 11.1 日志

- 同时输出到 stderr 与文件，默认 `./data/logs/KB-RAG.log`，配置项 `log.file` 可改。
- 使用 `lumberjack.v2` 按 10MB 轮转，保留 5 份。
- 使用标准库 `log/slog`，日志行包含时间戳、级别、命令、文档相对路径。
- INFO：每文件成功/跳过/失败、片段数、耗时、入库汇总。
- WARN/ERROR：可恢复/不可恢复错误及原因。
- **文档正文与问题原文不落盘**。问题只记录长度、耗时、top-k 片段 ID 与相似度分数。
- API Key 永不写入日志。

### 11.2 退出码

| 场景 | 退出码 | 行为 |
|------|-------|------|
| 配置/参数错误 | 2 | 打印具体字段错误 |
| ingest 部分文件失败 | 1 | 其余文件正常入库，汇总失败清单 |
| Ollama 不可达 / 库为空 | 1 | 提示检查服务或先执行 ingest |
| 生成失败但检索成功 | 1 | 打印警告 + 检索片段 |
| 用户 Ctrl+C | 130 | 当前事务回滚，已完成文档保留 |

## 12. 测试与验收

### 12.1 单元测试

- Chunker：标题边界不跨越、500 字封顶、中文句末切分、80 字重叠、碎片丢弃。
- Loader：UTF-8/GBK TXT、MD 标题层级、DOCX 最小样例、PDF 最小样例。
- Store：hash 未变跳过、变更后旧片段原子替换、维度不一致拒绝写入、top-k 排序正确。
- Config：缺字段、非法 URL、`size <= overlap` 等校验。

### 12.2 集成测试

- 假 Ollama HTTP 服务返回确定性向量（测试用低维配置）。
- 假 OpenAI 兼容服务捕获 prompt 并返回固定答案。
- 验证临时目录 → ingest → ask 全链路；引用编号与来源对应；生成失败时片段兜底输出。
- 真实 PDF/DOCX 的中文解析只做人工验证，不做精确断言。

### 12.3 人工评测

- 20-30 个真实业务问题，记录在 `docs/eval/questions.md`。
- 每条记录：正确片段是否进入 top-8、答案是否正确、引用是否准确。
- 另加 3-5 个知识库中无答案的问题，验证拒答行为。

验收线：

- 正确片段进入 top-8：≥ 85%
- 答案正确且引用无误：≥ 80%
- 无答案问题拒答：≥ 80%

### 12.4 环境冒烟

- Ollama 已启动、`bge-m3` 已拉取，`/api/embed` 返回 1024 维。
- 云端 LLM Key 可用、接口连通。
- 入库样例文档后 `stats` 数据合理。

### 12.5 实现前依赖的用户输入

1. 每种格式各几份有代表性的真实文档（可脱敏）。
2. 20-30 个真实问题与预期答案（评测阶段补齐即可）。

## 13. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| Go 纯 Go PDF 库中文提取乱码 | PDF 内容不可用 | 实现早期用真实 PDF 验证；不行则切换 `pdftotext` 子进程 |
| CPU 嵌入吞吐低 | 全量入库耗时长 | 先用代表性子集验证；实测后再全量后台运行 |
| DOCX 结构还原有限 | 标题层级不准，影响切分 | 标题缺失时退化为按段落/长度切分 |
| 云端 LLM 收到敏感片段 | 数据合规 | 仅发送 top-k 命中片段；评测使用脱敏数据；后续可换本地生成模型 |
| 向量维度变化 | 检索结果错乱 | `meta` 记录模型与维度，不一致直接拒绝 |
| 大库内存占用 | 16GB 机器压力 | `stats` 超过 20 万片段提示迁移 Qdrant |

## 14. 后续扩展方向（不在第一期承诺）

- 换 Qdrant 支撑更大规模与多租户过滤
- 重排模型、混合检索、问题改写
- Web 界面、权限与多用户
- 扫描件 OCR
- 本地部署生成模型，实现全离线
- 自动化评测（hit rate / MRR / 忠实度）
