# StreamHub 优化升级路线图（分步文档）

> 适用现状（2026-09 基线）：
> - api(8080)/scheduler(9001)/streamsever(9000) 编译通过，schema.sql 建好，库表齐全；
> - 静态前端 static/index.html + userhome.html 由 api 托管，登录态存 localStorage；
> - 遗留：web/ 目录未删、task.go executor 仍是 goroutine 并发版、错误处理三套风格、
>   密码明文、路径未校验、上传未登记 video_info、无定时 Worker 等。

每个阶段都按同一格式写：**目的 → 前置 → 动手清单 → 验收**。
建议**每完成一个阶段就 commit 一次**，出问题容易回退。

---

## Phase 0：收尾与工程规范（低风险，先做）

### 目的
把上阶段遗留的"能编译但有隐患"的代码清干净，给后面所有改动一个干净基线。

### 0.1 删除 web/ 目录

```powershell
git rm -r web
```

理由：空文件让 `go build ./...` 报错，且 web 与 api 都绑 8080。历史已提交过，可随时找回。

### 0.2 task.go 改成顺序执行版

文件：`scheduler/taskrunner/task.go`

- 删掉包级 `var err error`；
- `VideoClearExecuter` 去掉 goroutine，逐个删除，错误存 `errMap`，全部处理完再遍历；
- `DeleteVideo` 语义修正：文件不存在返回 nil（不打印误导日志），其他错误才返回：

```go
func DeleteVideo(vid string) error {
	err := os.Remove(VEDIO_PATH + vid)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
```

### 0.3 命名与格式清理

一次性整理（纯机械改动，编译器会兜底）：

- 文件改名：`api/resonse.go` → `api/response.go`；
- 常量改名：`VEDIO_PATH` → `VIDEO_PATH`（taskrunner/defs.go 与引用处一起改）；
- 函数改名：`VideoClearExecuter` → `VideoClearExecutor`；
- 目录改名遗留检查：代码里不应再有 `templetes`/`templates` 字样，统一 `static`；
- 全仓格式化：`gofmt -w api scheduler streamsever`；
- `.gitignore` 追加 `bin/`，删除根目录三个散落的 `.exe`。

### 0.4 一键启动脚本

新建 `dev.sh`（仓库根目录）：

```bash
#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
mkdir -p videos bin
go build -o bin/api ./api
go build -o bin/scheduler ./scheduler
go build -o bin/streamsever ./streamsever
./bin/api &
P1=$!
./bin/scheduler &
P2=$!
./bin/streamsever &
P3=$!
trap 'kill $P1 $P2 $P3 2>/dev/null' INT TERM
wait
```

Windows 原生可再加一个 `dev.ps1` 等价版（Start-Process 新窗口）。

### 验收

- 根目录执行 `bash dev.sh` 三服务同时起，Ctrl+C 全部退出；
- `go build ./...` 全绿（web 已删）；
- `go vet ./...` 无输出。

---

## Phase 1：安全基线（写业务前先堵洞）

### 目的
这些是教学项目最常见的真实漏洞，越早修成本越低。

### 1.1 路径穿越（最高优先级）

现状：`StreamHandler`/`UploadHandler` 直接用 URL 里的 `:vid-id` 拼文件路径：

```go
os.Open(VIDEO_DIR + vid)
```

请求 `/videos/..%2f..%2f.env` 可能读到任意文件。修法：所有从 URL/DB 进入文件名的
字符串，先过白名单校验。新建 `api/utils`（streamsever/scheduler 各放一份或抽共享包）：

```go
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func IsValidVideoID(vid string) bool {
	return uuidPattern.MatchString(vid)
}
```

在 streamsever 两个 handler 和 scheduler 的 `videoDelRecHandler` 入口校验，不合法直接 400。
不要只依赖 `filepath.Base`，白名单才是根治。

### 1.2 密码哈希

现状：`pwd` 明文存 MySQL（users.pwd 是 text）。

改用 bcrypt（`golang.org/x/crypto/bcrypt`，版本已在 go.mod 的 indirect 里）：

```go
import "golang.org/x/crypto/bcrypt"

// api/dbops/api.go
func AddCredential(loginName, pwd string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	// INSERT INTO users (login_name, pwd) VALUES (?, ?)  // 存 hash
}

// Login handler 里
err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(userBody.Pwd))
```

