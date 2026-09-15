# C3 迁移指南：handler 错误处理统一

> 对应 `docs/code-optimization-guide.md` 的 C3 第 2 步：
> "handler 不再调 SendErrorResponse，改成 `c.Error(errs.Xxx(...))` + `return`"。
>
> 本指南基于 2026-09-10 的仓库现状写成，逐文件、逐个调用点给了替换方案。

---

## 0. 现状快照（先看这个）

### 已经做好的

- `internal/errs/errs.go`：`AppError` + `BadRequest` / `Unauthorized` / `Forbidden` / `NotFound` / `Internal`；
- `internal/httpx/error.go`：`ErrorHandler()` 中间件；
- `internal/config`、`internal/dbconn`：配置与连接收敛（C2）；
- 三个旧的 `response.go` 已经删除（`api/`、`scheduler/`、`streamsever/`）；
- `api/main.go`、`scheduler/main.go` 已经接上 `config.Load` + `dbops.Init`。

### 还没做的（也就是本指南要解决的）

- 三个服务仍在调用已删除的 `SendErrorResponse` / `SendNormalResponse` → **编译不过**；
- 三个 `main` 还是 `gin.Default()`，没挂 `httpx.ErrorHandler()`；
- 旧的错误定义还留着：`api/defs/error.go`、`streamsever/defs.go`；
- 前端还在读 `data.error`，新信封是 `data.message`。

### 当前 `go build ./...` 的报错（原文）

```text
# api
api\auth.go:33:21: syntax error: unexpected name error in argument list; possibly missing comma or )

# streamsever
streamsever\handlers.go:15:3: undefined: SendErrorResponse   （另有 5 处）
streamsever\handlers.go:61:2: undefined: SendNormalResponse
streamsever\main.go:11:4: undefined: SendErrorResponse

# scheduler
scheduler\handlers.go:12:3: undefined: SendErrorResponse
scheduler\handlers.go:18:3: undefined: SendErrorResponse
scheduler\handlers.go:21:2: undefined: SendNormalResponse
```

> `api/auth.go` 是语法错误，它挡住了 api 包其余所有报错（包括 handlers 里那些
> `undefined: SendErrorResponse`）。先修它，才能看到 api 的真实清单。

---

## 1. 两条铁律

**handler 里：**

```go
if err != nil {
	context.Error(errs.Internal(err))   // 只报告错误
	return                              // 必须 return
}
```

**中间件里（SessionMiddleware / LimiterMiddleware）：**

```go
if !ok {
	context.Error(errs.Unauthorized("User authentication failed."))
	context.Abort()   // ← 中间件必须 Abort
	return
}
```

为什么中间件多一个 `Abort()`？因为 gin 的 `Next()` 是**循环**：中间件里只 `return`
（不 `Abort`）的话，最外层循环会继续把后面的 handler 跑完——鉴权失败也照样执行业务。
`Abort()` 把游标推到末尾，才能真正截断。

**响应只由 `ErrorHandler` 写。** handler 报告错误后必须 `return`，不能再自己写响应，
否则会出现两段 JSON / `superfluous WriteHeader`。

---

## 2. 先修两个阻塞性 bug

### 2.1 `api/auth.go`：语法错误 + 死代码

现状（第 29-38 行）：

```go
func ValidateUser(context *gin.Context) bool {
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	if len(username) == 0 {
		context.Error(err error)   // ← 语法错误
		return false
	}

	return true
}
```

这个 `ValidateUser` 全仓没有任何调用方（C4.3 已判定为死代码），**直接整段删掉**。
删完 `auth.go` 顶部的 `defs` import 如果不再被使用，也一并删掉
（`ValidateUserSession` 只用 `session` 包）。

### 2.2 `scheduler/main.go`：导错了 dbops 包

```go
import (
	...
	"github.com/Rafael-hwb/streamhub/api/dbops"          // ← 错
	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/Rafael-hwb/streamhub/scheduler/taskrunner"
	...
)
```

改成 `github.com/Rafael-hwb/streamhub/scheduler/dbops`。

不改的后果：`dbops.Init(*cfg)` 初始化的是 **api 的**连接，scheduler 自己的
`dbConnection` 永远是 nil → 第一次写 `video_del_rec` 就 nil panic。

---

## 3. 三个 main 挂上 ErrorHandler

把 `gin.Default()` 换成显式三件套（`gin.Default()` 内部的 Recovery 会自己写 500，
绕过统一信封，所以不能再用它）：

```go
router := gin.New()
router.Use(gin.Logger(), gin.Recovery(), httpx.ErrorHandler())
```

