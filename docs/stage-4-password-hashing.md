# 阶段 4：密码安全 —— 用 bcrypt 哈希替换明文存储

> 改造范围：`api/dbops` 的凭据读写、`api/handlers.go` 的登录校验，以及 `api/dbops/api_test.go` 中对应的集成测试。
> 前置状态：阶段 3（API 统一响应与错误处理）的改动还躺在工作区，尚未提交。本阶段开始前先把阶段 3 收尾提交，避免两个阶段的改动混在一起。

## 1. 学习目标

本阶段只学一个核心概念：**哈希（hash）与加密（encryption）的区别**，以及为什么密码必须用「单向哈希 + 随机盐」存储而不是明文或可逆加密。完成后，数据库里的 `users.pwd` 不再保存用户真实密码。

一句话验收：即使有人直接拿到 `users` 表的完整内容，也无法还原出任何用户的登录密码。

## 2. 前置：先收尾阶段 3

当前工作区有 4 个未提交文件：

```text
api/handlers.go
api/main_test.go
internal/httpx/error.go
internal/httpx/response.go
```

先验证并提交它们：

```powershell
gofmt -w api/handlers.go api/main_test.go internal/httpx
go test ./...
git diff --check
```

确认通过后：

```powershell
git add api/handlers.go api/main_test.go internal/httpx/error.go internal/httpx/response.go docs/stage-3-api-http-contract.md
git diff --cached
git commit -m "refactor: unify API HTTP response and error handling"
```

这样如果本阶段出错，你能明确区分「阶段 3 的改动」和「阶段 4 的改动」。

## 3. 现状与问题

先自己读一遍这三处代码，用一句话写下每一处在做什么、哪里不安全：

1. `api/dbops/api.go` 的 `AddCredential` —— 把 `pwd` 原样 `INSERT` 进 `users.pwd`。
2. `api/dbops/api.go` 的 `GetCredential` —— 把 `users.pwd` 原样 `SELECT` 出来返回。
3. `api/handlers.go` 的 `Login` —— 拿返回的密码和用户输入的 `userBody.Pwd` 做 `!=` 比较。

当前问题：密码明文落库。数据库一旦泄露（备份文件、误配置的日志、SQL 注入读到整行），所有用户的密码直接暴露。更糟的是很多用户在不同网站复用同一密码，一处泄露会连累其他账号。

同时注意 `schema.sql` 里 `pwd VARCHAR(64)`：bcrypt 的输出固定 60 个字符，`VARCHAR(64)` 足够，**不需要**改列长。这本身就是个知识点——先算清目标数据到底多长，再决定要不要动 schema。

## 4. 核心知识点：哈希 vs 加密

这是本阶段真正要理解的东西，比改代码更重要。

| 维度 | 加密（encryption） | 哈希（hash） |
| --- | --- | --- |
| 可逆性 | 可逆，有密钥就能还原明文 | 不可逆，无法从输出还原输入 |
| 用途 | 传输中保护数据、存储后要读回原值 | 验证「输入是否等于当初的输入」，不需要读回原值 |
| 密码场景 | 错误：谁拿到密钥谁就能解出所有密码 | 正确：只存哈希，登录时重新哈希后比对 |

密码登录的本质是「验证」，不是「还原」。我们永远不需要把密码读回来，只需要回答一个问题：**用户这次输入的密码，和当初注册的是不是同一个？** 哈希正好只回答这个问题。

### 为什么还要「盐（salt）」？

如果直接 `hash("123456")`，所有密码为 `123456` 的用户会得到同一个哈希。攻击者可以提前算好「常见密码 → 哈希」的对照表（彩虹表），一查就命中。盐是每个用户一段随机数据，混进密码一起哈希，让「同一个密码」在不同用户那里得到「不同的哈希」，彩虹表就失效了。

### 为什么选 bcrypt？

- bcrypt 自己内置了随机盐，你不需要手动管盐。
- bcrypt 是「慢哈希」：每次计算刻意耗时（cost 因子），拖慢暴力破解。SHA-256 这类「快哈希」不适合密码，因为攻击者每秒能试几十亿次。
- Go 标准库没有 bcrypt，官方推荐用 `golang.org/x/crypto/bcrypt`。

登录时的正确姿势不是「解密比对」，而是：

```go
bcrypt.CompareHashAndPassword([]byte(存储的哈希), []byte(用户输入))
```

这个函数内部会：取出盐 → 用同样的盐和 cost 重新哈希输入 → 比较结果是否一致。

## 5. 依赖与成本

`go.mod` 里已经有 `golang.org/x/crypto v0.48.0 // indirect`，但 bcrypt 子包还没被直接引用。执行：

```powershell
go get golang.org/x/crypto/bcrypt
```

之后 `go mod tidy` 会把它从 indirect 提升为 direct 依赖。

## 6. 步骤

### 步骤 1：在写库时哈希

修改 `api/dbops/api.go` 的 `AddCredential`，让「哈希」发生在数据访问层，而不是 handler 层。这样任何调用方都天然得到安全行为，不可能有人「忘记哈希」。