改动点：`AddCredential` 存 hash、`GetCredential` 返回 hash、Login 改为 `CompareHashAndPassword`。
已有明文数据：测试库直接 `TRUNCATE users`，或提供一次性迁移脚本。

需要先执行 `go get golang.org/x/crypto/bcrypt`（会把它从 indirect 提升为直接依赖）。

### 1.3 输入校验

- `user_name`：非空、长度 3-32、去空白；
- `pwd`：非空、长度 6-64；
- 校验失败统一返回 400（错误码见 Phase 3 的 errs 包，先写死也行，后面统一收口）。

### 1.4 CORS 收紧

把 streamsever cors.go 的 `Access-Control-Allow-Origin: *` 改为读环境变量：

```go
origin := os.Getenv("ALLOW_ORIGIN")
if origin == "" {
	origin = "http://localhost:8080"
}
context.Header("Access-Control-Allow-Origin", origin)
```

### 验收

- 上传/播放接口传 `../`、`..%2f` 均返回 400；
- users 表里新密码是 `$2a$...` 哈希，登录仍正常；
- 超短用户名/密码被 400 拦截；
- 从非白名单 origin 访问 9000 被浏览器拦截。

---

## Phase 2：会话与认证闭环

### 目的
session 目前只在 api 进程内存里，重启即失效，且没有登出接口。

### 2.1 修复 RetrieveSession

`api/dbops/internal.go` 的 `SELECT user_name, TTL` 改成 `login_name`（与表一致）。

### 2.2 启动时恢复 session

api main 里初始化时调用一次：

```go
session.LoadSessionsFromDB()
```

这样重启后数据库里的 session 仍有效。

### 2.3 过期清理补全

`IsSessionValid` 当前只有"查不到"才删，过期分支没删。补上：

```go
if nowTime >= perSimpleSession.(*defs.SimpleSession).TTL {
	DeleteSession(sid) // 同时清内存和数据库
	return "", false
}
```

### 2.4 登出接口 + 受保护样板接口

后端（api/main.go 注册到 /api 组）：

```go
api.DELETE("/logout", Logout)   // 删 session 后返回 200
api.GET("/me", GetMe)           // 返回当前用户名，验证 X-Session-Id
```

`GetMe` 用 `context.GetHeader(HEADER_FIELD_USERNAME)` 取用户名（SessionMiddleware 已注入）。

前端 `logout()` 改为两步：先 `fetch("/api/logout", {method:"DELETE", headers:{...}})`，
成功或 401 都清 localStorage 再跳首页；userhome 加载时调 `GET /api/me`，401 自动踢回首页。

### 验收

- 登录 → 重启 api → 不重新登录，`/api/me` 仍返回 200；
- 手动等 TTL 过期（或临时改小 TTL）后 `/api/me` 返回 401，sessions 表记录被删；
- 登出后旧 session_id 访问 `/api/me` 返回 401。

---

## Phase 3：统一错误处理与响应信封

### 目的
消除三套 `SendErrorResponse`/`ErrResponse` 并存，让错误结构、错误码、日志全局唯一。
这是之前聊过的方案落地，**必须在 Phase 5 新增业务接口前做**，否则返工。

### 3.1 共享错误包

新建 `internal/errs/errs.go`（三服务同属一个 module，可直接 import）：

```go
package errs

type AppError struct {
	HTTPStatus int
	Code       string
	Message    string
	Cause      error
}

func (e *AppError) Error() string { return e.Message }

func BadRequest(msg string) *AppError { return &AppError{400, "BAD_REQUEST", msg, nil} }
func Unauthorized(msg string) *AppError { return &AppError{401, "UNAUTHORIZED", msg, nil} }
func NotFound(msg string) *AppError { return &AppError{404, "NOT_FOUND", msg, nil} }
func Internal(cause error) *AppError {
	return &AppError{500, "INTERNAL", "Internal server error", cause}
}
```

### 3.2 统一响应信封

定一个全局格式（成功失败同一结构）：

```json
{"code": "OK", "message": "success", "data": {...}}
{"code": "BAD_REQUEST", "message": "Request body is not correct.", "data": null}
```

建议放 `internal/httpx`：`WriteOK(c, data)`、`WriteErr(c, err)`。

