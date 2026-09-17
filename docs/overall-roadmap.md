# StreamHub 总体升级路线（Go 全栈 + 云原生）

> 这是一份**北极星主计划**。它把项目从一个「能跑的三服务教学 Demo」升级成一个能证明 **Go 全栈开发能力 + 云原生理解** 的作品级项目。
>
> 定位：本文件是唯一权威路线图，取代之前分散的 `feature-guide.md` / `code-optimization-guide.md` / `upgrade-guide.md` / `frontend-guide.md`（保留作历史参考，不再维护）。`progressive-refactor-guide.md` 和 `stage-*.md` 继续作为**细粒度教学步骤**，本文件负责「大方向 + 里程碑 + 验收」。

## 0. 当前进度快照

- 阶段 0–3 已完成/收尾中（基线 → 统一错误 → 输入校验 → 统一 HTTP 契约）。
- 阶段 4（密码哈希）文档已写，代码待做。
- 工作区仍有 4 个未提交文件（阶段 3 的改动），需要先提交。

## 1. 北极星目标

一句话：**一个可容器化部署到 Kubernetes、可观测、带真实流媒体能力、由现代 React SPA 驱动的视频平台，后端全部用 Go 实现。**

完成后，一个面试官 / 代码评审者翻这个仓库时，应该能在一小时内看出你掌握了：

1. 干净的 Go 分层架构（handler / service / repository，接口驱动、依赖注入）。
2. 可测试性（确定性单测、mock/fake、集成测试与单测分离）。
3. 真实的 API 设计（REST 约定、分页、版本化、OpenAPI 契约、类型化客户端）。
4. 鉴权与安全（密码哈希、JWT/会话、RBAC、限流、路径安全、CORS 收紧）。
5. 流媒体与对象存储（HTTP Range、分片/断点上传、MinIO/S3、预签名 URL）。
6. 并发与异步（可靠 worker、outbox、幂等、优雅停机、context 传播）。
7. 云原生（多阶段 Docker、K8s Deployment/Service/Ingress、探针、ConfigMap/Secret、Helm）。
8. 可观测性（结构化日志 + request-id、Prometheus 指标、OpenTelemetry 追踪、健康检查）。
9. 交付闭环（GitHub Actions：lint → test → build → push → deploy）。
10. 前端工程化（React/Vue + TypeScript + Vite，与后端共享类型契约）。

## 2. 目标架构

```
                    ┌─────────────────────────────┐
                    │  React SPA (Vite + TS)      │
                    │  登录 / 列表 / 详情 / 评论 / 上传  │
                    └──────────────┬──────────────┘
                                   │ REST (JSON, 类型化契约来自 OpenAPI)
                    ┌──────────────▼──────────────┐
                    │  api  (Go, :8080)           │
                    │  用户 / 鉴权 / 元数据 / 评论    │
                    └───────┬──────────┬──────────┘
                            │          │
                 ┌──────────▼──┐   ┌───▼──────────────┐
                 │ MySQL       │   │ streamserver      │
                 │ (元数据)     │   │ (Go, :9000)       │
                 └──────────┬──┘   │ Range 流 / 分片上传 │
                            │      └────────┬─────────┘
                 ┌──────────▼───────────────▼─────────┐
                 │  MinIO / S3 (视频对象存储)           │
                 └───────────────────────────────────┘

                 ┌───────────────────────────────────┐
                 │  scheduler (Go worker)            │
                 │  可靠异步任务：删除 / 转码 / 缩略图   │
                 │  幂等 outbox + 重试 + 优雅停机        │
                 └───────────────────────────────────┘

  贯穿所有服务：slog 结构化日志 / Prometheus 指标 / OTEL 追踪 / 健康与就绪探针 / 优雅停机
  部署：Docker 多阶段构建 → docker-compose 本地 → kind 上的 K8s（Helm）
```

## 3. 三条已定决策（记录在案，后续阶段不反复摇摆）

| 维度 | 决策 | 含义 |
| --- | --- | --- |
| 前端 | **React / Vue SPA** | 独立前端工程，TypeScript + Vite，用 OpenAPI 生成类型化客户端 |
| 云原生 | **本地全套 K8s** | Docker + compose 起步，落到 kind 上跑 Deployment/Service/Ingress + 探针，零云成本 |
| 流媒体 | **折中** | HTTP Range + 对象存储 + 分片/断点上传；HLS/ffmpeg 转码留到可选拓展 |

