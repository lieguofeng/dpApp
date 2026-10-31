# 微服务部署服务（dpApp）设计规范

> 日期：2026-10-31
> 状态：基于前端原型（dpapp-demo.html）整理，待评审

## 1. 概述

**微服务部署服务**（代号 dpApp）是一个用 Go 编写的 Java JAR 微服务应用部署管理平台。中心服务（Center）提供 Web 管理界面（Vue 3 + Element Plus + ECharts），通过统一的「连接器（Connector）」抽象管理多台机器上的 Java 微服务应用，支持启停、日志采集/查看、JVM 内存监控、配置文件编辑、Nacos 配置获取/编辑/发布、JAR 上传替换与滚动发布，并提供**应用级部署目录与启动命令配置**、**版本回滚**、**数据库主备切换**等能力。

### 1.1 核心概念（术语）

| 概念 | 说明 | 例子 |
|------|------|------|
| **机器（Machine）** | 目标服务器，支持 SSH 直连与 Agent 两种接入方式 | `prod-web-01` |
| **应用（Application / 微服务）** | 逻辑单元，承载部署目录与启动命令，多个部署实例的总称 | `nsys` |
| **实例（Instance）** | 应用在某台机器上的一个部署（JAR 文件名 + 版本 + 端口 + pid） | `nsys@prod-web-01` |
| **连接器（Connector）** | 对目标机的统一远端操作接口，屏蔽 SSH 与 Agent 差异 | — |
| **发布（Release）** | 一次应用级的版本发布记录，含单实例发布与滚动发布 | v1.4.3 |
| **数据库主备（DB primary/backup）** | 应用数据源的读写主库与只读备库，支持一键切换 | `10.0.0.8` / `10.0.0.9` |

### 1.2 目标范围（本期）

- 多机管理，支持 **SSH 直连** 与 **Agent** 两种接入方式（新增/编辑/删除/测试连接）
- 应用级配置：**JAR 目录 / 日志目录 / 启动文件目录 / 配置文件目录 / 配置文件列表 / 启动命令**（目录留空按 `/opt/{应用名}` 自动推导）
- 实例生命周期：启停 / 重启 / 状态（pid、端口探活）
- 日志：**多日志文件**（app.log / error.log / gc.log / 滚动归档 .gz）、历史回放 + 实时 tail、过滤、下载 / 实时下载
- JVM 内存监控：堆 / Metaspace / GC / 线程（近 1 小时趋势）
- 配置：本地文件（YAML/properties 结构化编辑 + XML/text 文本编辑 + diff + 历史）
- Nacos：按 **IP（机器）+ 应用** 过滤列出配置；**yaml / properties 结构化编辑** 与文本编辑；发布回 Nacos
- 部署：JAR 上传替换（分片/断点续传）、**单实例发布**、**应用级滚动发布**
- 版本管理：**发布历史 + 版本回滚**
- **数据库主备切换**：更新 `spring.datasource.url` 指向目标库并重启实例生效
- 安全：单管理员登录、凭据加密落库
- 存储：SQLite
- 前端：Vue 3 + Element Plus + ECharts，`go:embed` 单二进制

### 1.3 非目标（本期不做）

- 多用户 / 角色权限（RBAC）
- 多中心副本、外部数据库
- Kubernetes / 容器编排
- 邮件/IM 告警通道（仅页面内告警）
- 数据库自动故障探测（切换为人工触发，不做自动 failover）

## 2. 总体架构

```
┌─────────────────────────────────────────────────────────┐
│                    浏览器 (Vue3 + Element Plus + ECharts) │
│  登录 / 机器·应用·实例 / 启停 / 日志 / 监控 / 配置 / 发布 │
│              / 数据库切换 / 版本回滚                      │
└───────────────▲──────────────────────▲──────────────────┘
                │ HTTPS / REST + WebSocket(日志、发布进度)
┌───────────────┴──────────────────────┴──────────────────┐
│                    Center (Go 中心服务)                   │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────┐  │
│  │  API/鉴权 │ │ Connector │ │  部署编排  │ │ 配置编辑器  │  │
│  └──────────┘ └─────┬────┘ └──────────┘ └────────────┘  │
│        ┌────────────┼─────────────┐  数据库切换 / 版本回滚 │
│        │        SQLite          Nacos客户端(HTTP)        │
└────────┼────────────┼─────────────┼──────────────────────┘
   SSHConnector   AgentConnector        │
        │              │          Nacos Server
   ┌────┴─────┐   ┌────┴─────┐
   │ 目标机    │   │ 目标机    │
   │ sshd     │   │ Agent    │
   │ java -jar│   │ java -jar│
   └──────────┘   └──────────┘
```

