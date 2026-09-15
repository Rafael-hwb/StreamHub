# 阶段 1：统一登录校验中间件的错误处理

> 这次只改一个错误分支，并增加一个测试。不要迁移其他 handler。

## 1. 这一步完成后会有什么变化

访问需要登录的接口时，如果请求没有携带有效的 `X-Session-Id`：

```text
请求进入 SessionMiddleware
→ 发现 session 无效
→ 记录 Unauthorized 错误
→ 立即终止后面的 handler
→ ErrorHandler 统一返回 HTTP 401 JSON
```

预期响应为：

```json
{
  "code": "UNAUTHORIZED",
  "message": "User is not authenticated.",
  "data": null
}
```

本阶段的核心知识点只有一个：**Gin 中间件怎样阻止未授权请求继续执行。**

## 2. 为什么现在的代码还没有完全使用新机制

打开 `api/main.go`，目前的代码类似：

```go
func SessionMiddleware(context *gin.Context) {
	if !ValidateUserSession(context) {
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}
	context.Next()
}
```

这里仍然使用旧的 `SendErrorResponse`。我们要把这一处迁移到：

```text
创建应用错误 → 交给 context.Error → ErrorHandler 输出响应
```

但仅仅调用 `context.Error` 还不够。`context.Error` 的职责是“记录错误”，不是“停止请求”。

Gin 使用一条 handler 链处理请求：

```text
Logger → Recovery → ErrorHandler → SessionMiddleware → MyVideos
```

未登录时必须调用 `context.Abort()`。`Abort` 的意思是：标记当前请求，禁止继续执行链条后面的 handler。

## 3. 任务一：修改 SessionMiddleware

### 3.1 修改 import

打开 `api/main.go`。

找到：

```go
"github.com/Rafael-hwb/streamhub/api/defs"
```

因为修改后这个文件不再使用 `defs`，删除这一行。

然后加入：

```go
"github.com/Rafael-hwb/streamhub/internal/errs"
```

不要删除已经存在的 `internal/httpx`。

### 3.2 修改未登录分支

把这个旧调用：

```go
SendErrorResponse(context, defs.ErrorNotAuthUser)
```

改成两个动作：

1. 使用 `errs.Unauthorized(...)` 创建应用错误，并交给 `context.Error(...)`。
2. 调用 `context.Abort()` 阻止受保护 handler 继续执行。

修改后的控制流应满足：

```go
if session 无效 {
	记录 Unauthorized 错误
	终止 handler 链
	return
}
```

错误消息统一使用：

```text
User is not authenticated.
```

这里请你自己写两行 Go 调用，不要复制整份函数。写完后检查：`Abort()` 必须位于 `return` 之前。

### 3.3 关于成功分支的 `context.Next()`

本阶段先保留：

```go
context.Next()
```

它表示 session 有效时继续执行后面的业务 handler。以后整理中间件风格时再讨论是否需要调整，不在本次扩大范围。

### 3.4 格式化

在项目根目录执行：

```powershell
gofmt -w api/main.go
```

`gofmt` 会整理 import 顺序和缩进。没有输出通常表示执行成功。

## 4. 任务二：先手工验证编译

执行：

```powershell
go test ./api
```

预期：

```text
? github.com/Rafael-hwb/streamhub/api [no test files]
```

如果出现：

```text
imported and not used
```

说明 import 中还留着已经不用的包。根据报错中的文件和包名删除它，再运行 `gofmt`。

如果出现：

```text
undefined: errs
```

说明没有正确导入 `internal/errs`，或者包名拼错。

## 5. 任务三：增加一个最小 HTTP 测试

编译通过只能说明语法正确，不能证明请求真的被拦截。因此增加一个测试。

在 `api` 目录新建：

```text
api/main_test.go
```

### 5.1 测试需要的包

这个测试会用到：

```go
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)
```

术语解释：

- `testing`：Go 自带的测试框架。
- `httptest`：在内存中模拟 HTTP 请求和响应，不需要真的启动 8080 端口。
- `strings`：检查响应 JSON 中有没有指定文本。

### 5.2 测试步骤

创建测试函数：

```go
func TestProtectedRouteRejectsMissingSession(t *testing.T) {
	// 第 1 步：创建路由
	// 第 2 步：创建一个没有 X-Session-Id 的 GET 请求
	// 第 3 步：用 httptest.NewRecorder 接收响应
	// 第 4 步：让路由处理请求
	// 第 5 步：检查状态码是不是 401
	// 第 6 步：检查响应中是否包含 UNAUTHORIZED
}
```