> 前端二选一：默认推荐 **React 18 + TypeScript + Vite + Tailwind**（生态、职位面、示例最多）；Vue 3 同构可替代。一旦选定就固定，不要再回头。

## 4. 能力地图（这个项目最终能证明什么）

| 能力域 | 关键证据 | 落在哪些阶段 |
| --- | --- | --- |
| Go 工程基础 | 分层、DI、错误封装、确定性测试、golangci-lint | P1 |
| API 设计 | OpenAPI 契约、分页、版本化、类型化客户端 | P2 |
| 鉴权安全 | 密码哈希、JWT/会话、RBAC、限流、路径安全 | P1 / P3 |
| 数据工程 | 迁移、事务、索引、repository 抽象 | P1 / P2 |
| 流媒体 | Range、分片上传、对象存储、预签名 URL | P5 |
| 并发分布式 | worker、outbox、幂等、重试、优雅停机 | P6 |
| 云原生 | 容器、K8s、探针、配置/密钥、Helm | P7 |
| 可观测性 | 日志/指标/追踪、健康检查 | P8 |
| 交付 | CI/CD、语义化版本、发布 | P8 |
| 前端 | React/Vue + TS 工程、类型契约、状态管理 | P4 |

## 5. 阶段总览（活状态表）

> 每完成一个阶段，把 `状态` 改成 `✅ 完成` 并写上完成日期；本表就是你的进度仪表盘。

| 阶段 | 主题 | 状态 |
| --- | --- | --- |
| P1 | 工程地基：可信、安全、可测的 Go 后端 | 🔵 进行中（承接 stage 0–11） |
| P2 | API 设计进阶：契约、分页、版本化、文档 | ⚪ 未开始 |
| P3 | 鉴权与安全进阶：JWT/会话、RBAC、限流 | ⚪ 未开始 |
| P4 | React SPA 前端 | ⚪ 未开始 |
| P5 | 流媒体与对象存储 | ⚪ 未开始 |
| P6 | 异步任务与并发健壮性 | ⚪ 未开始 |
| P7 | 云原生落地（Docker + 本地 K8s） | ⚪ 未开始 |
| P8 | 可观测性与交付闭环 | ⚪ 未开始 |

---

## 6. 各阶段详解

> 每个阶段用同一模板：**目标 → 交付物 → 涉及技术 → 证明什么 → 验收标准**。阶段之间有依赖，按顺序做，一次只做一个。

### Phase 1 — 工程地基：可信、安全、可测的 Go 后端

**目标**：把现有三服务代码从「能跑」升级到「可信」——正确、安全、分层、可测。这是后面一切的地基，没有它，云原生和流媒体都是建在沙子上。

**涉及技术**：Go 标准库 `database/sql`、`context`、`golang-migrate`（数据库迁移）、`golangci-lint`、`net/http/httptest` 测试、GitHub Actions。

**交付物**：
- 三层架构落地：`handler`（HTTP 解析）→ `service`（业务规则）→ `repository`（数据访问），接口驱动，依赖显式传入，移除 `dbops` 的包级全局 `dbConnection`。
- 数据库迁移改用 `golang-migrate`，`schema.sql` 变成 `migrations/0001_*.sql` 形式。
- 修复全部 P0/P1 安全与正确性问题：密码哈希（stage 4）、`streamsever` 路径穿越、session 生命周期（`RetrieveSession` 列名、`IsSessionValid` 逻辑、启动加载、TTL 类型）、评论 `id` 填进 `VideoId` 的 bug。
- 确定性测试：单元测试与集成测试分离（集成测试标记 tag 或 env 门控），去掉 `Sleep` 型测试。
- 加入 `golangci-lint`，在 CI 里跑 `gofmt` / `go vet` / `go test`。

**证明什么**：这是「Go 基本功」的门面。面试官看的是你**有没有把全局变量、明文密码、路径拼接、不可测的 goroutine 这些典型反模式主动清掉**，以及你有没有分层和写测试的纪律。

**验收标准**：
- [ ] `golangci-lint run` 通过，无高危告警。
- [ ] 每个 handler 依赖通过构造注入，`dbops` 不再有包级全局连接。
- [ ] 所有单测不需要真实 MySQL；集成测试显式门控并指向 `streamhub_test` 库。
- [ ] 密码、路径、session 三处安全/正确性问题全部修复并有测试覆盖。
- [ ] GitHub Actions 上 `lint` + `test` 绿灯。