导入：

```go
"golang.org/x/crypto/bcrypt"
```

改造后：

```go
func AddCredential(loginName string, pwd string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	stmtIns, err := dbConnection.Prepare("INSERT INTO users (login_name,pwd) VALUES (?,?)")
	if err != nil {
		return err
	}
	defer stmtIns.Close()

	_, err = stmtIns.Exec(loginName, string(hash))
	if err != nil {
		return err
	}

	return nil
}
```

`bcrypt.DefaultCost` 是 10，对学习和小规模项目足够；生产环境可调高，但要测一下单次耗时。

### 步骤 2：让 `GetCredential` 的语义变清楚

`GetCredential` 现在的名字暗示「拿到密码」，但实际上它应该返回「存储的哈希」。查询逻辑本身不用大改（还是 `SELECT pwd`），但它返回的是哈希字符串，登录方要用 `bcrypt.CompareHashAndPassword` 处理。

最小改法是把返回变量名从 `pwd` 改成 `hash`，让读者一眼看出它不是明文。是否重命名函数（例如 `GetPasswordHash`）由你决定，但至少不要在 `Login` 里把它当成明文去比较。

### 步骤 3：改 `Login` 校验

`api/handlers.go` 的 `Login` 当前：

```go
pwd, err := dbops.GetCredential(userBody.UserName)
if err != nil {
	context.Error(errs.Internal(err))
	return
}

if len(pwd) == 0 {
	context.Error(errs.Unauthorized("Invalid user name or password."))
	return
}

if pwd != userBody.Pwd {
	context.Error(errs.Unauthorized("Invalid user name or password."))
	return
}
```

改成：

```go
hash, err := dbops.GetCredential(userBody.UserName)
if err != nil {
	context.Error(errs.Internal(err))
	return
}

if len(hash) == 0 {
	context.Error(errs.Unauthorized("Invalid user name or password."))
	return
}

if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(userBody.Pwd)); err != nil {
	context.Error(errs.Unauthorized("Invalid user name or password."))
	return
}
```

关键点：**用户不存在和密码错误必须继续返回同一条消息**（阶段 3 已经定下的规则），不能因为「哈希比对失败」就换成另一条消息，否则攻击者能借此枚举用户名。

### 步骤 4：处理 `DeleteCredential`（一段死代码）

用 grep 确认 `DeleteCredential` 的业务调用方：

```powershell
rg -n "DeleteCredential" --glob "*.go" --glob "!*_test.go"
```

你会发现它只被测试调用，没有任何 handler 用它。它的签名 `DeleteCredential(loginName, pwd string)` 用明文密码做 `WHERE login_name = ? AND pwd = ?`，在哈希化之后这个语义已经彻底不成立——你没法用明文去匹配哈希。

选择（任选其一，但要讲出理由）：

- **推荐**：删除 `DeleteCredential`，同时删掉测试里对应的两个子用例（见步骤 5）。死代码 + 过时签名是负债，不是资产。
- 保守：把它改成 `DeleteCredential(loginName string)`，只按 `login_name` 删除，去掉密码参数。

不要保留一个带明文密码参数、却永远跑不对的函数。

### 步骤 5：更新集成测试

`api/dbops/api_test.go` 的 `TestUserWorkflow` 目前断言明文相等：

```go
func testGetUser(t *testing.T) {
	pwd, err := GetCredential("avenssi")
	if err != nil || pwd != "123" {
		t.Errorf("Error of GetUser: %v", err)
	}
}
```

哈希化后，这个断言必然失败。改成验证「存储的是哈希，且能通过 bcrypt 校验」：

```go
func testGetUser(t *testing.T) {
	hash, err := GetCredential("avenssi")
	if err != nil {
		t.Errorf("GetCredential: %v", err)
		return
	}
	if hash == "" {
		t.Errorf("expected a stored hash, got empty")
		return
	}
	if hash == "123" {
		t.Errorf("password must not be stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("123")); err != nil {
		t.Errorf("password should match stored hash: %v", err)
	}
}
```

这里有两个断言都重要：

1. `hash == "123"` 必须为假——确保没有明文落库。
2. `CompareHashAndPassword` 必须通过——确保登录逻辑依然能验证正确密码。

如果步骤 4 删掉了 `DeleteCredential`，就把 `TestUserWorkflow` 里的 `Delete`、`Reget` 两个子用例一并删掉，只保留 `Add` 和 `Get`。测试里不要引入明文密码参数。

### 步骤 6：迁移已有明文用户

真实项目里最容易被忽略的一步：改造完代码后，**数据库里已经存在的明文密码行不会自动变成哈希**，这些旧用户将无法登录。

本阶段是开发环境，最简做法是把 `users` 表清空重来：

```sql
TRUNCATE users;
```

但要写下来：生产环境不能这么干。生产上通常有三条路，思考一下各自的适用场景：

