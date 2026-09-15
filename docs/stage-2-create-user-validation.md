# 阶段 2：统一注册与登录的输入校验

> 本阶段仍只学习一个知识点：HTTP 请求输入校验。但范围扩大到同一类别中的两个 handler：`CreateUser` 和 `Login`。

## 1. 本阶段要完成什么

统一处理下面 6 种客户端输入错误：

| 接口 | 错误情况 | HTTP 状态码 | 新错误码 |
| --- | --- | --- | --- |
| 注册 `POST /user` | JSON 格式错误 | 400 | `BAD_REQUEST` |
| 注册 `POST /user` | 用户名为空 | 400 | `BAD_REQUEST` |
| 注册 `POST /user` | 密码为空 | 400 | `BAD_REQUEST` |
| 登录 `POST /user/login` | JSON 格式错误 | 400 | `BAD_REQUEST` |
| 登录 `POST /user/login` | 用户名为空 | 400 | `BAD_REQUEST` |
| 登录 `POST /user/login` | 密码为空 | 400 | `BAD_REQUEST` |

处理顺序统一为：

```text
解析 JSON → 检查用户名 → 检查密码 → 最后访问数据库
```

## 2. 为什么把它们放在一起

`CreateUser` 和 `Login` 都接收 `defs.UserCredential`，校验规则相同，也能用同一组测试方法验证。这是一整个“用户凭证输入校验”类别，不会混入数据库、session 或视频业务。

数据库错误暂不迁移，因为那属于服务端错误处理，不是客户端输入校验。

## 3. 三种输入错误

### 3.1 JSON 格式错误

例如 `{"user_name":` 不是完整 JSON，`ShouldBindJSON` 会返回错误。

统一返回消息：

```text
Request body is invalid.
```

### 3.2 用户名为空

空字符串、缺少字段、只有空格都视为没有用户名。使用：

```go
strings.TrimSpace(userBody.UserName) == ""
```

统一返回：

```text
User name is required.
```

### 3.3 密码为空

本阶段只检查：

```go
userBody.Pwd == ""
```

不要对密码调用 `TrimSpace`，因为空格可能是密码的一部分，程序不应悄悄改变用户密码。

统一返回：

```text
Password is required.
```

## 4. 修改 `api/handlers.go`

### 4.1 增加 import

加入：

```go
"strings"

"github.com/Rafael-hwb/streamhub/internal/errs"
```

保留 `api/defs`，文件中其他代码仍在使用它。

### 4.2 修改 `CreateUser`

把 JSON 解析失败分支中的旧调用：

```go
SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
```

替换为：

```go
context.Error(errs.BadRequest("Request body is invalid."))
```

保留紧随其后的 `return`。

在解析成功后、`dbops.AddCredential` 之前增加两个判断：

```text
TrimSpace(UserName) 为空
→ context.Error(BadRequest("User name is required."))
→ return

Pwd 为空
→ context.Error(BadRequest("Password is required."))
→ return
```

本阶段保留 `AddCredential` 失败时的旧 `ErrorDBError` 写法。

### 4.3 修改 `Login`

使用完全相同的顺序：

```text
ShouldBindJSON
→ TrimSpace(UserName) 是否为空
→ Pwd 是否为空
→ dbops.GetCredential
```

三个输入错误使用和注册接口完全相同的消息。保留数据库错误、用户不存在和密码错误的旧处理方式。

### 4.4 为什么这里不调用 `Abort()`

`CreateUser` 和 `Login` 是最终业务 handler，`context.Error(...)` 后 `return` 就能结束函数。

认证中间件后面还有业务 handler，所以认证中间件必须 `Abort()`。简单记忆：

```text
中间件拒绝请求：context.Error + context.Abort + return
最终 handler 报错：context.Error + return
```

### 4.5 格式化并编译

```powershell
gofmt -w api/handlers.go
go test ./api
```

## 5. 用表格驱动测试覆盖 6 个场景

打开 `api/main_test.go`，保留现有测试，新增：

```go
func TestCredentialEndpointsRejectInvalidInput(t *testing.T)
```

### 5.1 定义测试表

