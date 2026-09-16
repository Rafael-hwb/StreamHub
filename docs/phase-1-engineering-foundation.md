# Phase 1 — 工程地基：可信、安全、可测的 Go 后端

> 这是 [overall-roadmap.md](overall-roadmap.md) 里 P1 的总览与索引。P1 是后面一切（API 设计、鉴权、流媒体、云原生）的地基——没有它，那些都建在沙子上。
>
> 目标一句话：**把现有三服务从「能跑」升级到「可信」——正确、安全、分层、可测。**
>
> 进度承接：stage 0–3 已完成（基线 → 统一错误 → 输入校验 → 统一 HTTP 契约）。本文档从 stage 4 继续，到 stage 11 结束，即 P1 完成。
>
> 每个 stage 的**详细执行步骤**在独立文档里，本文件只保留骨架与验收，按下面表格逐个打开。

## 0. P1 完成后的最终状态（验收全景）

- 三层架构：`handler`（HTTP 解析）→ `service`（业务规则）→ `repository`（数据访问），依赖显式注入，没有包级全局数据库连接。
- 安全与正确性清空：密码哈希、路径穿越、session 生命周期、评论字段错位、资源泄漏全部修复。
- 数据库由 `golang-migrate` 管理，`schema.sql` 退役为迁移文件。
- 确定性测试：单测无 DB/无网络依赖，集成测试显式门控。
- `golangci-lint` + GitHub Actions 流水线绿灯。
- 健康检查 + 优雅停机。

## 1. 执行顺序与依赖

```
stage 4 密码哈希 ─────┐
stage 5 会话生命周期 ──┤（都只改 api，先修安全/正确性，彼此独立）
stage 6 路径安全 ─────┘
        │
        ▼
stage 7 显式依赖（DI）──── 最大的一步，重构 dbops → repository
        │
        ▼
stage 8 跨服务调用 ──── 依赖 stage 7 的注入，封装 scheduler client
stage 9 调度器并发 ──── 依赖 stage 7 的 scheduler/dbops 已重构
        │
        ▼
stage 10 数据模型 + migrate
        │
        ▼
stage 11 工程收尾
```

规则：**一次只做一个 stage**，做完三件套验收（`gofmt` / `go build ./...` / `go test ./...`）再提交，再开下一个。

## 2. 贯穿 P1 的纪律

- 每个 stage 完成后提交一次，提交信息用 Conventional Commits（`feat:` / `fix:` / `refactor:` / `test:` / `chore:`）。
- 业务错误向上 `return`，只在 HTTP 边界转响应；用 `%w` 保留 cause，不吞错。
- 每个跨边界调用（DB、HTTP）都传 `context.Context`，支持取消与超时（从 stage 7 起强制）。
- 改了代码 ≠ 改了数据：涉及 schema 的改动要同步迁移。

## 3. Stage 索引（逐个打开）

| Stage | 主题 | 学习目标一句话 | 详细文档 |
| --- | --- | --- | --- |
| 4 | 密码安全 | 哈希 vs 加密，为什么密码要「单向哈希 + 盐」 | [stage-4-password-hashing.md](stage-4-password-hashing.md) |
| 5 | 会话生命周期 | 会话会创建/读取/过期/持久化/清理，每个环节都要有人负责 | [stage-5-session-lifecycle.md](stage-5-session-lifecycle.md) |
| 6 | 输入与路径安全 | 永不信任外部输入，阻断 `../` 路径穿越 | [stage-6-path-security.md](stage-6-path-security.md) |
| 7 | 显式依赖 | 全局变量为何难测试，构造时显式传入依赖 | [stage-7-dependency-injection.md](stage-7-dependency-injection.md) |
| 8 | 跨服务调用 | 网络会失败/超时/非 200，必须显式处理 | [stage-8-scheduler-client.md](stage-8-scheduler-client.md) |
| 9 | 调度器并发 | channel 状态机、错误传播、确定性测试 | [stage-9-scheduler-concurrency.md](stage-9-scheduler-concurrency.md) |
| 10 | 数据模型 | schema 也是代码，外键/索引/正确的时间类型 | [stage-10-data-model-migrations.md](stage-10-data-model-migrations.md) |
| 11 | 工程收尾 | 可交付：README、lint、CI、健康检查、优雅停机 | [stage-11-engineering-polish.md](stage-11-engineering-polish.md) |

### 各 stage 的「现状问题」速览（定位用，细节看独立文档）

**Stage 5 — 会话生命周期**：`RetrieveSession` 查错列（`user_name` vs `login_name`）；`LoadSessionsFromDB` 从未被调用；`IsSessionValid` 逻辑反了（过期不删、不存在反而删）；`TTL VARCHAR(8)` 装不下 13 位毫秒；`GenerateSessionId` 吞两个错误。

**Stage 6 — 路径安全**：三处 `VIDEO_DIR + vid` 直接拼接（stream/upload/delete），`../` 可读、写、删任意文件。

**Stage 7 — 显式依赖**：`var dbConnection *sql.DB` 包级全局（api 与 scheduler 各一份），handler 无法注入依赖。

**Stage 8 — 跨服务调用**：`DeleteVideoHandler` 里 `http.Get` 无超时、硬编码 `localhost:9001`、不查状态码、失败仍返回成功。

**Stage 9 — 调度器并发**：`Start()` 从未真正触发过 worker；executor 的 goroutine 漏 `wg.Done()` 导致 `Wait()` 永久阻塞；`DeleteVideo` 吞错误；测试靠 `Sleep(3s)`。

**Stage 10 — 数据模型**：`schema.sql` 无版本管理；`display_ctime` 用字符串存时间；无外键索引；`ListAllVideos` 漏 `rows.Close()`；`ListComments` 死代码且字段错位。

**Stage 11 — 工程收尾**：README 只有一行；无 lint/CI；三服务直接 `router.Run` 无优雅停机；无健康检查。

## 4. P1 最终验收清单

完成 stage 4–11 后，回到 [overall-roadmap.md](overall-roadmap.md) 第 5 节的表，把 P1 勾成 `✅`。逐项确认：

- [ ] 密码哈希（stage 4）
- [ ] 会话生命周期（stage 5）
- [ ] 路径安全（stage 6）
- [ ] 显式依赖 / repository 层（stage 7）
- [ ] 跨服务调用可靠（stage 8）
- [ ] 调度器并发模型（stage 9）
- [ ] 数据模型 + migrate（stage 10）
- [ ] 工程收尾（stage 11）
- [ ] `golangci-lint run` 通过，GitHub Actions 绿灯
- [ ] 每个 handler 依赖构造注入，无包级全局连接
- [ ] 单测确定性、集成测试门控
- [ ] 三服务优雅停机 + 健康检查

**P1 完成意味着**：地基牢靠。之后的 P2（API 设计）开始，你是在一个「可注入、可测试、有契约、有安全底线」的代码库上做增量，而不是一边修洞一边盖楼。