## 3. 组件划分

| 组件 | 职责 | 依赖 |
|------|------|------|
| **Center** | HTTP API + 鉴权、机器/应用/实例 CRUD、启停/日志/监控/配置/发布/回滚/数据库切换编排、SQLite 存取、Nacos 客户端 | 无（独立二进制 `dpapp-center`） |
| **Agent** | 跑在目标机上，收 Center 指令：启停进程、tail 日志、采集 JVM 指标、读写配置、接收 JAR | 无（独立二进制 `dpapp-agent`） |
| **Connector** | 统一远端操作接口 | `x/crypto/ssh` / HTTP 客户端 |
| **配置编辑器** | YAML/properties 结构化读写 + diff | YAML 解析库 |
| **Nacos 客户端** | Nacos OpenAPI 封装（登录/命名空间/配置列表/读取/发布） | HTTP |
| **数据库切换器** | 更新目标配置文件的 `spring.datasource.url` 并触发实例重启 | Connector + 配置编辑器 |
| **前端** | 单页管理界面，构建产物由 Center `go:embed` 内置 | Center |

## 4. 核心抽象：Connector 接口

```go
type Connector interface {
    StartApp(ctx context.Context, app Instance, args []string) (pid int, err error)
    StopApp(ctx context.Context, app Instance) error
    GetStatus(ctx context.Context, app Instance) (Status, error)
    TailLog(ctx context.Context, app Instance, file string, offset int64, follow bool) (<-chan LogLine, error)
    ListLogs(ctx context.Context, app Instance) ([]LogFile, error)   // 多日志文件
    ReadConfig(ctx context.Context, app Instance, path string) ([]byte, error)
    WriteConfig(ctx context.Context, app Instance, path string, content []byte) error
    UploadFile(ctx context.Context, app Instance, src io.Reader, dest string) error
    GetJvmMetrics(ctx context.Context, app Instance) (JvmMetrics, error)
    Health(ctx context.Context) (HealthInfo, error)
}
```

- **SSHConnector**：`x/crypto/ssh` 直连 + `pkg/sftp` 传文件，`tail -n N -f` 流日志，`ls` 列出日志目录文件，目标机执行 `jstat` 采指标。
- **AgentConnector**：HTTPS + WebSocket，token 鉴权。
- Center 面向接口编程，`machines.mode` 决定实例化哪个实现；新增接入方式 = 加一个实现。
- `TailLog` / `ListLogs` 以**应用级日志目录**（`application.log_dir`）为基准，文件名由参数指定，支持多文件。

### 4.1 Status / JvmMetrics / HealthInfo / LogFile 定义

```go
type Status struct {
    Running bool
    PID     int
    Port    int
    UpSince time.Time
}

type JvmMetrics struct {
    HeapUsed, HeapCommitted, HeapMax uint64
    NonHeapUsed, NonHeapCommitted    uint64 // Metaspace 等
    YoungUsed, OldUsed               uint64
    YGC, FGC, YGCTimeMs, FGCTimeMs   uint64 // GC 次数 / 耗时
    ThreadCount                      int
    Timestamp                        time.Time
}

type HealthInfo struct {
    Reachable bool
    Error     string
}

type LogFile struct {
    Name    string // app.log / error.log / gc.log / app-2026-10-30.log.gz
    Size    int64
    MTime   time.Time
}
```

## 5. 数据模型（SQLite）

```
machines        id | name | host | port | mode(ssh|agent)
                ssh_user | ssh_secret_enc | ssh_key_enc
                agent_url | agent_token_enc
                status | last_heartbeat_at | created_at

applications    id | name(UNIQUE) | description
                jar_dir | log_dir | bin_dir | cfg_dir | start_cmd
                config_files(JSON, 相对 cfg_dir 的文件名列表)
                created_at

instances       id | application_id | machine_id | jar(文件名) | version
                port | pidfile | status | created_at
                -- 部署目录/启动命令由 application 提供，实例不重复存储

deployments     id | instance_id | release_id(NULLABLE) | version | jar_sha256
                operator | status(success|failed|rolledback)
                started_at | finished_at | note

releases        id | application_id | version | jar_sha256 | operator
                mode(single|rolling) | batch_size
                status(pending|running|paused|success|failed|cancelled)
                is_current(bool) | current_batch | started_at | finished_at | note

release_items   id | release_id | instance_id | batch_no
                status(pending|deploying|healthy|failed|rolledback|skipped)
                old_jar | error | finished_at

config_history  id | instance_id(NULLABLE) | source(local|nacos)
                file_path | old_content | new_content | operator | created_at
                -- local: file_path=相对路径; nacos: file_path=nacos:<dataId>@<group>#<namespace>

db_configs      id | application_id | primary_url | backup_url
                current(primary|backup)

settings        key(PK) | value
```