**与现有文档的映射**：本阶段直接消费 `progressive-refactor-guide.md` 的 stage 4–11。你已经写了 stage 4（密码哈希），按 stage 5→6→7→… 一路做下去，做完即 P1 完成。

---

### Phase 2 — API 设计进阶：契约、分页、版本化、文档

**目标**：把「手写 JSON、状态码靠约定、无文档」的 API 升级成「契约先行、可生成客户端」的专业 REST API。

**涉及技术**：OpenAPI 3.x、`oapi-codegen`（Go 服务端 + TypeScript 客户端生成）、`/api/v1` 版本前缀、分页（`?page=` + `page_size` + 返回 `total`/`next_cursor`）、请求校验（gin validator 或生成的 schema）。

**交付物**：
- 一份 `openapi.yaml` 描述全部接口（这是「契约之源」）。
- 用 `oapi-codegen` 生成 Go handler 骨架与 DTO，手写部分只保留业务逻辑；响应类型不再散落在 `api/defs`。
- 列表接口（视频、评论）支持分页；评论按时间倒序 + 游标分页。
- 文档：`/docs` 或 Swagger UI 可直接浏览。

**证明什么**：API 不是「能返回 JSON 就行」，而是**有一个单一事实来源（OpenAPI），前后端从它生成类型，改契约时编译期就报错**。这是前后端协作和可维护性的核心理解。

**验收标准**：
- [ ] `openapi.yaml` 覆盖全部接口，`oapi-codegen` 生成代码可编译。
- [ ] 前端 TS 客户端类型从同一份 OpenAPI 生成（P4 会用到）。
- [ ] 列表接口分页参数生效，返回 `total` 与游标。
- [ ] 接口路径带 `/api/v1` 前缀，旧路径有明确迁移说明或直接废弃。

---

### Phase 3 — 鉴权与安全进阶：JWT/会话、RBAC、限流

**目标**：把「session id 存 localStorage + 明文头」的朴素鉴权升级成能写进简历的安全模型。

**涉及技术**：JWT（`github.com/golang-jwt/jwt/v5`）或 Redis 会话、httpOnly Cookie、刷新令牌、RBAC（用户/管理员角色）、`golang.org/x/time/rate` 限流、安全响应头、CORS 白名单。

**交付物**：
- 决策并落地：JWT（无状态）或 Redis 会话（有状态）——**二选一并写清理由**，不要混用。
- 登录态从 localStorage 迁移到 httpOnly Cookie（防 XSS 窃取）；前端不再手动塞 header。
- 角色字段 + 中间件：普通用户只能删自己的视频，管理员可删任意；RBAC 用中间件表达。
- 登录/注册等敏感接口加限流；CORS 收紧到 SPA 的 origin。
- 若多副本部署，把单机限流（`x/time/rate`）升级为 **Redis 分布式限流**（计数跨实例共享）——这是 Redis 在这里第一次「挣到位置」。

**证明什么**：你分得清「认证（你是谁）」和「授权（你能干什么）」，理解 JWT 与会话的取舍、CSRF 与 XSS 的边界、以及限流为什么放在边缘/中间件层。

**验收标准**：
- [ ] 鉴权方案有书面决策记录（JWT vs 会话的取舍）。
- [ ] 受保护接口通过 httpOnly Cookie 校验身份，RBAC 中间件生效。
- [ ] 非所有者删除返回 403，管理员可越权（有测试覆盖）。
- [ ] 限流在暴力登录场景生效（有测试或可演示）。

---

### Phase 4 — React SPA 前端

**目标**：把 `static/` 下三个原生 HTML 页面重写成一个真正的 React（或 Vue）单页应用，与后端通过类型化契约对接。

**涉及技术**：React 18（或 Vue 3）+ TypeScript + Vite + Tailwind，路由（react-router），数据请求（TanStack Query），状态管理（按需，不硬上 Redux），从 OpenAPI 生成的类型化客户端，`<video>` 播放器 + Range。

**交付物**：
- 独立前端工程（`web/` 目录），独立 `package.json`、`tsconfig`、构建产物。
- 登录/注册、视频广场、视频详情（播放器）、评论、我的视频、上传（带进度条）完整闭环。
- API 客户端用 P2 生成的类型，前端不再有手写 `any` 和硬编码 `localhost:9000/9090`。
- 环境变量注入 API 地址，前后端分离开发（Vite proxy）。

**证明什么**：这是「全栈」的另一半。重点不是页面多炫，而是**类型契约贯通前后端、状态管理清晰、无硬编码、可独立构建部署**。

