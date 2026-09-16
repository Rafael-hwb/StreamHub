# 阶段 11：工程收尾 —— README、lint、CI、健康检查与优雅停机

> 改造范围：`README.md`、`.golangci.yml`、`.github/workflows/ci.yml`、三个服务的 `main.go`（健康检查 + 优雅停机）。
> 前置：阶段 10（数据模型 + migrate）已提交。这是 P1 的最后一个 stage。

## 1. 学习目标

理解「可交付」——别人能看懂、能一键检查、能自动验证、能安全关停。一个项目「能跑」和「能交付」之间，差的就是这些工程化细节。

## 2. 现状与问题

| # | 问题 | 位置 |
| --- | --- | --- |
| 1 | README 只有 `# StreamHub` 一行 | [README.md](README.md) |
| 2 | 无静态检查配置，全靠手动 `go vet` | — |
| 3 | 无 CI，代码质量靠自觉 | — |
| 4 | 三服务直接 `router.Run(...)`，无优雅停机 | [api/main.go:68](api/main.go:68) |
| 5 | 无健康检查端点 | — |

## 3. 分步骤改法

### 步骤 1：补 README

至少包含：

- 项目简介（一句话 + 三服务职责表）。
- 架构图（可以直接引用 [overall-roadmap.md](overall-roadmap.md) 第 2 节的图）。
- 环境变量说明（`MYSQL_*`、`SCHEDULER_URL`，从 `.env.example` 举例，不写真实密码）。
- 本地启动方式（`dev.sh` 或 `docker compose`，此时还没容器化，先用 `dev.sh`）。
- 端口（8080 / 9000 / 9001）。
- 测试命令（`go test ./...`，以及集成测试的门控说明）。
- 数据库迁移命令（`migrate up`）。

### 步骤 2：golangci-lint

```powershell
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

新建 `.golangci.yml`：

```yaml
run:
  timeout: 3m

linters:
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gofmt
    - misspell
```

跑 `golangci-lint run`，把告警清零。注意：`errcheck` 会揪出你之前「吞错误」的地方（比如阶段 5 之前 `dbops.DeleteSession(sid)` 的返回被忽略），这正是它有价值的地方。

### 步骤 3：健康检查 + 优雅停机（三服务都做）

以 `api` 为例，`main.go` 改成：

```go
func main() {
	// ... 加载配置、初始化 db / store / handler ...

	router := RegisterHandlers(h)

	// 健康检查
	router.GET("/healthz", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/readyz", func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Status(http.StatusOK)
	})

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
```

要点：

- `/healthz` 只回答「进程活着」，`/readyz` 回答「能接流量」（这里检查 DB 连接）。
- `srv.Shutdown` 会等所有在途请求处理完再退出，不会中途掐断。
- `scheduler` 的 `main.go` 也要同样处理，并把 `ctx` 传给 `taskrunner.Start`（阶段 9 已经留好了 `ctx` 参数）。
- `streamsever` 同理。

### 步骤 4：GitHub Actions

新建 `.github/workflows/ci.yml`：

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: gofmt check
        run: test -z "$(gofmt -l .)"
      - name: vet
        run: go vet ./...
      - name: lint
        uses: golangci/golangci-lint-action@v6
      - name: test
        run: go test ./...
```

> `go test ./...` 里的集成测试（`api/dbops`）在 CI 无 MySQL 时会跳过（阶段 0 已经约定好门控）。将来 P7/P8 会加上 MySQL 服务容器，让集成测试真正在 CI 跑起来。

## 4. 验收标准

- [ ] README 完整，陌生人能照文档把项目跑起来。
- [ ] `golangci-lint run` 无告警。
- [ ] 三服务都有 `/healthz` + `/readyz`，`/readyz` 检查 DB。
- [ ] 三服务都能优雅停机：`Ctrl+C` 后在途请求/任务完成后再退出。
- [ ] GitHub Actions `gofmt` + `vet` + `lint` + `test` 绿灯。
- [ ] 三件套通过。

## 5. 提交建议

```powershell
git add README.md .golangci.yml .github/workflows/ci.yml api/main.go scheduler/main.go streamsever/main.go
git commit -m "chore: add readme, lint, ci, health checks and graceful shutdown"
```

## 6. 拓展思考题

1. `/healthz` 和 `/readyz` 的区别是什么？K8s 里 `livenessProbe` 和 `readinessProbe` 分别对应哪个？（提前埋个伏笔给 P7）
2. `srv.Shutdown(ctx)` 和 `srv.Close()` 的区别是什么？为什么优雅停机要用 `Shutdown`？
3. `signal.NotifyContext` 收到第二次 `SIGINT` 会发生什么？为什么很多程序会「第二次信号强制退出」？
4. 为什么集成测试在 CI 里「跳过」是可以接受的，但「永远不在 CI 跑」是隐患？P7/P8 会怎么解决？

---

## 完成 P1 后

回到 [phase-1-engineering-foundation.md](phase-1-engineering-foundation.md) 第 12 节，逐项勾选最终验收清单，然后把 [overall-roadmap.md](overall-roadmap.md) 第 5 节状态表的 P1 行改成 `✅ 完成`，写完成日期和「本阶段最大的收获 / 最难的一个点」。之后进入 P2（API 设计进阶）。
