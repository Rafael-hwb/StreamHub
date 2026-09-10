# StreamHub 代码优化指南（只优化代码）

> 定位：**只做代码层面的优化**——修 bug、去重、统一风格、消除坏味道。
> 不做新功能、不做部署、不做安全策略改造、不大改前端 UI。
> 功能完善看 `docs/feature-guide.md`，安全/部署/可观测性升级看 `docs/upgrade-guide.md`。
>
> 基线（2026-09-09）：上传登记 / 我的视频 / 详情 / 评论 / 删除的 handler 与路由都已写完。
> 当前 `go build ./...` 通过，但 **`go vet ./...` 是红的**（1 处 struct tag 语法错误），
> 另有几处只有运行时才会爆的 SQL bug，必须先修。

## 优化顺序总览

```text
C0 修真实 bug（vet 红 + 运行时必挂的 SQL）   ← 先做，别带着 bug 优化
C1 数据模型瘦身（VideoDetail 嵌 VideoInfo）  ← 回应"类型太多太冗余"
C2 配置加载收敛 + 数据库连接合一（internal/config + internal/dbconn）
C3 错误处理统一（三服务一套 errs + 中间件）
C4 handler 样板收敛（当前用户、绑定、鉴权辅助函数）
C5 session 包质量（吞错误、裸断言、过期清理）
C6 scheduler / taskrunner 代码质量
C7 streamsever 代码质量
C8 死代码、格式化、测试收尾
```

每个 C 步骤：**改动文件 → 改法/代码 → 验收 → commit**。除 C0 外都建议独立提交。

---

## C0：先修真实 bug（会红 / 会挂的）

### C0.1 struct tag 语法错误（vet 必报）

文件：`api/defs/apidefs.go:48`

```go
type CommentCreateRequest struct {
	Content string `json:content`   // 少了引号！
}
```

改成：

```go
type CommentCreateRequest struct {
	Content string `json:"content"`
}
```

这类 tag 是"能编译但无效"的——json 解析时这个字段会退化成大写键 `Content`，
前端 `data.content` 取不到。

验收：`go vet ./api/...` 全绿。

### C0.2 重复导入

文件：`api/dbops/api.go` 顶部

```go
import (
	"database/sql"
	_ "database/sql"   // ← 删掉这一行，上面已正常导入
	...
)
```

### C0.3 评论列表 SQL 表名拼错（运行时必 500）

文件：`api/dbops/api.go` 的 `ListCommentsByVideo`

```go
SELECT comments.id, user.login_name, comments.content   // ← user 不存在
```

改成 `users.login_name`。当前 `GET /api/videos/:vid/comments` 一调就报
`Unknown table 'user'`，这个 bug 优先级最高。

### C0.4 列表 SQL 列数与 Scan 数不匹配（运行时必 500）

文件：`api/dbops/api.go` 的 `ListAllVideos`

SQL 只 SELECT 了 4 列：

```go
SELECT video_info.id, video_info.title, video_info.display_ctime, users.login_name
```

但 Scan 了 5 个变量（`id, aid, title, ctime, authorName`）→
`sql: expected 4 destination arguments in Scan, not 5`。

修复：SELECT 补回 `video_info.author_id`，与 Scan 对齐：

```go
stmtOut, err := dbConnection.Prepare(`SELECT video_info.id, video_info.author_id,
						video_info.title, video_info.display_ctime, users.login_name
					FROM video_info
					INNER JOIN users ON video_info.author_id = users.id`)
```

`GET /api/videos`（广场）目前一调就 500，就是这里。

### C0.5 我的列表漏了 AuthorId

文件：`api/dbops/api.go` 的 `ListVideosByAuthor`

```go
videoInfo := &defs.VideoInfo{
	Id:           id,
	Title:        title,
	DisplayCtime: ctime,
	// AuthorId 没填 → 返回 0
}
```

改成 `AuthorId: aid`（函数参数里有 aid，scan 也带了）。

### C0.6 删除接口用错错误码

文件：`api/handlers.go` 的 `DeleteVideoHandler`

```go
if video.AuthorId != aid {
	SendErrorResponse(context, defs.ErrorNotAuthUser)  // 401，语义错
}
```

改成 `defs.ErrorNotVideoOwner`（403，你已经在 defs/error.go 定义过 006）。

### C0.7 删除遗留空壳路由组

文件：`api/main.go`

```go
api := router.Group("/api")
api.Use(SessionMiddleware)   // 没有任何路由，纯死代码
```

