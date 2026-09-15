# 阶段 3：一次完成 API 的统一 HTTP 响应与错误处理

> 改造范围：整个 `api` 服务及共享的 `internal/httpx`。这次不再保留 API 的新旧双轨响应；完成后删除 `api/response.go` 和 `api/defs/error.go`。

## 1. 最终目标

所有 API 响应统一为：

```json
{
  "code": "OK",
  "message": "",
  "data": {}
}
```

失败响应统一为：

```json
{
  "code": "BAD_REQUEST",
  "message": "Request body is invalid.",
  "data": null
}
```

完成后，`api` 目录中不能再出现：

```text
SendErrorResponse
SendNormalResponse
defs.Error...
```

## 2. 先收尾并提交阶段 2

执行：

```powershell
gofmt -w api/handlers.go api/main_test.go
go test ./...
git diff --check
```

确认通过后提交当前工作：

```powershell
git add api/handlers.go api/main_test.go docs/progressive-refactor-guide.md docs/stage-2-create-user-validation.md
git diff --cached
git commit -m "feat: validate credential request input"
```

再开始本阶段。这样如果本阶段出错，可以明确区分两个阶段。

## 3. 第一步：建立统一响应结构

新建 `internal/httpx/response.go`：

```go
package httpx

import "github.com/gin-gonic/gin"

type Response struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, Response{
		Code:    "OK",
		Message: "",
		Data:    data,
	})
}
```

这里的 `any` 表示数据可以是用户、视频、数组或简单结果。

然后修改 `internal/httpx/error.go`，用同一个 `Response` 返回已知错误：

```go
c.AbortWithStatusJSON(appError.HTTPStatus, Response{
	Code:    appError.Code,
	Message: appError.Message,
	Data:    nil,
})
```

未知错误同样使用 `Response`：

```go
c.AbortWithStatusJSON(http.StatusInternalServerError, Response{
	Code:    "INTERNAL",
	Message: "Internal server error",
	Data:    nil,
})
```

格式化并检查共享包：

```powershell
gofmt -w internal/httpx
go test ./internal/...
```

## 4. 第二步：统一迁移 API 成功响应

在 `api/handlers.go` 中导入：

```go
"github.com/Rafael-hwb/streamhub/internal/httpx"
```

将所有成功响应替换为 `httpx.Success`：

| Handler | 状态码 | data |
| --- | ---: | --- |
| `CreateUser` | 201 | `signUpMessage` |
| `Login` | 200 | `signUpMessage` |
| `CreateVideoInfo` | 201 | `videoInfo` |
| `MyVideos` | 200 | `videos` |
| `GetVideoInfo` | 200 | `video` |
| `ListCommentsHandler` | 200 | `comments` |
| `AddCommentHandler` | 201 | `gin.H{"success": true}` |
| `DeleteVideoHandler` | 200 | `gin.H{"success": true}` |
| `ListVideosHandler` | 200 | `videos` |

替换示例：

```go
httpx.Success(context, http.StatusCreated, signUpMessage)
```

以及：

```go
httpx.Success(context, http.StatusOK, videos)
```

不要继续使用裸数字 `200`、`201`；统一使用 `net/http` 中的命名常量。

## 5. 第三步：按语义迁移全部失败分支

通用规则：

```go
context.Error(errs.BadRequest("...")) // 400，客户端输入不合法
context.Error(errs.Unauthorized("...")) // 401，没有有效身份或登录失败
context.Error(errs.Forbidden("...")) // 403，身份有效但无权操作
context.Error(errs.NotFound("...")) // 404，资源不存在
context.Error(errs.Internal(err)) // 500，内部错误；保留原始 cause 用于日志
```

每个错误调用后立即 `return`。

### 5.1 CreateUser

- JSON、用户名、密码校验：保留当前 `BadRequest`。
- `AddCredential` 返回错误：改为 `errs.Internal(err)`。

注意：用户名重复目前也会被当作 500。正确识别 MySQL duplicate key 留到数据访问层错误分类阶段，不要在 handler 中解析错误字符串。

### 5.2 Login

- JSON、用户名、密码校验：保留当前 `BadRequest`。
- `GetCredential` 查询失败：`errs.Internal(err)`。
- 用户不存在：`errs.Unauthorized("Invalid user name or password.")`。
- 密码错误：使用完全相同的 Unauthorized 消息。

用户不存在和密码错误必须返回相同消息，避免攻击者通过响应判断某个用户名是否注册。

### 5.3 CreateVideoInfo