```go
tests := []struct {
	name    string
	path    string
	body    string
	message string
}{
	// 六个用例写在这里
}
```

字段含义：

- `name`：场景名称。
- `path`：`/user` 或 `/user/login`。
- `body`：请求 JSON。
- `message`：预期错误消息。

### 5.2 注册接口的三个用例

```go
{
	name:    "register malformed json",
	path:    "/user",
	body:    `{"user_name":`,
	message: "Request body is invalid.",
},
{
	name:    "register missing username",
	path:    "/user",
	body:    `{"user_name":"   ","pwd":"123456"}`,
	message: "User name is required.",
},
{
	name:    "register missing password",
	path:    "/user",
	body:    `{"user_name":"rafael","pwd":""}`,
	message: "Password is required.",
},
```

再为 `/user/login` 写对应三个用例，总计 6 个。

### 5.3 循环执行

先创建一次 router：

```go
router := RegisterHandlers()
```

再循环：

```go
for _, test := range tests {
	t.Run(test.name, func(t *testing.T) {
		request := httptest.NewRequest(
			http.MethodPost,
			test.path,
			strings.NewReader(test.body),
		)
		request.Header.Set("Content-Type", "application/json")

		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		// 在这里写三个断言
	})
}
```

### 5.4 三个断言

检查 HTTP 400：

```go
if response.Code != http.StatusBadRequest {
	t.Fatalf("expected status 400, got %d; body=%s", response.Code, response.Body.String())
}
```

检查统一错误码：

```go
if !strings.Contains(response.Body.String(), `"code":"BAD_REQUEST"`) {
	t.Fatalf("expected BAD_REQUEST response, got %s", response.Body.String())
}
```

检查具体消息：

```go
if !strings.Contains(response.Body.String(), test.message) {
	t.Fatalf("expected message %q, got %s", test.message, response.Body.String())
}
```

三个断言分别证明：HTTP 层是 400、错误分类正确、具体校验分支正确。

## 6. 运行测试

```powershell
gofmt -w api/handlers.go api/main_test.go
go test ./api -run TestCredentialEndpointsRejectInvalidInput -v
go test ./api -v
go test ./...
```

第一个命令应显示 6 个子测试全部通过。

## 7. 常见问题

### 空用户名用例访问数据库

校验放到了数据库调用之后。将校验移动到 `AddCredential` 或 `GetCredential` 之前。

### 登录空密码返回 401

程序先查询或比较了密码。空密码是请求输入问题，应在数据库调用前返回 400。

### 只有空格的用户名没被拒绝

不要只写 `UserName == ""`，应先使用 `strings.TrimSpace`。

### 返回旧的 `error_code`

对应输入分支仍在调用 `SendErrorResponse`。检查两个 handler 的 JSON、用户名和密码三个分支。

## 8. 本阶段不要修改

- 不迁移数据库错误。
- 不迁移用户不存在或密码错误的 401。
- 不修改密码存储方式。
- 不抽取公共校验函数；先保持代码直观。
- 不修改视频、评论、scheduler、streamsever 或前端。
- 不删除旧错误定义和 `response.go`。

## 9. 验收标准

- [ ] 两个 handler 都按“JSON → 用户名 → 密码 → 数据库”的顺序执行。
- [ ] JSON 错误使用 `errs.BadRequest("Request body is invalid.")`。
- [ ] 用户名使用 `strings.TrimSpace` 检查。
- [ ] 密码使用 `Pwd == ""` 检查。
- [ ] 每个错误分支在 `context.Error(...)` 后立即 `return`。
- [ ] 6 个表格驱动子测试全部通过。
- [ ] 每个用例检查 400、`BAD_REQUEST` 和具体消息。
- [ ] `go test ./...` 通过。
- [ ] 没有修改范围外业务代码。

## 10. 完成后发给导师

```powershell
git diff -- api/handlers.go api/main_test.go
go test ./api -run TestCredentialEndpointsRejectInvalidInput -v
go test ./...
```

再回答：为什么用户名适合使用 `TrimSpace`，而密码不应该自动 `TrimSpace`？