### 3.3 错误中间件

`internal/httpx/error.go`：

```go
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

### 3.4 三服务接入

- 各 main 改为 `gin.New()` + `gin.Recovery()` + `ErrorHandler()`；
- handler 不再直接 `SendErrorResponse`，改 `c.Error(errs.Xxx(...))` + return；
- 删除 api/resonse.go、scheduler/response.go、streamsever/response.go 与三个 defs 里的
  ErrResponse/ErrorXxx 变量；
- 前端错误解析统一读 `data.message`（index.html 的 `data.error` 同步改）。

### 验收

- 参数错误/未登录/DB 错误分别返回对应 code 和 status；
- 未知 panic 返回 500 且真实错误进日志；
- curl 每个错误分支抽查一遍，三服务响应格式一致。

---

## Phase 4：scheduler 定时任务健壮化

### 目的
Worker 代码已存在但有两个 bug，且没接进 Start。

### 4.1 修 Worker

- `interval * time.Second`：参数已是 time.Duration，去掉乘法，调用方传 `2*time.Second`；
- 每轮新建 Runner：上一轮 CLOSE 后通道已关闭，复用同一 Runner 会 `send on closed channel`
  panic。最小改法：

```go
func (w *Worker) StartWorker() {
	for {
		select {
		case <-w.ticker.C:
			go CreateNewRunner(
				w.runner.dataSize,
				w.runner.longLived,
				w.runner.Dispatcher,
				w.runner.Executor,
			).StartAll()
		}
	}
}
```

### 4.2 Start 改为周期任务

```go
func Start() {
	w := CreateNewWorker(2*time.Second, *CreateNewRunner(3, false, VideoClearDispatcher, VideoClearExecuter))
	go w.StartWorker()
}
```

### 4.3 优雅停止（可选）

引入 context + `ticker.Stop()`，scheduler main 收到 SIGINT 时退出 Worker。

### 验收

- 手动插一条 `video_del_rec`，观察 scheduler 日志：每 2 秒检查一次，删除后不再处理；
- 空队列时不 panic、不刷错误日志；
- 连跑 10 分钟无 `send on closed channel`。

---

## Phase 5：视频业务闭环（打通三服务）

### 目的
现在上传只是"文件落盘"，video_info 表没人写、没有列表、没有删除链路。

### 5.1 上传后登记元数据

职责划分建议：

- streamsever `/upload/:vid-id`：只负责存文件、返回成功；
- 前端上传成功后调 api `POST /api/videos`（带 X-Session-Id）登记
  `{id, title, author_id}`（author_id 从登录态取）；

这样 api 保持"业务入口"的角色，streamsever 不碰业务表。

### 5.2 视频列表

api 新增（都挂在 /api 组）：

- `GET /api/videos`：列出当前用户视频（video_info WHERE author_id=?）；
- userhome 前端加载列表渲染播放链接。

### 5.3 删除链路

- api `DELETE /api/videos/:vid`：校验归属 → 删 video_info → 调 scheduler
  登记删除（HTTP POST/DELETE 到 `http://localhost:9001/video-del-rec/:vid`）；
- scheduler 把路由语义从 GET 改成 DELETE，入口做 Phase 1 的 UUID 校验；
- executor 真正删文件 + 删 video_del_rec；文件不存在视为成功（不重试）；
- 前端 userhome 每行加删除按钮。

### 5.4 跨服务调用封装

新建 `internal/streamclient`（或 api 内 http 小封装）：

```go
func NotifyVideoDeletion(vid string) error {
	// DELETE http://127.0.0.1:9001/video-del-rec/<vid>
	// 超时 2s；失败只记日志，不阻塞主流程（可后续加失败重试表）
}
```

端口从环境变量读（`SCHEDULER_ADDR`），别写死。

### 验收

- 上传 → 文件落盘 + video_info 出现一条记录；
- userhome 刷新能列出自己上传的视频并可播放；
- 删除后 video_info 记录消失、文件被清、video_del_rec 不留残余；
- 删别人的视频返回 403/404（归属校验生效）。

---

## Phase 6：前端体验与结构

### 目的
HTML 现在内联 script/css，页面功能也单一。

### 6.1 拆分静态资源