三个服务都一样，import 里加：

```go
"github.com/Rafael-hwb/streamhub/internal/httpx"
```

**注册顺序要求**：`httpx.ErrorHandler()` 必须在 `SessionMiddleware`（api）和
`LimiterMiddleware`（streamsever）**之前**进入链，否则中间件报的错没人渲染。
现在 api 是 `authAPI.Use(SessionMiddleware)`、streamsever 是 `router.Use(LimiterMiddleware(10))`，
只要保证 `router.Use(httpx.ErrorHandler())` 写在它们前面即可。

### api/main.go 的 SessionMiddleware 同步改

```go
func SessionMiddleware(context *gin.Context) {
	if !ValidateUserSession(context) {
		context.Error(errs.Unauthorized("User authentication failed."))
		context.Abort()
		return
	}
	context.Next()
}
```

import 加 `internal/errs`；`defs` 若不再使用则删。

---

## 4. 错误码映射表（旧 → 新）

| 旧写法 | 新写法 | 状态码 / code |
|---|---|---|
| `defs.ErrorRequestBodyParseFailed` | `errs.BadRequest("Request body is not correct.")` | 400 / `BAD_REQUEST` |
| `defs.ErrorNotAuthUser` | `errs.Unauthorized("User authentication failed.")` | 401 / `UNAUTHORIZED` |
| `defs.ErrorNotVideoOwner`（原来错用 401） | `errs.Forbidden("Not the owner of this video.")` | 403 / `FORBIDDEN` |
| `defs.ErrorVideoNotFound` | `errs.NotFound("Video not found.")` | 404 / `NOT_FOUND` |
| `defs.ErrorDBError` / `ErrorInternalFaults` | `errs.Internal(err)` | 500 / `INTERNAL` |
| streamsever `ErrorTooManyRequests` | `errs.TooManyRequests("Too many requests.")` | 429 / `TOO_MANY_REQUESTS` |
| streamsever `ErrorFileTooBig` | `errs.BadRequest("File is too big.")` | 400 / `BAD_REQUEST` |
| streamsever `ErrorRequestError` | `errs.BadRequest("File request error.")` | 400 / `BAD_REQUEST` |
| `SendNormalResponse(context, code, data)` | `context.JSON(code, data)` | 结构不变 |

**`internal/errs` 需要补一个构造函数**（现在没有 429）：

```go
func TooManyRequests(msg string) *AppError {
	return &AppError{429, "TOO_MANY_REQUESTS", msg, nil}
}
```

> 关于成功响应：本期**只统一错误信封**，成功响应继续用 `context.JSON(...)`，结构不变——
> 这样前端除了错误提示那一行，其他地方都不用动。`httpx.WriteOK` 留到后续做。

> 关于 `errs.Internal(err)`：**把真实 err 传进去**（它会作为 `Cause` 进日志）。
> 原代码 `SendErrorResponse(context, defs.ErrorDBError)` 丢掉了错误内容，
> 迁移时正好补回来——这是这次改造最实际的收益之一。

---

## 5. 逐文件改法

> 本步骤**不动参数名**（`context` 改 `c` 是 C4 的事），只换错误调用，保持 diff 机械、可对照。

### 5.1 `api/handlers.go`（26 处错误 + 9 处成功）

按行号逐一替换，条件不变、文案保持兼容：