**验收标准**：
- [ ] 前端可独立 `npm run build`，产物可被 `api` 或 Nginx 托管。
- [ ] 所有 API 调用走生成的类型化客户端，无裸 `fetch` + 手写 URL。
- [ ] 上传带进度与失败重试，视频用 Range 播放（P5 打通后端）。
- [ ] 无残留 `localhost:9000` / `9090` 硬编码。

---

### Phase 5 — 流媒体与对象存储（折中）

**目标**：把「本地文件 + 整文件 ServeContent」升级为「对象存储 + Range 流式播放 + 分片/断点上传」。

**涉及技术**：HTTP Range（`http.ServeContent` 已有，但要做对：支持 `HEAD`、`206`、`Content-Range`）、MinIO（S3 兼容，本地免费）、AWS/MinIO Go SDK、分片上传（multipart upload）、预签名 URL。

**交付物**：
- 视频文件从本地磁盘迁到 MinIO；`streamserver` 变成「签发预签名 URL / 流式代理」。
- 播放走 Range，支持拖动进度条（返回 `206 Partial Content`）。
- 上传改为分片/断点：大文件分片传，可续传，前端显示每片进度。
- 上传前校验内容类型与大小上限；文件名由服务端生成，不再信任客户端 `vid-id`（顺带彻底堵死路径穿越）。

**证明什么**：这是视频平台最有区分度的工程深度——你理解「大文件不该整体进内存」「对象存储与本地文件系统的差异」「Range 请求如何支撑拖动」。

**验收标准**：
- [ ] 视频对象存 MinIO，元数据存 MySQL，两者通过 `object_key` 关联。
- [ ] `GET /videos/:id` 支持 Range，返回 `206`，拖动播放正常。
- [ ] 上传支持分片与断点续传（可中断后重连继续）。
- [ ] 无路径穿越（`../` 请求被拒，有测试）。

---

### Phase 6 — 异步任务与并发健壮性

**目标**：把手写的 `taskrunner`（channel 状态机 + `Sleep` 测试）替换成可解释、可恢复、幂等的可靠 worker。

**涉及技术**：`context` 传播与取消、worker pool（`errgroup`）、DB outbox 模式（任务表，消费后标记，保证至少一次投递）、幂等消费、重试 + 退避、优雅停机。

**交付物**：
- 删除视频等异步动作走「outbox 表 + 消费者」：业务先写任务，worker 消费并幂等执行，崩溃重启不丢任务、不重复执行。
- `scheduler` 用 `errgroup` + `context` 管理并发，信号量限并发，去掉 `Sleep` 型测试。
- 所有服务加优雅停机（`http.Server.Shutdown` + 等待在途任务完成）。
- 队列选型决策点：先用 **DB outbox** 理解「为什么需要队列」，再评估升级到 **Redis Streams / NATS**——升级前要能回答「DB 表当队列在哪会撑不住」。不是为加而加。

**证明什么**：你理解「异步 ≠ 丢任务」「至少一次投递 + 幂等消费」「如何优雅关停一个正在消费的 worker」。这是后端从 Demo 走向可生产的分水岭。

**验收标准**：
- [ ] 任务中断后重启可恢复，不重复执行（幂等性有测试）。
- [ ] worker 并发可控，测试不依赖 `time.Sleep`。
- [ ] 三服务都能优雅停机，`SIGTERM` 下在途请求/任务完成后再退出。

---

### Phase 7 — 云原生落地（Docker + 本地 K8s）

**目标**：把「三个 `go run` 进程 + `.env`」变成「容器镜像 + compose 本地 + kind 上的 Kubernetes 部署」。

**涉及技术**：Docker 多阶段构建（`golang:1.26` 构建 → `alpine/distroless` 运行）、docker-compose（本地全套：MySQL + MinIO + 三服务 + web）、kind 或 minikube、K8s 核心对象（Deployment / Service / Ingress / ConfigMap / Secret / 探针）、Helm。

**交付物**：
- 每个 Go 服务 + 前端一个多阶段 Dockerfile，镜像尽量小、非 root 运行。
- `docker-compose.yml` 一键起整套本地环境（含 MySQL、MinIO）。
- K8s 清单（或 Helm chart）：三服务 + web + MySQL + MinIO，配置走 ConfigMap，密码走 Secret。
- 就绪/存活探针（`/healthz`、`/readyz`），Ingress 统一入口。

