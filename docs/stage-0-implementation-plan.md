# 阶段 0 具体实施方案：恢复可信编译基线

> 适用项目：StreamHub  
> 本阶段只解决“当前改动无法完整编译和测试反馈不清晰”。不重构业务、不修改接口契约、不处理密码与路径安全。

## 1. 完成定义

阶段 0 完成时，应同时满足：

1. API、scheduler、streamsever 三个程序包均可编译。
2. 当前错误处理中间件代码保留，但尚未迁移的旧 handler 仍可通过临时兼容层工作。
3. 不依赖 MySQL 的测试可以独立执行。
4. 依赖 MySQL 的测试在环境未准备好时不能发生 nil pointer panic。
5. 所有修改可以拆成 3～4 个容易审查的小提交。

## 2. 本阶段允许和禁止的修改

### 允许修改

- `api/auth.go`
- `scheduler/main.go`
- 恢复 `api/response.go`
- 恢复 `scheduler/response.go`
- 恢复 `streamsever/response.go`
- 对上述文件执行 `gofmt`
- 最小调整 `api/dbops/api_test.go`，使测试依赖缺失时给出清晰结果
- 选做：README 增加本地检查命令

### 禁止顺手修改

- 不迁移 `api/handlers.go` 中全部错误响应。
- 不修改前端接口或 JSON 格式。
- 不引入 service/repository 等新分层。
- 不修改数据库表结构。
- 不处理明文密码和视频路径问题。
- 不升级 Go 或第三方依赖。

这些问题并非不重要，而是需要各自独立阶段，避免一次修改过多变量。

## 3. 开始前：建立现场快照

在 PowerShell 中进入项目目录：

```powershell
Set-Location C:\Users\Rafael\StreamHub
git status --short
git diff --stat
git diff -- api/auth.go api/main.go scheduler/main.go streamsever/main.go
```

把输出保存到你的学习笔记。当前预期能看到：

- `api/auth.go`、三个 `main.go` 等文件被修改；
- 三个 `response.go` 被删除；
- `internal/httpx` 和迁移文档属于新增内容。

检查敏感文件没有被跟踪：

```powershell
git ls-files .env
```

预期：没有输出。若出现 `.env`，立即停止，不要提交，交给导师处理。

## 4. 实施步骤 A：修复 API 语法错误

### 4.1 先理解错误

当前代码：

```go
context.Error(err error)
```

`err error` 只能出现在参数声明位置，例如 `func f(err error)`。调用函数时必须传入一个已经构造好的错误值。

### 4.2 修改目标

在 `api/auth.go` 中：

1. 移除已经不需要的 `api/defs` 导入。
2. 导入 `internal/errs`。
3. 用户名为空时，构造“未认证”应用错误，并传给 `context.Error`。
4. 保留 `return false`，避免函数继续执行。

请自己完成调用表达式。完成后回答：这里应该使用 `Unauthorized` 还是 `Forbidden`？为什么？

### 4.3 格式化和局部验证

```powershell
gofmt -w api/auth.go
go test ./api
```

此时 `go test ./api` 仍可能报告 `SendErrorResponse` 未定义。这属于下一步要恢复的兼容层，不代表步骤 A 失败。

步骤 A 的通过标准是：输出中不再包含：

```text
syntax error: unexpected name error
```

### 4.4 建议提交点

```powershell
git add api/auth.go
git diff --cached
git commit -m "fix: pass authentication error to gin context"
```

提交不是强制要求；若暂不提交，至少在继续前查看一次 `git diff -- api/auth.go`。

## 5. 实施步骤 B：修正 scheduler 数据库包

### 5.1 先理解问题

目前存在两个不同的包：

```text
api/dbops        -> 持有 API 自己的 dbConnection
scheduler/dbops  -> 持有 scheduler 自己的 dbConnection
```

包名都叫 `dbops` 不代表它们共享变量。scheduler handler 调用 `scheduler/dbops`，main 却初始化 `api/dbops`，因此真正被 handler 使用的连接仍是 nil。

### 5.2 修改目标

在 `scheduler/main.go` 中，仅把 dbops 导入路径切换为 scheduler 自己的包。初始化调用本身不需要改名。