删掉这 3 行，只保留 publicAPI / authAPI。

### C0.8 绑定方法统一

`AddCommentHandler` 用了 `ShouldBindBodyWithJSON`，其余 handler 都是
`ShouldBindJSON`。统一成 `ShouldBindJSON`（功能相同，少一个"为什么这个不一样"的疑问）。

### C0.9 格式统一

仓库根目录执行：

```powershell
gofmt -w api scheduler streamsever
```

重点看 `api/handlers.go` 的 import 分组（gin 被塞在标准库中间）和 defs 里的空行。

### C0 验收

1. `go vet ./...` 无输出；
2. `go build ./...` 通过；
3. 真库起服务后 curl：
   - `GET /api/videos` 返回数组（之前 500）；
   - `GET /api/videos/<vid>/comments` 返回数组（之前 500）；
   - 非作者 DELETE 返回 403；
   - 评论 content 序列化键是小写 `content`。

commit：`fix: correct sql columns, struct tags and error codes`

---

## C1：数据模型瘦身（解决"类型太多"的观感）

### 1.1 VideoDetail 嵌入 VideoInfo

现状：`VideoDetail` 和 `VideoInfo` 有 4 个一模一样的字段——这正是你觉得冗余的地方。

Go 结构体嵌入可以让输出 JSON 扁平化且不重复定义：

```go
type VideoDetail struct {
	VideoInfo
	AuthorName string `json:"author_name"`
}
```

嵌入的 `VideoInfo` 不带 json tag 时会**内联展开**，序列化结果不变：

```json
{"id":"...","author_id":1,"title":"...","display_ctime":"...","author_name":"avenssi"}
```

使用处（`GetVideoDetail`、`ListAllVideos`）改为：

```go
detail := &defs.VideoDetail{
	VideoInfo:  defs.VideoInfo{Id: id, AuthorId: aid, Title: title, DisplayCtime: ctime},
	AuthorName: authorName,
}
```

### 1.2 命名统一约定（写进注释，防再乱）

定一个表，全仓照此执行：

| 类别 | 规则 | 例子 |
|---|---|---|
| HTTP handler | 动作 + 资源，不带 Handler 后缀 | CreateVideoInfo / GetVideoInfo / DeleteVideoInfo |
| 请求体类型 | 资源 + 动作 + Request | VideoCreateRequest → 建议改 CreateVideoRequest |
| 领域模型 | 直接对应表 | VideoInfo / Comment |
| 输出模型 | 只加展示需要的字段 | VideoDetail = VideoInfo + AuthorName |
| dbops 函数 | 沿用课程 Add/Get/Delete/List | AddVideo / ListVideosByAuthor |

据此统一现有 handler 命名：

- `DeleteVideoHandler` → `DeleteVideoInfo`
- `ListVideosHandler` → `ListVideos`
- `ListCommentsHandler` → `ListComments`（dbops 里有个带时间范围参数的旧 ListComments，重名不同包，可接受；若嫌乱，旧函数改名 `ListCommentsInRange`）

`main.go` 注册处同步改名。

### C1 验收

- `go build ./...` 通过；
- `GET /api/videos/:vid` 返回字段和之前完全一样（用 curl diff 对比一次）；
- 仓库里不再有两个各写 4 字段的视频结构体。

commit：`refactor: embed VideoInfo into VideoDetail and unify naming`

---

## C2：配置加载收敛 + 数据库连接合一

### 现状问题

`api/dbops/connection.go` 和 `scheduler/dbops/connection.go` 是**两份几乎一模一样的
200 行**：同样的 .env 查找、同样的 DSN 拼接、同样的 `init()`。
另外 `sql.Open` 是惰性的——**不报密码错**，真正请求时才炸，很难排查。

还有三个坏味道：

- `init()` 里做 I/O 并且 `panic`：包级副作用，导入即连库，测试时没法替换；
- `.env` 靠 `os.Getwd()` 依次猜 `.env` / `../.env` / `api/.env`：从不同目录启动结果
  完全不同——现在 scheduler 能连上库，只是 fallback 凑巧命中了 `api/.env`；
- streamsever 完全没接这套（`VIDEO_DIR` 等仍然裸写，留给 upgrade-guide Phase 7）。

### 改法

分两步：**先把"读配置"从"连数据库"里拆出来**，再合并重复的连接代码。

#### 2.1 新建 `internal/config/config.go`：只读环境变量，不碰文件