调整执行顺序：先解析和校验 title，再查询当前用户 ID，避免无效请求访问数据库。

- JSON 错误：`BadRequest("Request body is invalid.")`
- `strings.TrimSpace(videoBody.Title) == ""`：`BadRequest("Video title is required.")`
- `GetUserIDByName` 返回 error：`Internal(err)`
- `aid == 0`：构造一个带上下文的内部错误，例如 `fmt.Errorf("authenticated user %q not found", username)`，再传给 `Internal`。
- `AddVideo` 失败：`Internal(err)`

输入校验通过后，可将规范化标题写回：

```go
videoBody.Title = strings.TrimSpace(videoBody.Title)
```

### 5.4 MyVideos

- 查询用户 ID 出错：`Internal(err)`
- session 中的用户在数据库不存在：这是系统状态不一致，使用带 cause 的 `Internal(...)`
- 查询视频列表失败：`Internal(err)`

不要把数据库错误伪装成 400 或 401。

### 5.5 GetVideoInfo

- 数据库查询失败：`Internal(err)`
- `video == nil`：`NotFound("Video not found.")`

### 5.6 ListCommentsHandler

- 查询失败：`Internal(err)`

当前函数不会区分“视频不存在”和“视频存在但没有评论”。空评论列表返回 200 是合理的；如果以后必须验证视频存在，再单独增加业务规则。

### 5.7 AddCommentHandler

调整顺序：先解析评论 JSON、校验 content，再查询视频和用户。

- JSON 错误：`BadRequest("Request body is invalid.")`
- `strings.TrimSpace(Content) == ""`：`BadRequest("Comment content is required.")`
- 查询视频失败：`Internal(err)`
- 视频不存在：`NotFound("Video not found.")`
- 查询当前用户失败：`Internal(err)`
- 用户不存在：带 cause 的 `Internal(...)`
- 添加评论失败：`Internal(err)`

校验通过后执行：

```go
commentBody.Content = strings.TrimSpace(commentBody.Content)
```

### 5.8 DeleteVideoHandler

- 查询当前用户失败：`Internal(err)`
- 用户不存在：带 cause 的 `Internal(...)`
- 查询视频失败：`Internal(err)`
- 视频不存在：`NotFound("Video not found.")`
- 当前用户不是作者：`Forbidden("You are not allowed to delete this video.")`
- 删除元数据失败：`Internal(err)`

这里最重要的修正是：已登录但不是所有者应返回 403，而不是 401。

scheduler 通知失败暂时保留日志并返回成功，因为数据库元数据已经删除；跨服务一致性将在单独模块中用超时 client 和可靠任务策略解决。

### 5.9 ListVideosHandler

- 列表查询失败：`Internal(err)`

## 6. 第四步：减少重复的“当前用户查询”

迁移完成后，`CreateVideoInfo`、`MyVideos`、`AddCommentHandler` 和 `DeleteVideoHandler` 都会重复：

```text
从 header 取 username → 根据 username 查询用户 ID
```

本阶段先不要抽 service，但可以在 `api/handlers.go` 内加入一个小辅助函数：

```go
func currentUserID(context *gin.Context) (int, error) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	authorID, err := dbops.GetUserIDByName(username)
	if err != nil {
		return 0, err
	}
	if authorID == 0 {
		return 0, fmt.Errorf("authenticated user %q not found", username)
	}

	return authorID, nil
}
```

四个 handler 改为：

```go
authorID, err := currentUserID(context)
if err != nil {
	context.Error(errs.Internal(err))
	return
}
```

这是合理的小型去重：只封装一个稳定动作，不创建空洞的 service 层。

## 7. 第五步：删除 API 旧错误系统

先搜索：

```powershell
rg -n 'SendErrorResponse|SendNormalResponse|defs\.Error' api
```

预期只剩旧文件自身的定义，handler 中不得有结果。

确认后删除：

```powershell
Remove-Item api/response.go
Remove-Item api/defs/error.go
```

这次删除是目标明确且可以由 Git 恢复的源码文件。

再搜索一次：

```powershell
rg -n 'SendErrorResponse|SendNormalResponse|ErrResponse|ErrorDBError|ErrorNotAuthUser' api
```

预期无输出。

## 8. 第六步：升级 HTTP 测试

### 8.1 不再用字符串查找 JSON 字段

字符串查找会受到空格和字段顺序影响。改为解码结构化 JSON。

在 `api/main_test.go` 中增加测试响应结构：

```go
type testResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}
```

增加辅助函数：