**证明什么**：你理解「配置与代码分离」「镜像可复现」「探针与滚动更新」「密钥不进镜像」。这是「云原生」最硬核、最能在简历上站得住的一层。

**验收标准**：
- [ ] `docker compose up` 一键起整套环境，可访问。
- [ ] 所有镜像多阶段构建，非 root、体积合理（有 `docker images` 证据）。
- [ ] kind 集群上 `kubectl get pods` 全部 `Running`，探针生效。
- [ ] 密码/密钥在 Secret 中，不出现于镜像或 ConfigMap。

---

### Phase 8 — 可观测性与交付闭环

**目标**：让系统「能被看见、能被追踪、能被自动交付」。

**涉及技术**：`log/slog` 结构化日志 + request-id 贯穿、Prometheus 客户端（RED 指标：速率/错误/延迟）、OpenTelemetry 追踪（跨服务 span）、健康/就绪端点、GitHub Actions（lint → test → build → push 镜像 → deploy 到 kind）、语义化版本 + changelog。

**交付物**：
- 统一结构化日志格式，每个请求带 `request_id`，跨服务可关联。
- 三个服务暴露 `/metrics`（Prometheus 格式），`/healthz` + `/readyz`。
- 关键路径（登录、上传、播放、删除）有 span，可看到一次请求跨 api → scheduler 的调用链。
- CI/CD 流水线：合并到 main 自动构建镜像、跑测试、部署到 kind 的 staging 命名空间。

**证明什么**：这是「生产级」的最后一块拼图——你理解可观测性的三支柱（日志/指标/追踪）各自回答什么问题，以及「自动化交付」如何把代码变更安全地推到环境里。

**验收标准**：
- [ ] 三服务 + 前端全部进入 CI/CD，合并 main 自动部署。
- [ ] `/metrics`、`/healthz`、`/readyz` 可用，Prometheus 能抓取。
- [ ] 一次跨服务请求能通过 request_id / trace_id 串联起来。
- [ ] 有语义化版本与 changelog 记录发布历史。

---

## 7. 贯穿所有阶段的工程规范

这些不是某个阶段的事，而是从 P1 起就要养成的习惯，否则后面会返工：

- **错误处理**：业务错误向上 `return`，只在 HTTP 边界转换成响应；用 `fmt.Errorf("...: %w", err)` 保留 cause，不吞错。
- **context 传播**：每个跨边界调用（DB、HTTP、MinIO）都传 `ctx`，支持取消与超时。
- **命名**：统一 `ID`（不是 `Id`）、`URL`、`HTTP` 等 Go 惯用缩写；`streamsever` 的拼写可在一次重命名中修正。
- **配置**：遵循 12-factor，配置走环境变量，K8s 里走 ConfigMap/Secret，永远不硬编码、不进镜像。
- **测试纪律**：单测确定性、无网络/无 DB 依赖；集成测试门控；每次改动 `gofmt` + `go vet` + `go test` 三件套。
- **提交**：Conventional Commits（`feat:` / `fix:` / `refactor:` / `test:` / `docs:` / `chore:`），一个提交做一件事。
- **中间件引入**：Redis / NATS / MinIO 这类组件，先写清「解决什么具体问题、不用的后果、引入的代价」三点，再决定引入；绝不为了「看起来技术多」而加。

## 8. 如何持续迭代这份路线图

1. **只动当前阶段**：任何时候只有一个阶段处于 `🔵 进行中`，做完验收、提交、再开下一个，避免多线并进。
2. **每完成一个阶段**：把第 5 节的表勾成 `✅`，写完成日期，并在这里补一句「本阶段最大的收获 / 最难的一个点」。
3. **决策先记录再动手**：任何影响架构的取舍（JWT vs 会话、React vs Vue、MinIO vs 直连 S3）先写进第 3 节，不要边做边改。
4. **细粒度步骤继续走 stage 文档**：P1 内部仍按 `progressive-refactor-guide.md` 的 stage 4–11 小步推进；P2 以后，本文件就是唯一的阶段来源。
5. **定期回看北极星**：每完成 2 个阶段，回来对照第 4 节的「能力地图」，确认积累的证据确实对应目标能力，而不是偏到炫技却无关的角落。

## 9. 一句话总结

> 先把地基打牢（P1 安全 + 分层 + 可测），再让 API 有契约（P2）、鉴权有纵深（P3）、前端有工程化（P4）、视频有真流媒体（P5）、后台有可靠异步（P6），最后用云原生（P7）和可观测 + 交付（P8）把它托成一件能拿得出手的作品。