> 凭据（SSH 密码/私钥、Agent token、Nacos 密码）一律 **AES-GCM 加密**存储，主密钥由管理员密码派生或存 key 文件。

> 版本回滚 = 在 `releases` 中追加一条目标版本的记录（`note="回滚至 vX"`，`is_current=true`），并据其重新执行部署；单实例视角的每次实际部署写入 `deployments`。

## 6. 关键流程

### 6.1 启停 / 状态

- **Start**：在应用 `jar_dir` 下按应用 `start_cmd`（或默认 `java -jar <jar>`）后台启动，写 pidfile。
- **Stop**：读 pidfile，`SIGTERM`，超时（默认 30s）未退出再 `SIGKILL`。
- **Status**：pidfile + `ps`/`jps` 校验 + 可配端口 TCP 探活。

### 6.2 日志采集（多文件）

- 实例的日志文件位于应用 `log_dir` 下，**一个实例可有多个日志文件**（app.log / error.log / gc.log / 滚动归档 .gz）。
- 前端日志页先 `ListLogs` 拉取文件列表，用户选择文件后再 `tail -n 1000` 回放 + `tail -f` 实时流。
- SSH 走 ssh session，Agent 走 WebSocket；Center 统一转成浏览器 WebSocket。
- 断线自动重连、按行缓冲、前端支持过滤/暂停/下载。
- 日志下载：`下载日志` = 取当前文件（`tail -n N` 或全量）；`实时下载` = `tail -f` 流式持续落盘，前端结束即导出为文件。

### 6.3 JVM 内存监控

- **数据来源（默认免改应用）**：
  1. `jstat -gc <pid>`（目标机装 JDK）——堆/年轻代/老年代/Metaspace + GC 次数与耗时
  2. JMX（`java_opts` 已配 `-Dcom.sun.management.jmxremote…`）——更丰富
  3. Spring Actuator（应用暴露 `/actuator/metrics`）
- 默认 `jstat`；已配 JMX/Actuator 则自动走更丰富来源。SSH 模式 Center 经 ssh 执行，Agent 模式 Agent 本机执行。
- Center 对每个运行实例起后台采集器，每 5s 调 `GetJvmMetrics`，存入**进程内环形缓冲**（最近 1h ≈ 720 点），**不落 SQLite**；`jstat` 不可用时标记「监控不可用」。
- 前端每 5s 轮询 `GET /api/instances/:id/metrics`。

### 6.4 配置编辑（本地文件）

- 支持格式：`yaml` / `properties`（结构化编辑）+ `xml`（如 `log4j2.xml`）/ `text`（文本编辑）。
- 结构化（YAML/properties）：读文件 → 解析成键值结构 → 前端键值编辑 → 保存前 **old/new diff** 预览 → **surgical patch** 仅替换被改键的原文片段，保留注释与格式。
- 文本（XML/纯文本）：编辑器带语法高亮，保存前同样出 diff 预览，整体替换写回。
- 写回远端后记录 `config_history`。
- 配置文件以 `application.config_files` 为清单，路径 = `cfg_dir + 文件名`。

### 6.5 Nacos 配置

- **Center 直连 Nacos**（HTTP 8848），不走 Connector。模块 `internal/nacos`。
- 能力：`Login`（可选鉴权，拿 accessToken）、`ListNamespaces`、`ListConfigs`（模糊搜索 dataId/group）、`GetConfig`、`PublishConfig`。
- **按 IP（机器）+ 应用过滤**：Nacos 配置列表支持按目标机器 IP 与应用名二次筛选。
- 格式 `format`：`yaml`/`properties`/`text`/`json`/`xml`。
  - **yaml / properties**：结构化键值编辑 + 文本编辑两种模式（properties 同样支持键值修改）。
  - 其余格式：文本编辑。