```text
static/
├── index.html        # 只留结构
├── userhome.html
├── css/style.css
└── scripts/
    ├── auth.js       # 登录/注册/登出/session 存取
    ├── api.js        # fetch 封装：自动带 X-Session-Id、统一 401 处理、解析信封
    ├── index.js
    └── userhome.js
```

`api.js` 里的统一请求函数（错误结构已在 Phase 3 统一，这里正好消费）：

```js
async function request(path, options = {}) {
  const res = await fetch(path, {
    ...options,
    headers: {
      "X-Session-Id": localStorage.getItem("session_id"),
      ...(options.headers || {}),
    },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401) { location.href = "/"; }
    throw new Error(data.message || "请求失败");
  }
  return data.data;
}
```

### 6.2 列表渲染

userhome 用 `document.createElement` 或简单模板字符串渲染视频列表，替换现在的单条硬编码。

### 6.3 上传状态

显示"上传中/成功/失败"，成功后刷新列表而不是只给一个播放器。

### 验收

- 全站无内联 JS/CSS；
- 任何接口 401 都自动回登录页且不白屏；
- 上传成功 → 列表自动多一行 → 可直接播放。

---

## Phase 7：可观测性与配置外置

### 目的
让服务可配置、可排查、可优雅停机。

### 7.1 配置集中

**原则**：环境变量是唯一真相来源；`.env` 只是本地开发的便利文件，生产环境只读真实 env
（Docker/K8s 注入），**不引入 yaml/viper 之类的配置文件**——多一个真相来源、多一层依赖，
还容易把密钥提交进仓库。

**新建 `internal/config/config.go`**（三服务同属一个 module，可直接 import）：

```go
package config

type MySQL struct {
	User, Pwd, Host, Port, DB string
}

func (m MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", m.User, m.Pwd, m.Host, m.Port, m.DB)
}

type Config struct {
	MySQL         MySQL
	APIAddr       string
	StreamAddr    string
	SchedulerAddr string
	VideoDir      string
	MaxUploadMB   int
	AllowOrigin   string
	GinMode       string
}

func Load() (*Config, error) // os.Getenv + 默认值 + 必填校验（如 MYSQL_PWD 缺失即报错）
func MustLoad() *Config      // 启动期使用：Load 失败直接 log.Fatal，fail fast
```

`MustXxx` 是 Go 的惯例（`regexp.MustCompile`、`template.Must`），语义是"启动期出错就退出"——
配置缺失时服务本来就不该起来，早崩早发现；但**请求处理里禁止 Must**，那会把配置问题变成用户的 500。

**新建根目录 `.env.example`**（提交到仓库；真实的 `.env` 已被 .gitignore 忽略）：

```text
MYSQL_USER=root
MYSQL_PWD=            # 必填，缺失启动即失败
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_DB=streamhub
API_ADDR=:8080
STREAM_ADDR=:9000
SCHEDULER_ADDR=:9001
VIDEO_DIR=./videos
MAX_UPLOAD_SIZE_MB=50
ALLOW_ORIGIN=http://localhost:8080
GIN_MODE=debug
```

**改造现有加载点**（`api/dbops/connection.go`、`scheduler/dbops/connection.go` 两份几乎一字不差）：

- 删掉 `init()` 里的加载逻辑、`os.Getwd()` 猜 `.env` 路径的三段 fallback、包级 `var err error`；
- dbops 只保留"拼 DSN + `sql.Open` + `Ping`"，改成显式 `Init(cfg config.Config)`
  （或 `Connect(m config.MySQL)`），由 main 调用，包内不再有副作用；
- 只在 main 入口加载一次 `.env`：本地 `godotenv.Load(".env")` 且**忽略 not-exist**
  （找不到说明用的是真实环境变量）；生产不调用 godotenv；
- streamsever 的 `VIDEO_DIR`、`MAX_UPLOAD_SIZE`、`ALLOW_ORIGIN`、监听端口同样接 config（它目前完全没接）；
- 代码里不再出现裸写的 `":8080"`、`"./videos/"`、`"localhost:3306"`、`"http://localhost:9001"`。

**为什么不能靠 cwd 猜路径**：同一份二进制从不同目录启动，`.env` 命中与否完全不同——
现在 scheduler 能连上库，只是因为 fallback 凑巧命中了 `api/.env`。统一约定
"从仓库根启动 + 根目录一份 .env"，生产则完全不读 `.env`。