环境变量是唯一真相来源；`.env` 只是本地开发的便利文件，生产只读真实 env——
**不引入 yaml/viper**（多一个真相来源、多一层依赖，还容易把密钥提交进仓库）。

```go
package config

import (
	"fmt"
	"os"
)

type MySQL struct {
	User, Pwd, Host, Port, DB string
}

func (m MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", m.User, m.Pwd, m.Host, m.Port, m.DB)
}

type Config struct {
	MySQL MySQL
}

// Load 只读环境变量并校验：纯函数、无副作用，测试可直接用 t.Setenv 构造
func Load() (*Config, error) {
	mysql, err := loadMySQL()
	if err != nil {
		return nil, err
	}
	return &Config{MySQL: mysql}, nil
}

// 约定：密钥必填，其余给安全的默认值
func loadMySQL() (MySQL, error) {
	pwd, err := envRequired("MYSQL_PWD")
	if err != nil {
		return MySQL{}, err
	}

	return MySQL{
		User: envOr("MYSQL_USER", "root"),
		Pwd:  pwd,
		Host: envOr("MYSQL_HOST", "localhost"),
		Port: envOr("MYSQL_PORT", "3306"),
		DB:   envOr("MYSQL_DB", "streamhub"),
	}, nil
}

func envRequired(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

**`.env` 只在 main 里加载一次**（config 包不碰文件，才能保持可测）：

```go
// main.go
if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
	log.Printf("warning: load .env: %v", err) // 文件不存在是正常的：用的是真实环境变量
}

cfg, err := config.Load()
if err != nil {
	log.Fatalf("load config: %v", err)
}
```

两点理由：

- 只忽略"文件不存在"，**格式写坏会告警**；若写成 `_ = godotenv.Load(".env")` 全吞，
  你会拿着空配置继续跑，最后只看到 `MYSQL_PWD is required`，被引到错误方向；
- 不需要 `MustLoad`：`MustXxx` 适合"包级初始化、拿不到 error"的场合，而 main 能接住
  error，`log.Fatalf` 比 `panic` 少一段堆栈、信息更干净。

> 顺带一个正确行为：`godotenv.Load` **不覆盖已存在的环境变量**，所以真实 env 的优先级
> 天然高于 `.env`，生产注入的值不会被本地文件篡改。

> 本步只收敛"加载方式"（单一入口、显式注入、不再猜路径）。**完整的多服务配置外置**
> （各服务端口、`VIDEO_DIR`、`ALLOW_ORIGIN`、`.env.example`、生产不读 .env）仍留给
> upgrade-guide Phase 7，避免这一步范围膨胀。

#### 2.2 新建 `internal/dbconn/dbconn.go`：只负责打开 + ping

不再自己找 `.env`，配置由参数传入；**出错返回 error，把"退不退出"的决定权留给 main**：

```go
package dbconn

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/Rafael-hwb/streamhub/internal/config"
)