```go
func decodeResponse(t *testing.T, response *httptest.ResponseRecorder) testResponse {
	t.Helper()

	var body testResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return body
}
```

需要新增 import：

```go
"encoding/json"
```

原测试的断言改为直接比较：

```go
body := decodeResponse(t, response)
if body.Code != "BAD_REQUEST" {
	t.Fatalf("expected code BAD_REQUEST, got %q", body.Code)
}
if body.Message != test.message {
	t.Fatalf("expected message %q, got %q", test.message, body.Message)
}
```

### 8.2 增加视频和评论输入测试

受保护路由需要有效 session，而当前 session 包难以测试注入。为了不引入全局测试状态，本阶段直接对最终 handler 构造 Gin 测试上下文，验证数据库调用之前的输入校验。

至少覆盖：

- `CreateVideoInfo`：错误 JSON、空标题、纯空格标题。
- `AddCommentHandler`：错误 JSON、空内容、纯空格内容。

如果这些测试触发 nil 数据库 panic，说明 handler 的输入校验仍放在数据库访问之后。

### 8.3 增加 ErrorHandler 单元测试

在 `internal/httpx/error_test.go` 覆盖：

| 输入错误 | 预期状态 | code |
| --- | ---: | --- |
| `errs.BadRequest` | 400 | `BAD_REQUEST` |
| `errs.Unauthorized` | 401 | `UNAUTHORIZED` |
| `errs.Forbidden` | 403 | `FORBIDDEN` |
| `errs.NotFound` | 404 | `NOT_FOUND` |
| `errs.Internal` | 500 | `INTERNAL` |
| 普通 `errors.New` | 500 | `INTERNAL` |

使用表格驱动测试，路由中写一个测试 handler：

```go
func(c *gin.Context) {
	c.Error(test.err)
}
```

这组测试直接保护整个 API 依赖的错误映射规则。

## 9. 第七步：同步前端响应读取

成功响应现在包在 `data` 中。检查 `static/*.html` 中所有 `response.json()` 的使用。

以前：

```javascript
const result = await response.json();
const sessionId = result.session_id;
```

现在：

```javascript
const result = await response.json();
const sessionId = result.data.session_id;
```

列表、视频详情和创建视频返回值也都需要从 `result.data` 读取。

错误展示统一读取：

```javascript
result.message
```

不要再读取 `error`、`error_code` 或 `error_message`。

搜索残留：

```powershell
rg -n 'error_code|error_message|\.error\b|session_id|await .*\.json' static
```

逐处判断成功数据是否需要加 `.data`，不要盲目全局替换。

## 10. 完整验证

```powershell
gofmt -w api internal/httpx
git diff --check
go test ./internal/httpx -v
go test ./api -v
go test ./...
go build ./...
```

残留检查：

```powershell
rg -n 'SendErrorResponse|SendNormalResponse|defs\.Error|ErrResponse' api
```

预期无输出。

手工启动后至少验证：

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/user -ContentType application/json -Body '{"user_name":"","pwd":"123"}'
```

应返回 400、`BAD_REQUEST`、`User name is required.`。

## 11. 完成后的目录结果

```text
internal/httpx/
├── error.go          # error → HTTP 响应
├── error_test.go     # 错误映射测试
└── response.go       # 统一 Response 与成功响应

api/
├── handlers.go       # 只产生 AppError 或 Success
├── main.go
├── main_test.go
└── defs/
    └── apidefs.go    # 只保留请求与业务响应结构
```

## 12. 验收标准

- [ ] API 所有成功响应都通过 `httpx.Success`。
- [ ] API 所有失败都通过 `context.Error(errs...)`。
- [ ] 所有内部错误保留原始 cause，客户端只看到通用消息。
- [ ] 登录失败不泄露用户名是否存在。
- [ ] 非视频所有者返回 403。
- [ ] 视频不存在返回 404。
- [ ] 视频标题和评论内容在访问数据库前校验。
- [ ] 重复的当前用户 ID 查询已收敛到辅助函数。
- [ ] API 旧响应文件和旧错误定义已删除。
- [ ] 前端已适配统一响应信封。
- [ ] ErrorHandler 的 6 类映射测试通过。
- [ ] `go test ./...` 和 `go build ./...` 通过。

## 13. 提交建议

这个模块较大，但仍建议拆成三个可运行提交：

```text
1. feat: unify shared HTTP response envelope
2. refactor: migrate API handlers to application errors
3. test: cover API response and error contracts
```

每次提交前运行相关测试。第三个提交完成后再整体提交前端适配，或者把前端适配作为第四个提交。