- 编辑复用 `configedit`：拉取 → 结构化树 → 编辑 → diff 预览 → 序列化 → `PublishConfig` 发布。
- 变更写 `config_history`（`source=nacos`）做本地审计；Nacos 服务端自带版本历史可互补。

### 6.6 JAR 上传替换（单实例）

> JAR 典型 200–300MB，上传采用**分片 + 断点续传**，全程流式、不整包进内存。

1. 前端用 `Blob.slice` 把 JAR 切成 5MB 分片，逐片上传（带进度、可断点续传）；Center 收分片落临时文件并记录已收分片，重传时跳过已完成片。
2. 收齐后流式计算 SHA256，校验一致 → 得到临时 JAR 引用。
3. 推送目标机（SFTP / Agent 分片上传，均流式写）。
4. 停实例 → 旧 JAR 备份为版本目录 → 替换 → （如有配置变更则写配置）→ 启动 → 探活校验。
5. 写 `deployments`（成功/失败/回滚点）；临时文件清理。

### 6.7 发布（单实例 / 应用级滚动）

- 发布对话框二选一：**单实例发布**（指定目标实例）或 **应用滚动发布**（该应用下所有实例）。
- 单实例：仅替换选定实例的 JAR，更新其版本。
- 滚动：发布单位 = **应用**；滚动对象 = 该应用下的**多个实例**；`batch_size` = 每批几个实例。

```
创建发布(选应用+上传JAR+版本+batch_size / 单实例+目标实例)
  → [滚动] 应用下实例按批次划分
  → 逐批执行：
      批内每实例: 停 → 备份旧JAR → 替换 → (写配置) → 启动
      健康检查(端口探活 + 启动日志关键字, 可配超时默认60s)
      ├─ 全通过 → healthy → 下一批
      └─ 任一失败 → 状态=paused → 页面告警 → 等待人工
  → 人工: [继续(重试失败项)] / [回滚已发布批次] / [取消]
  → 全部 healthy → success
```

- 每个实例的实际部署写 `deployments`（带 `release_id`），保证单实例视角部署历史完整。

### 6.8 版本回滚

- 发布历史按应用展示（版本 / 时间 / 操作人 / 说明 / 是否当前）。
- 对非当前版本提供「回滚到此版本」：确认后以目标版本创建新的 `release`（`note="回滚至 vX"`），执行与常规发布相同的「停 → 备份 → 替换 → 启动 → 探活」流程。
- 成功后置 `is_current=true`，旧当前版本标记失效；实例版本号与 JAR 文件名同步更新。
- 回滚走 `deploy` 编排，失败同样可再回滚，历史完整可追溯。

### 6.9 数据库主备切换

- 每个应用在 `db_configs` 维护 `primary_url` / `backup_url` / `current`。
- 切换流程（人工触发）：
  1. 校验备库可用
  2. 切换数据库连接（更新配置文件的 `spring.datasource.url` 指向目标库）
  3. 更新应用配置（写回远端，经 Connector）
  4. 重启实例生效
- 主库为读写、备库为只读（MySQL 8.0 示例）；切换完成后 `current` 翻转，前端主备卡片实时标注「当前连接 / 备用」。

## 7. 安全

- 单管理员：bcrypt 存密码 hash，登录发 JWT，中间件保护全部 `/api`。
- SSH：首连记录 host key 指纹防中间人；支持密码 / 私钥两种。
- Agent：固定 token 鉴权，建议 TLS。
- Nacos：密码 AES-GCM 加密落库；可选关闭鉴权（内网）。
- 上传文件做大小 / 类型限制。

## 8. 错误处理

- Connector 错误归一化为类型化错误（`ErrConnTimeout` / `ErrAppNotRunning` / `ErrAuthFailed`…），API 层统一转 HTTP 状态码 + 中文提示。
- 部署 / 启停 / 回滚 / 数据库切换为可耗时操作：同步执行带超时；部署记录状态机，失败可回滚（恢复旧 JAR + 旧配置）。
- 滚动发布失败 → `paused`，等待人工继续/回滚/取消，不自动带病推进。
- 数据库切换前校验备库可用，任一步失败即中止并回滚连接配置。
- 日志流处理断连 / 重连，不拖垮中心。
- 大文件传输（上传/推送）使用更长超时与重试；分片上传断点续传，网络中断不丢已完成分片。

## 9. 技术选型