使用的受保护地址是：

```text
GET /api/my/videos
```

创建路由：

```go
router := RegisterHandlers()
```

创建请求和响应记录器：

```go
request := httptest.NewRequest(http.MethodGet, "/api/my/videos", nil)
response := httptest.NewRecorder()
```

注意：不要给 request 添加 `X-Session-Id`，因为本测试就是模拟未登录用户。

处理请求：

```go
router.ServeHTTP(response, request)
```

检查 HTTP 状态码：

```go
if response.Code != http.StatusUnauthorized {
	t.Fatalf("expected status 401, got %d; body=%s", response.Code, response.Body.String())
}
```

检查新错误码：

```go
if !strings.Contains(response.Body.String(), `"code":"UNAUTHORIZED"`) {
	t.Fatalf("expected UNAUTHORIZED response, got %s", response.Body.String())
}
```

请根据上述小片段自行组装测试函数。这不是让你设计新架构，只是练习 HTTP 测试的 Arrange、Act、Assert：

```text
Arrange（准备）→ 创建路由、请求、响应记录器
Act（执行）     → router.ServeHTTP
Assert（断言）  → 检查 401 和错误码
```

### 5.3 为什么这个测试不需要数据库

请求先经过 `SessionMiddleware`。因为没有 session，它应该在访问 `MyVideos` 之前调用 `Abort()`。

因此：

```text
正确实现 → MyVideos 不执行 → 不访问数据库 → 返回 401
```

如果忘记 `Abort()`，后面的 `MyVideos` 可能继续访问未初始化的数据库连接，测试就可能 panic。这个测试也间接证明了权限中间件确实拦住了请求。

### 5.4 格式化并运行测试

```powershell
gofmt -w api/main_test.go
go test ./api -run TestProtectedRouteRejectsMissingSession -v
```

预期末尾看到：

```text
--- PASS: TestProtectedRouteRejectsMissingSession
PASS
```

## 6. 任务四：完成全量验证

单个测试通过后依次执行：

```powershell
go test ./api
go build ./...
go test ./...
```

三条命令都应通过。

然后检查本阶段差异：

```powershell
git diff -- api/main.go api/main_test.go
```

你的差异应该只包含：

- `api/main.go`：登录中间件从旧响应改为新错误，并调用 `Abort()`。
- `api/main_test.go`：新增未登录请求测试。

注意：如果 `api/main_test.go` 还没有被 Git 跟踪，普通 `git diff` 不会显示它。可以直接查看：

```powershell
Get-Content api/main_test.go
```

## 7. 本阶段不要做的事情

- 不修改 `api/handlers.go`。
- 不删除 `api/response.go`，其他 handler 仍然需要它。
- 不迁移 scheduler 和 streamsever。
- 不修改 `ErrorHandler` 的 JSON 格式。
- 不添加有效 session 的测试，因为那会引入数据库和 session 状态，超出本阶段范围。
- 不处理 `ValidateUser` 是否为死代码，留到代码清理阶段。

## 8. 验收清单

- [ ] `api/main.go` 不再导入 `api/defs`。
- [ ] session 无效时调用了 `context.Error(errs.Unauthorized(...))`。
- [ ] session 无效时在 `return` 前调用了 `context.Abort()`。
- [ ] `go test ./api -run TestProtectedRouteRejectsMissingSession -v` 通过。
- [ ] 测试断言了 HTTP 401。
- [ ] 测试断言了响应中的 `UNAUTHORIZED`。
- [ ] `go build ./...` 通过。
- [ ] `go test ./...` 通过。
- [ ] 没有修改其他 handler。

## 9. 你需要真正理解的三件事

1. `errs.Unauthorized(...)`：创建一个包含 HTTP 401 信息的错误对象。
2. `context.Error(...)`：把错误交给统一错误中间件；它不会自动停止请求。
3. `context.Abort()`：阻止 Gin 执行后面的 handler；它本身也不会自动生成 JSON。

三者配合关系是：

```text
Unauthorized 决定“是什么错误”
context.Error 决定“让统一中间件知道”
context.Abort 决定“不要继续执行业务”
```

## 10. 完成后发给导师

请发送：

```powershell
git diff -- api/main.go
Get-Content api/main_test.go
go test ./api -run TestProtectedRouteRejectsMissingSession -v
```

同时用一句话回答：为什么调用了 `context.Error` 以后还要调用 `context.Abort()`？

我会先审查这一小步，通过后再开启下一步。