func Open(m config.MySQL) (*sql.DB, error) {
	db, err := sql.Open("mysql", m.DSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.Ping(); err != nil { // sql.Open 是惰性的，ping 才能真正暴露密码/网络错误
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}
```

#### 2.3 两份 dbops/connection.go 改成显式注入

包内不再有副作用，也不再有 `init()`：

```go
// api/dbops/connection.go（scheduler 同构）
package dbops

import (
	"database/sql"

	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/Rafael-hwb/streamhub/internal/dbconn"
)

var dbConnection *sql.DB

// Init 必须在 main 启动时调用一次，否则 dbConnection 为 nil
func Init(cfg config.Config) error {
	db, err := dbconn.Open(cfg.MySQL)
	if err != nil {
		return err
	}
	dbConnection = db
	return nil
}
```

main 里串起来（接着 2.1 那段 `cfg`）：

```go
if err := dbops.Init(*cfg); err != nil {
	log.Fatalf("init db: %v", err)
}
```

> 若不想给两个 dbops 加 `Init`，也可以直接在 main 里 `db, err := dbconn.Open(cfg.MySQL)`
> 再 `dbops.SetDB(db)`，效果一样。**不建议**回到包级 `var db = MustOpen()` 那种写法：
> 它既 panic 又拿不到 error，还会像现在这样在 import 时就连库。

### C2 验收

- api 和 scheduler 单独启动，密码错误时**启动即失败**（`log.Fatal` 报 ping 失败），而不是请求时才 500；
- 缺 `MYSQL_PWD` 时启动即失败，错误信息点名缺哪项；
- `config.Load()` 无副作用：测试里 `t.Setenv` 就能覆盖缺密码分支，不需要真文件；
- 从任意目录启动结果一致（不再依赖 cwd 猜 `.env`）；
- 两份 connection.go 合计约 400 行 → 40 行以内，且没有重复的 `.env` 查找逻辑。

commit：`refactor: extract config and shared db connection`


---

## C3：错误处理统一（跨三服务）

### 现状问题

三套并存：

- api：`defs.ErrResponse` + `api/response.go`；
- scheduler：`(statusCode int, message string)` 直接返回裸字符串；
- streamsever：自己的 `ErrResponse/ErrStruct` + `streamsever/response.go`；

同一个"数据库错误"，三处 JSON 结构完全不同，handler 里全是
`if err != nil { SendErrorResponse(...); return }` 样板。

### 改法（代码重构，契约会变，前端同步改一次）

新建 `internal/errs` 和 `internal/httpx`，方案与之前讨论一致：

```go
// internal/errs/errs.go
package errs

type AppError struct {
	HTTPStatus int
	Code       string
	Message    string
	Cause      error
}

func (e *AppError) Error() string { return e.Message }

func BadRequest(msg string) *AppError   { return &AppError{400, "BAD_REQUEST", msg, nil} }
func Unauthorized(msg string) *AppError { return &AppError{401, "UNAUTHORIZED", msg, nil} }
func Forbidden(msg string) *AppError    { return &AppError{403, "FORBIDDEN", msg, nil} }
func NotFound(msg string) *AppError     { return &AppError{404, "NOT_FOUND", msg, nil} }
func Internal(cause error) *AppError    { return &AppError{500, "INTERNAL", "Internal server error", cause} }
```

```go
// internal/httpx/error.go
package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err

		var appErr *errs.AppError
		if errors.As(err, &appErr) {
			c.AbortWithStatusJSON(appErr.HTTPStatus, gin.H{
				"code": appErr.Code, "message": appErr.Message, "data": nil,
			})
			if appErr.Cause != nil {
				slog.Error("request failed", "path", c.Request.URL.Path, "cause", appErr.Cause)
			}
			return
		}

		slog.Error("unhandled error", "path", c.Request.URL.Path, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code": "INTERNAL", "message": "Internal server error", "data": nil,
		})
	}
}
```

接入步骤：

1. 三个 main 都改成 `gin.New()` + `gin.Recovery()` + `httpx.ErrorHandler()`；
2. handler 不再调 SendErrorResponse，改成 `c.Error(errs.Xxx(...))` + `return`；
3. 删除 `api/response.go`、`scheduler/response.go`、`streamsever/response.go`，
   删除 defs/streamsever defs 里的 ErrResponse/ErrStruct/ErrorXxx 变量；
4. `c.JSON(200, data)` 统一走 `httpx.WriteOK(c, data)`（可后续做，先保持直接 JSON）；
5. 前端 `data.error` 解析改成 `data.message`，401/403/404 分支按 `code` 判断。

> 这一步改动面最大，建议单独一个 commit，并在改完前后各跑一遍功能联调，
> 用 curl 对比每个错误分支的状态码是否一致。

### C3 验收

- 参数错/未登录/非作者/不存在/DB 错误，三服务返回结构都是 `{code, message, data}`；
- 未知 panic 返回 500 且真实错误进日志；
- 前端页面不再解析旧的 `{"error":"..."}`。

commit：`refactor: unify error response across services`

---

## C4：handler 样板收敛

### 4.1 参数名统一

现在所有 handler 参数叫 `context`。将来要用标准库 `context` 包（超时、优雅停机）
时必然冲突。统一改成 `c`（gin 官方习惯），机械替换即可。

### 4.2 当前用户辅助函数

"GetHeader 拿用户名 → GetUserIDByName"在 5 个 handler 里重复。抽一个：

```go
// handlers.go 底部
func currentUserID(c *gin.Context) (int, bool) {
	username := c.GetHeader(HEADER_FIELD_USERNAME)
	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		c.Error(errs.Internal(err))   // C3 之后；之前先 SendErrorResponse
		return 0, false
	}
	return aid, true
}
```

CreateVideoInfo 变成：

```go
func CreateVideoInfo(c *gin.Context) {
	aid, ok := currentUserID(c)
	if !ok {
		return
	}

	var req defs.CreateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
		c.Error(errs.BadRequest("title is required"))
		return
	}

	video, err := dbops.AddVideo(aid, req.Title)
	if err != nil {
		c.Error(errs.Internal(err))
		return
	}
	c.JSON(http.StatusCreated, video)
}
```

### 4.3 消除死代码

逐个确认去留，留的要有调用方，不留的删：

| 代码 | 现状 | 建议 |
|---|---|---|
| api/auth.go `ValidateUser` | 无调用方 | 删除（ValidateUserSession 已覆盖） |
| api/dbops `RetrieveSession` | 无调用方 + SQL 列名 `user_name` 与表不符 | 删除，或改成 login_name 并接上调用方 |
| session `LoadSessionsFromDB` | 无调用方 | 二选一：main 启动时调用，或删除 |
| session `DeleteSession` | 有调用（IsSessionValid）但吞错 | 返回 error / 记录日志 |
| taskrunner `CreateNewWorker/StartWorker` | 无调用方 | C6 里修好并让 Start() 使用，否则删除 |
| streamsever `TestPageHandler` | 有路由，但依赖 ./videos/upload.html | 保留，若不再用则删路由 |

### C4 验收

- `rg "ValidateUser\\(|RetrieveSession\\(|LoadSessionsFromDB\\("` 只剩有调用方的；
- handler 内重复的"查当前用户"代码只剩一处定义。

commit：`refactor: add handler helpers and remove dead code`

---

## C5：session 包质量

### 5.1 GenerateSessionId 吞了两个错误

```go
func GenerateSessionId(username string) string {
	sid, _ := utils.NewUUID()          // ← uuid 失败被吞
	...
	dbops.InsertSession(sid, TTL, username)  // ← DB 失败被吞
	return sid
}
```

改成返回 error，调用方（CreateUser/Login）失败时返回 500，别把无效 session 发给用户：

```go
func GenerateSessionId(username string) (string, error) {
	sid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}
	...
	if err := dbops.InsertSession(sid, TTL, username); err != nil {
		return "", err
	}
	return sid, nil
}
```

### 5.2 裸类型断言（可能 panic）

```go
perSimpleSession.(*defs.SimpleSession).TTL   // 类型不对直接 panic
```

统一逗号断言：

```go
s, ok := perSimpleSession.(*defs.SimpleSession)
if !ok {
	return "", false
}
```

### 5.3 过期清理补全

`IsSessionValid` 现在：查不到 → 删；**过期 → 只返回 false，不删**。补上：

```go
if s != nil && nowTime >= s.TTL {
	DeleteSession(sid)   // 内存 + DB 一起清
	return "", false
}
```

### C5 验收

- 注册/登录仍正常返回 session_id；
- 临时把 TTL 改小（比如 +1 秒），过期后再请求 /api/me → 401，且 sessions 表记录被删；
- 永不出现 `panic: interface conversion`。

commit：`fix: session error handling, safe asserts and expiry cleanup`

---

## C6：scheduler / taskrunner 代码质量

### 6.1 DeleteVideo 语义错误

```go
func DeleteVideo(vid string) error {
	err := os.Remove(VIDEO_PATH + vid)
	if err != nil && os.IsNotExist(err) {
		log.Printf("Deleting video error: %v", err)  // 文件不存在打错误日志，还返回 nil
	}
	return nil                                        // 永远 nil → 上层错误处理形同虚设
}
```

改成：

```go
func DeleteVideo(vid string) error {
	err := os.Remove(VIDEO_PATH + vid)
	if os.IsNotExist(err) {
		return nil // 文件本来就不在，视为删除成功
	}
	return err
}
```

### 6.2 executor 的安全断言

```go
errMap.Range(func(k, v interface{}) bool {
	if err := v.(error); err != nil && firstErr == nil {   // 裸断言
```

改成：

```go
errMap.Range(func(k, v interface{}) bool {
	if e, ok := v.(error); ok && e != nil && firstErr == nil {
		firstErr = e
	}
	return true
})
```

### 6.3 Runner 的两个 if 改成 else-if

```go
if c == READY_TO_DISPATCH { ... }
if c == READY_TO_EXECUTE { ... }   // 同一时刻只可能是其中一个 → else if
```

### 6.4 Worker：单位 bug + 复用 Runner bug + 接进 Start

```go
func CreateNewWorker(interval time.Duration, runner Runner) *Worker {
	return &Worker{
		ticker: *time.NewTicker(interval),   // interval 已是 Duration，别再 * time.Second
		runner: runner,
	}
}

func (w *Worker) StartWorker() {
	for {
		select {
		case <-w.ticker.C:
			// 每轮新建 Runner：上一轮 CLOSE 后通道已关闭，复用会 panic
			go CreateNewRunner(w.runner.dataSize, w.runner.longLived,
				w.runner.Dispatcher, w.runner.Executor).StartAll()
		}
	}
}

func Start() {
	template := *CreateNewRunner(3, false, VideoClearDispatcher, VideoClearExecutor)
	w := CreateNewWorker(2*time.Second, template)
	go w.StartWorker()
}
```

### 6.5 去掉 sleep 型测试

`runner_test.go` 现在是 `time.Sleep(3 * time.Second)` 猜结束时间。改成显式同步：

```go
func TestRunner(t *testing.T) {
	var count atomic.Int32
	done := make(chan struct{})

	d := func(dc DataChannel) error {
		for i := 0; i < 30; i++ {
			dc <- i
		}
		return nil
	}
	e := func(dc DataChannel) error {
		for {
			select {
			case <-dc:
				count.Add(1)
			default:
				close(done)
				return errors.New("stop")   // 触发 CLOSE，runner 正常退出
			}
		}
	}

	runner := CreateNewRunner(30, false, d, e)
	go runner.StartAll()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if count.Load() != 30 {
		t.Fatalf("executed %d, want 30", count.Load())
	}
}
```

### C6 验收

- `go test ./scheduler/...` 秒级跑完、无 Sleep；
- scheduler 跑 10 分钟无 `send on closed channel`；
- 手动插删除记录 → 文件存在时被删、不存在时静默成功、DB 记录被清。

commit：`fix: taskrunner semantics, worker interval and deterministic test`

---

## C7：streamsever 代码质量

### 7.1 限流器换成 select-default

现在 `len(bucket) >= max` 的判断 + 发送不原子，高并发下会退化成"阻塞"而不是"拒绝"，
而且每次拒绝都打日志会刷屏：

```go
func (limiter *ConnectionLimiter) GetConnection() bool {
	select {
	case limiter.bucket <- 1:
		return true
	default:
		log.Printf("Reach the rate limitation.")
		return false
	}
}
```

### 7.2 defer Close 紧跟 open

```go
video, err := os.Open(videoLink)
if err != nil { ... }
defer video.Close()          // 现在就 defer，别等 ServeContent 之后
http.ServeContent(...)
```

### 7.3 去掉双重解析

`UploadHandler` 先手动 `ParseMultipartForm(MAX_UPLOAD_SIZE)` 又调 `c.FormFile`，
后者内部会再走一遍解析。二选一，保留 `c.FormFile` 并单独校验大小即可
（或保留 ParseMultipartForm 后用 `c.Request.MultipartForm.File["file"]` 取值）。

### C7 验收

- 第 11 个并发请求立即 429，而不是挂着等；
- 上传/播放/限流行为与之前一致。

commit：`refactor: streamsever limiter and file handling`

---

## C8：格式化、静态检查与测试收尾

每完成一阶段后执行：

```powershell
gofmt -w api scheduler streamsever
go vet ./...
go build ./...
go test ./...
```

目标状态：

- `go vet` 零告警；
- 测试无 Sleep、无依赖真实执行顺序；
- `api/dbops/api_test.go` 里共享的 `tempvid` 全局变量改成测试内局部变量 +
  `t.Run` 顺序依赖改成子测试内自给自足（或在测试内创建数据再删除）；
- 不再有 `//I think lost the error` 这类"知道有问题但没处理"的注释——要么处理要么删行。

---

## 明确不做的（避免范围膨胀）

- 安全：路径穿越校验、密码 bcrypt、CORS 收紧 → upgrade-guide Phase 1；
- 配置外置 / 端口从环境读 → upgrade-guide Phase 7（本指南 C2 只做加载方式收敛与连接合一）；
- 前端 UI/页面重构 → 功能稳定后另议；
- 视频 ID 校验、session 持久化恢复 → upgrade-guide Phase 2。

## 每阶段通用动作

1. `gofmt -w` 对应目录；
2. `go vet ./...`、`go build ./...`、`go test ./...`；
3. 按该阶段验收点过一遍；
4. `git add -A && git commit`，message 用文档里给的标题。

遇到"改这个会不会影响另一个接口"的疑问，先停手问，别在没对齐的情况下硬改。