| 行号 | 所属 handler | 触发条件 | 改成 |
|---|---|---|---|
| 15 | CreateUser | body 解析失败 | `errs.BadRequest("Request body is not correct.")` |
| 20 | CreateUser | AddCredential 失败 | `errs.Internal(err)` |
| 35 | CreateVideoInfo | 取用户 id 失败 | `errs.Internal(err)` |
| 41 | CreateVideoInfo | body 解析失败 | `errs.BadRequest("Request body is not correct.")` |
| 46 | CreateVideoInfo | title 为空 | `errs.BadRequest("title is required")` |
| 52 | CreateVideoInfo | AddVideo 失败 | `errs.Internal(err)` |
| 63 | Login | body 解析失败 | `errs.BadRequest("Request body is not correct.")` |
| 69 | Login | GetCredential 失败 | `errs.Internal(err)` |
| 74 | Login | 用户不存在 | `errs.Unauthorized("User authentication failed.")` |
| 79 | Login | 密码不匹配 | `errs.Unauthorized("User authentication failed.")` |
| 94 | MyVideos | 取用户 id 失败 | `errs.Internal(err)` |
| 100 | MyVideos | 查询失败 | `errs.Internal(err)` |
| 113 | GetVideoInfo | 查询失败 | `errs.Internal(err)` |
| 117 | GetVideoInfo | 视频不存在 | `errs.NotFound("Video not found.")` |
| 130 | ListCommentsHandler | 查询失败 | `errs.Internal(err)` |
| 143 | AddCommentHandler | GetVideo 失败 | `errs.Internal(err)` |
| 148 | AddCommentHandler | 视频不存在 | `errs.NotFound("Video not found.")` |
| 154 | AddCommentHandler | 取用户 id 失败 | `errs.Internal(err)` |
| 160 | AddCommentHandler | body 解析失败 | `errs.BadRequest("Request body is not correct.")` |
| 165 | AddCommentHandler | content 为空 | `errs.BadRequest("content is required")` |
| 171 | AddCommentHandler | AddComment 失败 | `errs.Internal(err)` |
| 185 | DeleteVideoHandler | 取用户 id 失败 | `errs.Internal(err)` |
| 191 | DeleteVideoHandler | GetVideo 失败 | `errs.Internal(err)` |
| 196 | DeleteVideoHandler | 视频不存在 | `errs.NotFound("Video not found.")` |
| 201 | DeleteVideoHandler | **非作者** | `errs.Forbidden("Not the owner of this video.")` ← 顺带修 401→403 |
| 206 | DeleteVideoHandler | 删除失败 | `errs.Internal(err)` |
| 224 | ListVideosHandler | 查询失败 | `errs.Internal(err)` |

成功响应对应替换（结构不变，只换函数）：

```go
SendNormalResponse(context, 201, signUpMessage)              // CreateUser
SendNormalResponse(context, 201, videoInfo)                  // CreateVideoInfo
SendNormalResponse(context, 200, signUpMessage)              // Login
SendNormalResponse(context, 200, videos)                     // MyVideos
SendNormalResponse(context, 200, video)                      // GetVideoInfo
SendNormalResponse(context, 200, comments)                   // ListCommentsHandler
SendNormalResponse(context, 201, gin.H{"success": true})     // AddCommentHandler
SendNormalResponse(context, 200, gin.H{"success": true})     // DeleteVideoHandler
SendNormalResponse(context, 200, videos)                     // ListVideosHandler
```

统一改成 `context.JSON(状态码, 原数据)`，例如：

```go
context.JSON(http.StatusCreated, videoInfo)
```

改造后的示例（CreateVideoInfo，完整对照）：

```go
func CreateVideoInfo(context *gin.Context) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)
	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		context.Error(errs.Internal(err))
		return
	}

	videoBody := &defs.VideoCreateRequest{}
	if err := context.ShouldBindJSON(videoBody); err != nil {
		context.Error(errs.BadRequest("Request body is not correct."))
		return
	}

	if len(videoBody.Title) == 0 {
		context.Error(errs.BadRequest("title is required"))
		return
	}

	videoInfo, err := dbops.AddVideo(aid, videoBody.Title)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	context.JSON(http.StatusCreated, videoInfo)
}
```

`defs` import 保留（`UserCredential`、`VideoCreateRequest`、`CommentCreateRequest` 还在用），
只是不再引用它里面的错误变量。

### 5.2 `scheduler/handlers.go`（2 处 + 1 处成功）

```go
func videoDelRecHandler(context *gin.Context) {
	vid := context.Param("vid-id")

	if len(vid) == 0 {
		context.Error(errs.BadRequest("Video id should not be empty."))
		return
	}

	if err := dbops.AddVideoDeletionRecord(vid); err != nil {
		context.Error(errs.Internal(err))
		return
	}

	context.JSON(http.StatusOK, gin.H{"success": true})
}
```

注意原来最后是 `SendNormalResponse(context, 200, "")`（空字符串响应体），
改成 `gin.H{"success": true}` 更规范；api 侧 `http.Get` 只关心状态码，不受影响。

### 5.3 streamsever

**先补 `errs.TooManyRequests`**（见第 4 节）。

`streamsever/handlers.go`：

| 行号 | 触发条件 | 改成 |
|---|---|---|
| 15 | 模板解析失败 | `errs.Internal(err)` |
| 21 | 模板执行失败 | `errs.Internal(err)` |
| 32 | 打开视频文件失败 | `errs.Internal(err)`（见下方备注） |
| 43 | ParseMultipartForm 失败 | `errs.BadRequest("File is too big.")` |
| 51 | FormFile 失败 | `errs.BadRequest("File request error.")` |
| 57 | SaveUploadedFile 失败 | `errs.Internal(err)` |
| 61 | 上传成功 | `context.JSON(http.StatusCreated, gin.H{"success": true})` |