| 项 | 选型 | 理由 |
|----|------|------|
| 语言 | Go 1.22+ | 单一语言，易交叉编译 |
| HTTP 框架 | Gin | 中间件 / JSON 绑定成熟 |
| SQLite 驱动 | `modernc.org/sqlite` | 纯 Go 无 CGO，跨平台易交叉编译 |
| SSH | `x/crypto/ssh` + `pkg/sftp` | 官方生态 |
| YAML | `goccy/go-yaml`（注释保留） | 结构化编辑需回写注释 |
| WebSocket | `gorilla/websocket` | 日志流 / 进度 |
| 前端 | Vue 3 + Vite + Element Plus + Pinia + ECharts | 已选定；图表配色遵循 dataviz 规范 |

## 10. 项目结构

```
tlDeployWork/
├── cmd/
│   ├── center/main.go        # 中心服务入口
│   └── agent/main.go         # 目标机 agent 入口
├── internal/
│   ├── api/                  # 路由、handler、鉴权中间件
│   ├── connector/            # 接口 + ssh/ + agent/ 两实现
│   ├── store/                # SQLite 数据访问
│   ├── configedit/           # YAML/properties 结构化编辑 + diff
│   ├── nacos/                # Nacos OpenAPI 客户端
│   ├── deploy/               # 部署编排（单实例/滚动发布/回滚）
│   ├── dbswitch/             # 数据库主备切换编排
│   ├── metrics/              # JVM 指标采集器 + 环形缓冲
│   └── auth/                 # 登录、token、凭据加解密
├── web/                      # Vue3 前端源码
├── static/                   # 前端构建产物（embed 目标）
└── docs/
```

## 11. 页面设计

整体布局：左侧深色侧边导航（品牌标题「微服务部署服务」）+ 顶部栏（面包屑 + 当前用户）+ 内容区。侧边菜单：总览 / 机器管理 / 应用管理 / 数据库切换 / 日志管理 / 配置管理 / Nacos 配置 / 设置。

- **P1 登录页**：居中卡片（品牌标题 + 用户名 / 密码），失败红条提示。
- **P2 总览 Dashboard**：统计卡片（机器 / 应用 / 运行中 / 已停止）；实例状态饼图；最近部署记录表。
- **P3 机器管理**：表格（名称 / 主机:端口 / 模式 / 状态 / 操作[测试连接·编辑·删除]）；新增/编辑弹窗按模式切换字段（名称 / 主机·IP / 端口 / 连接模式[SSH 直连·Agent] / 用户名 / 密码），带凭证加密提示。
- **P4 应用管理**：微服务应用列表（应用名 / 描述 / 实例数 / 运行·停止统计 / 操作[详情·滚动发布]）；新增/编辑弹窗配置 **JAR 目录 / 日志目录 / 启动文件目录 / 配置文件目录 / 配置文件 / 启动命令**。
- **P5 应用详情**：顶部返回 + 描述；**四个独立目录卡片**（JAR 目录 / 日志目录 / 启动文件目录 / 配置文件目录）；启动命令卡片（含「编辑应用配置」）；实例列表（机器 / 状态 / 端口 / PID / JAR / 操作[详情·启动|重启·停止]）；**发布历史表**（版本 / 发布时间 / 操作人 / 说明 / 操作[回滚到此版本]）。
- **P6 实例详情 Tab**：
  - **Tab1 概览**：基本信息（状态 / PID / 端口 / JAR 版本 / 启动时间）+ 部署配置（JAR 位置 / 配置文件 / 日志路径 / 启动命令，均派生自应用级配置）。
  - **Tab2 日志**：终端风格（黑底、行号、关键词高亮、过滤）。
  - **Tab3 监控**：卡片（堆 used/max、Metaspace、GC 次数、线程数）+ 堆趋势面积图、GC 耗时折线、线程数折线；5s 刷新；不可用时占位。
