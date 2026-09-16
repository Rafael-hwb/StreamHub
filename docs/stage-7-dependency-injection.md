# 阶段 7：显式依赖 —— 移除全局数据库连接（DI + repository 层）

> 改造范围：`api/dbops`、`scheduler/dbops`、`api/handlers.go`、`api/main.go`、`scheduler/main.go`、`scheduler/handlers.go`、`api/main_test.go`。
> 前置：阶段 6（路径安全）已提交。
>
> **这是 P1 最大的一步**。它会牵动所有 handler，务必按下面的「渐进顺序」小步走，每一步都保持能编译。

## 1. 学习目标

理解「包级全局变量为什么让代码难测试、难复用」，以及「构造时显式传入依赖」如何改变这一切。做完后，`dbops` 从「一堆包级函数 + 隐藏的全局连接」变成「一个持有 `*sql.DB` 的 repository，通过构造函数注入」。

## 2. 现状与问题

| # | 位置 | 问题 |
| --- | --- | --- |
| 1 | [api/dbops/connection.go:10](api/dbops/connection.go:10) | `var dbConnection *sql.DB` 包级全局，所有函数隐式依赖它 |
| 2 | [scheduler/dbops/connection.go:9](scheduler/dbops/connection.go:9) | 同样的问题 |
| 3 | [api/handlers.go](api/handlers.go) | handler 是包级函数，无法接收依赖 |

全局连接带来两个直接后果：

1. **测试难**：`dbConnection` 要么是 nil（测试 panic），要么指向真实库（测试慢、有副作用）。你没法在测试里塞一个假实现。
2. **初始化难**：`dbops.Init(cfg)` 是隐式的副作用，谁调用、调用几次、顺序如何，都藏在代码里。多实例、多环境都别扭。

## 3. 目标形态

```
handler (struct 方法，持有依赖)
   │
   ▼
repository (Store，持有 *sql.DB，方法做数据访问)
   │
   ▼
*sql.DB（由 dbconn.Open 创建，只创建一次，显式传入）
```

`main` 里变成清晰的装配线：

```go
db, err := dbconn.Open(cfg.MySQL)   // 1. 建连接
store := dbops.NewStore(db)          // 2. 包成 repository
h := NewHandler(store)               // 3. 注入 handler
router := RegisterHandlers(h)        // 4. 注册路由
```

## 4. 分步骤改法

### 步骤 1：先拿最小的 `scheduler/dbops` 练手

它只有 3 个函数，是理解「全局函数 → 方法」的最小样本。

新建 `scheduler/dbops/store.go`：

```go
package dbops

import "database/sql"

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}
```

把 `api.go` 和 `internal.go` 里的三个函数改成方法，`dbConnection` 换成 `s.db`：

```go
func (s *Store) AddVideoDeletionRecord(vid string) error {
	stmtIn, err := s.db.Prepare("INSERT INTO video_del_rec (video_id) VALUES (?)")
	// ...
}

func (s *Store) ReadVideoDeletionRecord(count int) ([]string, error) { /* ... */ }
func (s *Store) DeleteVideoDeletionRecord(vid string) error { /* ... */ }
```

删除 `scheduler/dbops/connection.go` 里的全局 `dbConnection` 和 `Init`。

改 `scheduler/main.go`：

```go
db, err := dbconn.Open(cfg.MySQL)
if err != nil {
	log.Fatalf("open db: %v", err)
}
store := dbops.NewStore(db)

// taskrunner 要用 store，见步骤 4 的说明
go taskrunner.Start(store)
```

先让 `scheduler` 编译通过：`go build ./scheduler`。此时 `taskrunner` 还在用旧的包级函数，会报错——这是预期的，先标记，到步骤 4 一并改。

### 步骤 2：重构 `api/dbops` 为 `Store`

同样的模式，`api/dbops/connection.go` 变成 `store.go`：

```go
package dbops

import "database/sql"

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}
```

`api.go`、`internal.go` 里所有函数改成 `(s *Store)` 方法，`dbConnection` 换成 `s.db`。这一步是纯机械替换：函数签名加接收者，函数体里的 `dbConnection` 换成 `s.db`，删掉 `Init`。

涉及函数：`AddCredential`、`GetCredential`、`DeleteCredential`（若阶段 4 没删）、`AddVideo`、`GetVideo`、`DeleteVideo`、`AddComment`、`ListComments`、`ListCommentsByVideo`、`GetUserIDByName`、`ListVideosByAuthor`、`GetVideoDetail`、`ListAllVideos`、`InsertSession`、`RetrieveSession`、`RetrieveAllSessions`、`DeleteSession`。

### 步骤 3：handler 变成 struct 方法

`api/handlers.go` 顶部：

```go
type Handler struct {
	store *dbops.Store
}

func NewHandler(store *dbops.Store) *Handler {
	return &Handler{store: store}
}
```

每个 `func Xxx(context *gin.Context)` 改成 `func (h *Handler) Xxx(context *gin.Context)`，函数体内的 `dbops.AddCredential(...)` 改成 `h.store.AddCredential(...)`，依此类推。

`api/main.go` 的 `RegisterHandlers` 改成接收 handler：

```go
func RegisterHandlers(h *Handler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), httpx.ErrorHandler())

	router.POST("/user", h.CreateUser)
	router.POST("/user/login", h.Login)
	// ... 其余路由把 CreateVideoInfo 等换成 h.Xxx
}
```