### 5.3 格式化和局部验证

```powershell
gofmt -w scheduler/main.go
go test ./scheduler
```

此时仍可能只剩旧响应 helper 缺失错误。确认输出中没有导包冲突或 `dbops.Init` 未定义。

### 5.4 建议提交点

```powershell
git add scheduler/main.go
git diff --cached
git commit -m "fix: initialize scheduler database package"
```

## 6. 实施步骤 C：恢复临时响应兼容层

### 6.1 为什么不马上迁移全部 handler

当前三个服务有约 40 个旧 helper 调用。如果同时迁移，任何响应状态、JSON 字段或控制流错误都会混在一起。本阶段先恢复原 helper，使仓库重新可编译；下一阶段只迁移 API。

兼容期间遵守一条规则：一个错误分支只能选择一种响应方式。

```text
旧分支：SendErrorResponse 后 return
新分支：context.Error 后 return，由 ErrorHandler 输出
```

不要在同一分支中两者都调用，否则可能重复写响应头或拼出两个 JSON 响应。

### 6.2 从 Git 恢复三个文件

先确认 Git 中确实存在旧版本：

```powershell
git show HEAD:api/response.go
git show HEAD:scheduler/response.go
git show HEAD:streamsever/response.go
```

确认内容正确后，恢复这三个明确文件：

```powershell
git restore --source=HEAD -- api/response.go scheduler/response.go streamsever/response.go
```

这条命令只恢复列出的三个文件，不会覆盖其他未提交修改。

### 6.3 格式化和编译

```powershell
gofmt -w api/response.go scheduler/response.go streamsever/response.go
go build ./...
```

预期：不再出现 `undefined: SendErrorResponse` 或 `undefined: SendNormalResponse`。

如果编译仍失败，按“第一个编译错误”处理，不要同时猜测后面所有错误。记录：文件、行号、错误文本、你认为的原因。

### 6.4 检查新旧机制边界

```powershell
rg -n "context\.Error|SendErrorResponse|SendNormalResponse" api scheduler streamsever
```

检查要点：

- 新 `ErrorHandler` 已挂载在三个 router。
- `ValidateUser` 使用新机制。
- 其余尚未迁移的 handler 使用旧 helper。
- 没有一个错误分支连续调用两种机制。

### 6.5 建议提交点

因为这些文件在 HEAD 中原本存在，恢复后它们通常不会显示为新 diff。此时提交中主要应是步骤 A、B 及此前新增的错误处理中间件。提交前务必检查：

```powershell
git status --short
git diff --check
git diff
```

不要使用 `git add .`。明确选择本阶段文件，避免把 `.env`、视频或二进制误加进去。

## 7. 实施步骤 D：隔离数据库集成测试

### 7.1 Bug 根源

生产程序会在 `main()` 中调用 `dbops.Init`，但执行 `go test ./api/dbops` 时不会运行 API 的 `main()`。因此测试包中的全局 `dbConnection` 初始值为 nil。

当前 `TestMain` 一开始调用 `clearTables()`，最终相当于在 nil 上调用：

```text
nil *sql.DB -> Exec -> panic
```

此外，`m.Run()` 的返回码应传给 `os.Exit`，否则测试失败状态可能没有被正确交给操作系统。

### 7.2 本阶段推荐方案：显式测试开关

使用一个明确的环境变量，例如 `STREAMHUB_INTEGRATION_TEST`，区分普通单测和会修改真实数据库的集成测试：

```text
未设置为 1
  -> 打印或记录“跳过 MySQL 集成测试”
  -> 不清表、不访问数据库

设置为 1
  -> 加载测试配置
  -> 初始化 dbops
  -> 确认连接成功后清表
  -> 执行测试
  -> 再次清理
  -> 用 os.Exit 返回测试退出码
```

注意：这里会 `TRUNCATE` 表，绝不能默认连接生产数据库。使用专门的测试库，例如 `streamhub_test`。

### 7.3 最小实现任务

你需要在 `api/dbops/api_test.go` 中完成以下逻辑，但不要引入新测试框架：