1. **一次性迁移脚本**：用程序读出所有明文，逐行哈希后写回，在低峰期执行一次。
2. **惰性升级**：登录时若发现存储值不是 `$2a$` 开头（bcrypt 前缀），先用明文校验一次，成功后立刻重写成哈希。
3. **强制重置**：直接让所有用户走「忘记密码」流程，把旧密码作废。

本阶段只要求你理解「代码改了 ≠ 数据改了」，并在开发库里用 `TRUNCATE` 走通全流程。

### 步骤 7：验证

```powershell
gofmt -w api
git diff --check
go build ./...
go test ./...
```

集成测试需要先配置测试库（`STREAMHUB_INTEGRATION_TEST=1` 且 `MYSQL_DB=streamhub_test`），否则会跳过——这一点阶段 0 已经约定。

手工验证登录链路：

```powershell
# 注册
Invoke-RestMethod -Method Post -Uri http://localhost:8080/user -ContentType application/json -Body '{"user_name":"alice","pwd":"s3cret"}'
# 登录（应返回 200 和 session_id）
Invoke-RestMethod -Method Post -Uri http://localhost:8080/user/login -ContentType application/json -Body '{"user_name":"alice","pwd":"s3cret"}'
# 登录（错误密码，应返回 401）
Invoke-RestMethod -Method Post -Uri http://localhost:8080/user/login -ContentType application/json -Body '{"user_name":"alice","pwd":"wrong"}'
```

最后直接查库确认落库的不是明文：

```sql
SELECT login_name, pwd FROM users;
```

`pwd` 应该是 `$2a$10$...` 开头的 60 字符字符串，而不是 `s3cret`。

## 7. 验收标准

- [ ] 阶段 3 已单独提交，本阶段改动与阶段 3 可区分。
- [ ] `AddCredential` 在数据访问层完成 bcrypt 哈希，handler 不负责哈希。
- [ ] `Login` 用 `bcrypt.CompareHashAndPassword` 校验，不再做明文 `!=` 比较。
- [ ] 用户不存在与密码错误返回同一条消息。
- [ ] 库里 `users.pwd` 是 `$2a$` 开头的哈希，不包含明文密码。
- [ ] `DeleteCredential` 要么删除、要么去掉密码参数，不存在带明文参数的死代码。
- [ ] 集成测试断言「哈希不等于明文」且「正确密码能通过校验」。
- [ ] `go build ./...` 和 `go test ./...` 通过。
- [ ] 你能解释：为什么「改了代码」之后还需要单独处理「已有数据」。

## 8. 提交建议

本阶段改动集中，可以一个提交收尾：

```powershell
git add api/dbops/api.go api/handlers.go api/dbops/api_test.go docs/stage-4-password-hashing.md
git commit -m "feat: hash user passwords with bcrypt"
```

如果你保留了 `DeleteCredential` 的简化版，把它放进同一个提交；如果删除了，在提交说明里写明「删除无业务调用方的 DeleteCredential」。

## 9. 拓展思考题

1. 为什么说「MD5/SHA-256 快」是它们不适合做密码哈希的原因，而不是优点？
2. 如果两个用户用了完全相同的密码，bcrypt 存储的两个哈希会相同吗？为什么？盐在这个问题里起什么作用？
3. `bcrypt.GenerateFromPassword` 每次调用结果都不同，那 `CompareHashAndPassword` 是怎么做到「知道盐」的？（提示：看 60 字符哈希里都包含了什么）
4. 为什么哈希应该放在 `dbops` 层而不是 `handler` 层？如果以后有第二个入口也创建用户，放在 handler 层会有什么风险？
5. 生产环境里，已有 10 万明文用户，你会选步骤 6 里三条迁移路径的哪一条？理由是什么？

## 10. 后续路线导航

> 这是「渐进式改造」的活地图。每完成一个阶段，就把下面表格里的对应行勾掉，并在这个文档或 `progressive-refactor-guide.md` 里更新「当前状态」。

| 阶段 | 单一学习目标 | 核心问题 |
| --- | --- | --- |
| 5 | 会话生命周期 | `RetrieveSession` 查错列、`IsSessionValid` 逻辑反了、启动不加载历史 session、`TTL VARCHAR(8)` 装不下毫秒时间戳 |
| 6 | 输入与路径安全 | 阻断 `streamsever` 的 `../` 路径穿越，理解「永不信任外部输入」 |
| 7 | 显式依赖 | 移除 `dbops` 的包级全局 `dbConnection`，改为构造时传入 |
| 8 | 跨服务调用可靠性 | 封装 scheduler client，加超时与状态码校验 |
| 9 | 调度器并发模型 | 修 channel 状态机、错误传播，用确定性测试替代 `Sleep` |
| 10 | 数据模型与约束 | 统一 `Id`→`ID` 命名、时间列类型、外键与索引 |
| 11 | 工程收尾 | README、健康检查、优雅停机、部署准备 |

每个阶段只开启一个，完成后先 `gofmt`、`go build ./...`、`go test ./...` 三件套验收，再提交，再开下一个。