### 7.2 日志统一 + request id

- 全部换 `log/slog`（Go 标准库）；
- api/streamsever 加请求日志中间件（method、path、status、耗时）；
- 响应头带 `X-Request-Id`，错误日志带同一 id，方便前后端对账。

### 7.3 健康检查与优雅停机

- 每服务加 `GET /healthz`（DB 服务顺带 `dbConnection.PingContext`）；
- main 用 `signal.NotifyContext` + `http.Server.Shutdown(ctx)`，收到 Ctrl+C 先停接新请求，
  等存量请求结束再退出。

### 验收

- 改端口/目录只动 .env，不动代码；
- 缺必填配置（如 `MYSQL_PWD`）时服务启动即失败，日志指明缺哪项；
- 三服务均通过 `internal/config` 取值，代码里搜不到 `":8080"`、`"./videos/"`、`"localhost:3306"`；
- 请求日志能看到 status/耗时/request id；
- Ctrl+C 时日志显示 graceful shutdown，无残留进程；
- `/healthz` 200，DB 断开时 api/scheduler 的 healthz 返回 503。

---

## Phase 8：测试补齐

### 目的
用测试锁住前面所有行为，防止升级回归。

### 8.1 替换 sleep 型测试

`runner_test.go` 现在 `time.Sleep(3s)` 等结束，改成确定性同步：
用 Runner.Error 通道收 CLOSE，或包一层 done channel。

### 8.2 单测清单

- `internal/errs`：AppError 构造与 errors.As；
- `internal/httpx`：ErrorHandler 对 AppError/未知错误/无错误的渲染（gin httptest）；
- `internal/config`：默认值填充、必填缺失报错、DSN 拼装；
- 视频 ID 校验：合法 UUID 通过，`../`、空串、超长拒绝；
- bcrypt 往返：注册 hash 后登录比对成功/失败；
- taskrunner：dispatcher 空记录 → 返回错误；executor 顺序删除调用次数正确（可注入 mock）。

### 8.3 handler 测试

用 `gin.CreateTestContext` + `httptest` 测 Login/CreateUser 的参数错误分支；
dbops 真库测试已有（api/dbops/api_test.go），保留但标记为需要 DB 的集成测试。

### 验收

- `go test ./...` 全绿且无 Sleep；
- 故意改坏 UUID 校验/错误码，测试能抓到。

---

## Phase 9：容器化与部署（远期，可选）

### 目的
把"本地能跑"变成"哪里都能部署"。

### 9.1 Docker

- 每服务一个多阶段 Dockerfile（build → 小镜像）；
- `docker-compose.yml`：mysql + api + streamsever + scheduler，
  mysql 初始化挂载 `schema.sql`，`videos` 用 volume；
- 前端仍由 api 托管（同源），或单独 nginx 静态层 + 反向代理 `/api`（等价 Phase 6 的分离）。

### 9.2 安全收尾

- 上线必须 HTTPS（前端明文传密码的问题由 TLS 收敛）；
- GIN_MODE=release；ALLOW_ORIGIN 指向真实域名；
- MySQL 用独立账号（不用 root）、密码走 secret 管理。

### 验收

- 新机器一条 `docker compose up` 起全套；
- schema.sql 自动初始化；
- 公网访问走 HTTPS，CORS 白名单只含自己的域名。

---

## 依赖关系速查

```text
Phase 0（收尾）
   └─ Phase 1（安全：先堵路径穿越/明文密码）
          └─ Phase 2（会话闭环）
                 └─ Phase 3（统一错误，必须先于新接口）
                        └─ Phase 5（业务闭环，依赖 2+3）
                               └─ Phase 6（前端，消费 3 的信封）
Phase 4（scheduler 健壮化）可与 1-3 并行
Phase 7/8 穿插在任何阶段后做；Phase 9 最后
```

## 每阶段做完的通用动作

1. `go build ./... && go vet ./...`；
2. `go test ./...`；
3. 按该阶段"验收"清单过一遍（浏览器 + curl）；
4. `git add -A && git commit`，commit message 写阶段名，如 `phase1: security baseline`。

遇到想不清取舍的步骤（比如 5.1 的职责划分、3.1 的错误码命名），先停手问，
不要在没对齐设计的情况下硬写。