1. 检查集成测试开关。
2. 未开启时，在 `TestMain` 中直接以成功状态退出，或把具体测试改为统一 skip。
3. 开启时调用配置加载和 `Init`。
4. 初始化失败时输出清晰错误，并以非零状态退出。
5. 保存 `m.Run()` 的返回值，清理完成后 `os.Exit(code)`。
6. `clearTables` 返回 `error`，不要忽略每一次 `Exec` 的错误。

这里先不要求抽象测试数据库、事务回滚或 Docker。

### 7.4 验证命令

普通检查：

```powershell
go test ./api/dbops
```

预期：明确跳过或不运行 MySQL 集成测试，且绝不 panic。

准备好专用测试库后才运行：

```powershell
$env:STREAMHUB_INTEGRATION_TEST = "1"
$env:MYSQL_DB = "streamhub_test"
go test ./api/dbops -v
Remove-Item Env:STREAMHUB_INTEGRATION_TEST
Remove-Item Env:MYSQL_DB
```

执行前必须人工确认 `MYSQL_DB` 是测试库。不要把真实密码写进命令历史、源码或文档。

### 7.5 建议提交点

```powershell
git add api/dbops/api_test.go
git diff --cached
git commit -m "test: make database integration dependency explicit"
```

## 8. 最终验证矩阵

按以下顺序执行，遇到失败就在当前命令停下：

### 8.1 格式与静态差异

```powershell
gofmt -w api/auth.go scheduler/main.go api/response.go scheduler/response.go streamsever/response.go
git diff --check
```

预期：`git diff --check` 无输出。

### 8.2 编译

```powershell
go build ./...
```

预期：退出码为 0，无编译错误。

### 8.3 无外部依赖测试

```powershell
go test ./scheduler/taskrunner ./api/utils ./internal/...
```

预期：全部 `ok` 或 `[no test files]`。

### 8.4 普通全量测试

```powershell
go test ./...
```

预期：数据库集成测试没有开启时被明确隔离；整体不发生 panic。

### 8.5 残留检查

```powershell
rg -n "context\.Error\(err error\)" .
rg -n 'github.com/Rafael-hwb/streamhub/api/dbops' scheduler
git status --short
```

前两条预期无输出。最后一条只应出现你理解且准备提交的源码或文档变化。

## 9. 异常处理表

| 现象 | 可能原因 | 当前阶段处理方式 |
| --- | --- | --- |
| `SendErrorResponse` 未定义 | 兼容文件没有恢复或包名错误 | 核对三个 `response.go` 和各自 `package main` |
| scheduler 运行时 DB 为 nil | main 初始化了另一个 dbops 包 | 核对完整导入路径 |
| 测试出现 nil pointer panic | 测试没有显式调用 `Init` | 完成步骤 D，不要用 recover 掩盖 |
| MySQL access denied | 测试账号、密码或库名错误 | 停止集成测试，核对专用测试配置 |
| Go telemetry token access denied | 本机 Go 工具目录权限，与业务代码无关 | 单独记录，不要靠改项目源码规避 |
| `git diff --check` 报 whitespace | 文件格式或行尾问题 | 对 Go 文件执行 gofmt，再检查具体行 |
| 编译生成根目录 exe | Windows 的 `go build` 行为或构建命令指定输出 | 不提交二进制，确认 `.gitignore` 生效 |

## 10. 阶段 0 验收提交模板

完成后把以下内容发给导师：

```text
阶段 0 验收申请

1. 我修改了：
- ...

2. 我对问题根源的理解：
- auth.go 语法错误：...
- scheduler dbops 错包：...
- DB 测试 panic：...

3. 验证结果：
- go build ./...：通过/失败
- 无外部依赖测试：通过/失败
- go test ./...：通过/失败

4. 思考题：
- Unauthorized 与 Forbidden 的区别：...
- 为什么兼容层只能临时存在：...

5. 最不确定的地方：
- ...
```

导师会依次检查：修改范围、错误处理控制流、scheduler 依赖、测试退出码、命令输出。阶段 0 验收通过后，才开启阶段 1“仅迁移 API 的统一错误边界”。