`SessionMiddleware` 和 `auth.go` 里的 `ValidateUserSession` 暂时不动（它们依赖 `session` 包，不是 `dbops` 全局，等步骤 5 处理）。

### 步骤 4：让 `scheduler` 的 taskrunner 也拿到 store

`taskrunner` 的 `VideoClearDispatcher` / `VideoClearExecutor` 目前调用包级 `dbops`。把它们改成接收 `*dbops.Store`：

```go
func VideoClearDispatcher(store *dbops.Store, dc DataChannel) error {
	ids, err := store.ReadVideoDeletionRecord(3)
	// ...
}

func VideoClearExecutor(store *dbops.Store, dc DataChannel) error {
	// ... 内部用 store.DeleteVideoDeletionRecord
}
```

`Start` 函数加参数：`func Start(store *dbops.Store)`，`main.go` 里 `go taskrunner.Start(store)`。

### 步骤 5：处理 `session` 包

`api/session` 包也依赖 `dbops` 的全局函数（`InsertSession`、`RetrieveAllSessions`、`DeleteSession`）。有两个选择：

- **方案 A（更干净）**：`session` 包也改成持有一个 `*dbops.Store`。但 `session` 用了包级 `sessionMap` 和 `init()`，要一并处理。
- **方案 B（更小步）**：把 session 的持久化函数也并入 `Store`（它们本来就在 `dbops/internal.go`），`session` 包只保留纯内存逻辑，持久化由 handler 层或一个 `SessionStore` 完成。

本阶段推荐 **方案 B 的最小变体**：`session` 继续持有内存 `sessionMap`，但 `LoadSessionsFromDB` / `GenerateSessionId` / `DeleteSession` 改为接收 `*dbops.Store` 作为参数传入，不再在包内隐式调用全局。这样 `session` 包成为「纯逻辑 + 显式传入持久化依赖」。

具体：

```go
func LoadSessionsFromDB(store *dbops.Store) error {
	r, err := store.RetrieveAllSessions()
	// ...
}

func GenerateSessionId(store *dbops.Store, username string) (string, error) {
	// ... 内部 store.InsertSession(sid, ttl, username)
}

func DeleteSession(store *dbops.Store, sid string) {
	sessionMap.Delete(sid)
	_ = store.DeleteSession(sid) // 或返回 error 由调用方处理
}
```

`ValidateUserSession`（auth.go）调用 `IsSessionValid` 不涉及持久化，不用改；但 `DeleteSession` 被 `IsSessionValid` 调用（过期时），所以 `IsSessionValid` 也要接收 `store` 或在 handler 层处理过期删除——这里需要你做一个小的设计取舍，把依赖链理清即可。

### 步骤 6：修测试

`api/main_test.go` 里 `router := RegisterHandlers()` 现在需要传 `*Handler`。测试里有两种做法：

1. 用真实 DB 的 store（集成测试风格，需要门控）。
2. 用 `sqlmock` 或一个不碰 DB 的输入校验测试——因为 `TestCredentialEndpointsRejectInvalidInput` 只测「校验在 DB 之前」，理论上不需要真 DB。

最小改法：让 `NewHandler` 接受一个可空的 store，或者测试里构造 `dbops.NewStore(nil)`，只要被测路径（输入校验）在访问 `h.store` 之前就返回，就不会 panic。但要小心：这掩盖了「依赖没注入」的问题，不是长久之计。

更诚实的做法：给 handler 引入一个窄接口，测试注入 fake。本阶段先记录方向，最小改动是让输入校验测试通过；真正用 mock 取代 DB 属于后续「测试补齐」的收尾工作，放到阶段 11 一并处理。

## 5. 关于 service 层的边界

这一步完成后你有 `repository`（Store）和 `handler` 两层。**不要现在就抽 service 层**——当前业务逻辑太薄，硬抽会得到空壳接口。真正的 service 层在 P2（API 设计进阶）业务规则变复杂时自然浮现。P1 只需要「依赖可注入、可替换、可测试」这个结果。

## 6. 验收标准

- [ ] `rg "var dbConnection"` 无结果。
- [ ] `rg "dbops.Init"` 无结果（`Init` 已被 `NewStore` + `dbconn.Open` 取代）。
- [ ] handler 通过构造函数接收 `*dbops.Store`，测试里可注入。
- [ ] `scheduler` 和 `api` 两个服务都迁移完成，`session` 包不再隐式依赖全局连接。
- [ ] 单测不依赖真实 MySQL（输入校验类测试能脱离 DB 运行）。
- [ ] 三件套通过，`go test ./...` 里非 DB 测试依然绿。

## 7. 提交建议

拆成多个可运行提交，不要一个巨型提交：

```text
1. refactor: turn scheduler/dbops into an injected Store
2. refactor: turn api/dbops into an injected Store
3. refactor: inject Store into api and scheduler handlers
4. refactor: pass Store into session and taskrunner
```

每步之间跑 `go build ./...` 保持可编译。

## 8. 拓展思考题

1. `Init(cfg)` 和 `NewStore(db)` 在语义上差在哪？为什么「建连接」和「组装依赖」应该分开？
2. 测试里 `dbops.NewStore(nil)` 能让输入校验测试通过，但这为什么不是一个好信号？它掩盖了什么？
3. 接口（`interface`）和具体类型（`*Store`）注入各有什么取舍？什么时候该用接口？
4. 为什么 `session` 包「在包内隐式调用全局 DB」比 handler 直接调用更隐蔽、更难改？这次重构暴露了什么耦合？