- **P7 部署对话框**：选择「单实例发布」（目标实例选择器）或「应用滚动发布」（应用 + 每批数量 batch_size）；JAR 拖拽上传（分片进度条 + 断点续传）、版本号；部署中显示阶段步骤（上传→停止→备份→替换→写配置→启动→健康检查）。
- **P8 数据库切换**：选择应用；主库 / 备库两张卡片（标注「当前连接 / 备用」+ 连接串 + 读写/只读）；「执行数据库切换」弹出步骤（校验备库可用→切换连接→更新配置→重启实例）。
- **P9 日志管理**：顶部按「机器 → 实例 → **日志文件**」选择（多文件），实时 tail；关键字过滤；「下载日志」（当前文件）与「实时下载」（跟随流持续捕获，结束即导出）。
- **P10 配置管理（本地文件）**：顶部选 应用 → 实例；左侧显示部署目录 + 配置文件列表（application.yml / application.properties / log4j2.xml 等）；右侧按格式结构化编辑（YAML/properties 键值、XML/文本）+ [查看差异][保存][历史版本]。本地配置为实例级（每台机器各自一份）。
- **P11 Nacos 配置管理**：顶部展示并可编辑 **Nacos 连接信息**（IP、端口、命名空间、用户名、密码，密码加密存储）；**按 IP（机器）+ 应用过滤**；左配置列表（dataId + 类型标签，超长省略）；右 **yaml/properties 结构化键值编辑 + 文本编辑**（properties 同样支持修改）+ [查看差异][发布到 Nacos]。
- **P12 设置**：全局参数（JVM 采集间隔、日志回放行数、健康检查超时、分片大小）。

## 12. API 摘要

| 模块 | 方法 | 路径 |
|------|------|------|
| 鉴权 | POST/GET | `/api/login` `/api/logout` `/api/me` |
| 机器 | CRUD + test | `/api/machines` `/api/machines/:id` `/api/machines/:id/test` |
| 应用 | CRUD | `/api/applications` `/api/applications/:id` |
| 实例 | CRUD | `/api/applications/:id/instances` `/api/instances/:id` |
| 实例操作 | POST | `/api/instances/:id/start\|stop\|restart\|status` |
| 日志文件列表 | GET | `/api/instances/:id/log-files` |
| 日志 | WS | `/api/instances/:id/logs?file=app.log` |
| 日志下载 | GET | `/api/instances/:id/logs/download`（当前文件）/ `.../logs/follow`（流式跟随下载） |
| 监控 | GET | `/api/instances/:id/metrics` |
| 配置 | GET/PUT | `/api/instances/:id/config` `/api/instances/:id/config/:file` |
| JAR 分片上传 | POST/PUT/DELETE | `/api/uploads` `/api/uploads/:id/chunk/:seq` `/api/uploads/:id/complete` `/api/uploads/:id/status` `/api/uploads/:id/cancel` |
| 单实例部署 | POST | `/api/instances/:id/upload`（引用已上传 JAR） |
| 滚动发布 | POST/GET | `/api/releases` `/api/releases/:id` `/api/releases/:id/continue\|rollback\|cancel` |
| 版本回滚 | POST | `/api/releases/:id/rollback` |
| 部署历史 | GET | `/api/deployments` |
| 数据库切换 | GET/POST | `/api/db-configs` `/api/applications/:id/db-switch` |
| Nacos | GET/PUT | `/api/nacos/namespaces` `/api/nacos/configs?ip=&app=` `/api/nacos/config` `/api/nacos/config/diff` |
| 设置 | GET/PUT | `/api/settings` |

## 13. 测试策略

- **单元**：配置编辑器 round-trip 与 diff（含 properties 解析/序列化）、凭据加解密、deploy/滚动发布状态机（mock Connector）、版本回滚状态机、数据库切换状态机。
- **集成**：SSH 连接器对本地 Docker/VM（含 sshd）测试；Agent 连接器对真实 agent 进程测试。
- **前端**：Vitest 冒烟（可选）。
- **上传**：大文件（≥200MB）分片 / 断点续传、SHA256 校验用例。

## 14. 里程碑

1. **M1 骨架**：Center + SQLite + 登录鉴权 + 机器/应用/实例 CRUD（含应用级目录与启动命令配置）+ 最小页面
2. **M2 SSH 连接器**：启停 + 日志 tail（多文件）+ 配置读写
3. **M2.5 Nacos**：Nacos 客户端 + P11 页面 + IP/应用过滤 + yaml/properties 编辑/发布（与 M4 并行）
4. **M3 Agent 连接器** + Agent 程序
5. **M4 配置**：结构化编辑（YAML/properties）+ diff + 历史
6. **M5 部署**：JAR 上传替换 + 单实例发布 + 滚动发布 + 版本回滚
7. **M5.5 数据库切换**：db_configs + 切换编排 + P8 页面
8. **M6 JVM 监控**：指标采集 + 环形缓冲 + 监控页
9. **M7 前端打磨** + 打包（go:embed 单二进制）+ 文档