> 备注：第 32 行"文件不存在"现在返回 500。更合理的是 404（`errs.NotFound`），
> 但那会改变对外行为（播放器对 404 的处理不同），本步保持 500，改不改单独决定。

`streamsever/main.go` 的限流中间件（第 11 行）——注意这是**中间件**，要 `Abort`：

```go
func LimiterMiddleware(maxCount int) gin.HandlerFunc {
	limiter := CreateConnectionLimiter(maxCount)
	return func(context *gin.Context) {
		if !limiter.GetConnection() {
			context.Error(errs.TooManyRequests("Too many requests."))
			context.Abort()
			return
		}
		defer limiter.ReleaseConnection()
		context.Next()
	}
}
```

### 5.4 删除旧的错误定义

调用点全部替换完之后：

- **删除整个 `api/defs/error.go`**（`Err`、`ErrResponse` 和 6 个 `ErrorXxx` 变量）；
- `streamsever/defs.go`：删掉 `ErrStruct`、`ErrResponse` 和 4 个 `ErrorXxx` 变量，
  保留 `VIDEO_DIR` / `MAX_UPLOAD_SIZE`；`net/http` import 随之删掉
  （它只被那些错误变量用到）。

---

## 6. 前端同步（2 处）

新错误信封是 `{code, message, data}`，旧的是 `{error, error_code}`。需要改：

- `static/index.html:43`：`data.error || "请求失败"` → `data.message || "请求失败"`；
- `static/userhome.html:72`：`meta.error || "登记失败"` → `meta.message || "登记失败"`。

顺带检查 `static/video.html`（评论/详情的 fetch 分支）有没有直接显示错误文案的地方，
有的话同样改成 `data.message`。

---

## 7. 验收

### 7.1 静态检查

```powershell
gofmt -w api scheduler streamsever internal
go vet ./...
go build ./...
```

### 7.2 残留检查（应无输出）

```powershell
rg -n "SendErrorResponse|SendNormalResponse" api scheduler streamsever
rg -n "ErrorDBError|ErrorNotAuthUser|ErrorVideoNotFound|ErrorNotVideoOwner|ErrorRequestBodyParseFailed|ErrorInternalFaults" api scheduler
rg -n "ErrorTooManyRequests|ErrorFileTooBig|ErrorRequestError" streamsever
```

### 7.3 接口行为（curl）

| 场景 | 期望 |
|---|---|
| `POST /user`（空 body） | 400 `{"code":"BAD_REQUEST",...}` |
| `POST /user/login`（错密码） | 401 `{"code":"UNAUTHORIZED",...}` |
| `GET /api/videos/<不存在的 vid>` | 404 `{"code":"NOT_FOUND",...}` |
| `DELETE /api/videos/<别人的 vid>`（带 session） | 403 `{"code":"FORBIDDEN",...}`（原来是 401） |
| 停掉 MySQL 后调 `GET /api/videos` | 500 `{"code":"INTERNAL",...}`，日志里有 `cause=` 真实错误 |
| streamsever 第 11 个并发请求 | 429 `{"code":"TOO_MANY_REQUESTS",...}` |
| 不合法 session 调 `GET /api/my/videos` | 401，且**没有**执行业务逻辑 |

### 7.4 前端

- 登录失败 → 提示显示后端 `message`（不再只显示"请求失败"兜底）；
- 上传登记失败 → 提示显示后端 `message`；
- 其余成功流程（登录、列表、详情、评论、上传）行为与改造前一致。

---

## 8. 坑清单

1. **中间件只 `return` 不 `Abort`，后面的 handler 照样跑**——gin 的 `Next()` 是循环，
   鉴权/限流失败必须 `context.Abort()`。
2. **`context.Error(...)` 之后必须 `return`**，且不能自己再写响应，否则两段 JSON。
3. **`httpx.ErrorHandler()` 要注册在 `SessionMiddleware` / `LimiterMiddleware` 之前**，
   否则中间件报的错不会被渲染。
4. **`gin.Default()` 要换掉**：它自带的 Recovery 会直接写 500，绕过统一信封；
   用 `gin.New()` + `gin.Logger()` + `gin.Recovery()`。
5. **`errs.Internal(err)` 一定要把 err 传进去**，别学旧代码丢掉——那是排查线上问题的唯一线索。
6. **成功响应这次不改结构**，避免前端连带改动；统一信封（`httpx.WriteOK`）留到后续。
7. **`scheduler/main.go` 的 `api/dbops` import 必须改成 `scheduler/dbops`**，
   否则 scheduler 运行必 nil panic（见 2.2）。
