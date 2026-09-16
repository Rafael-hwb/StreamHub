# 阶段 5：会话生命周期 —— 修复持久化、过期与错误传播

> 改造范围：`api/session`、`api/dbops/internal.go`、`api/handlers.go`（生成 session 的两处）、`api/main.go`、`schema.sql`。
> 前置：阶段 4（密码哈希）已提交。

## 1. 学习目标

理解「会话是有生命周期的状态」：它会被创建、被读取、会过期、会持久化、会清理。每个环节都要有人负责，错误不能被吞掉，过期状态不能无限堆积。

## 2. 现状与问题

先自己读 `api/session/ops.go` 和 `api/dbops/internal.go`，对照下表逐条定位：

| # | 问题 | 位置 | 后果 |
| --- | --- | --- | --- |
| 1 | `RetrieveSession` 查 `user_name`，表里列名是 `login_name` | [api/dbops/internal.go:27](api/dbops/internal.go:27) | SQL 运行时必错 |
| 2 | `LoadSessionsFromDB` 从未被调用 | [api/session/ops.go:23](api/session/ops.go:23) | 重启后 session 全丢 |
| 3 | `IsSessionValid` 逻辑反了：过期不删、不存在反而删 | [api/session/ops.go:45](api/session/ops.go:45) | 过期 session 永久滞留 |
| 4 | `sessions.TTL` 是 `VARCHAR(8)`，装不下 13 位毫秒时间戳 | [schema.sql:28](schema.sql:28) | 写库截断或失败 |
| 5 | `GenerateSessionId` 吞掉 UUID 和写库两个错误 | [api/session/ops.go:35](api/session/ops.go:35) | 失败被静默 |
| 6 | 无过期清理 | [api/session/ops.go](api/session/ops.go) | 内存与 DB 里的过期 session 只增不减 |

## 3. 分步骤改法

### 步骤 1：修 `RetrieveSession` 的列名（问题 1）

`api/dbops/internal.go` 里：

```go
stmtOut, err := dbConnection.Prepare("SELECT user_name, TTL FROM sessions WHERE session_id=?")
```

改成：

```go
stmtOut, err := dbConnection.Prepare("SELECT login_name, TTL FROM sessions WHERE session_id=?")
```

注意：`RetrieveSession` 当前没有调用方（`IsSessionValid` 走的是内存 map，`LoadSessionsFromDB` 走的是 `RetrieveAllSessions`）。先把它修正确，后面它作为「按 id 单查」的语义才可信；如果你确认永远用不到，也可以删掉，但要写下理由。

### 步骤 2：把 TTL 换成 `BIGINT`（问题 4）

先改 `schema.sql`：

```sql
CREATE TABLE IF NOT EXISTS sessions (
  session_id VARCHAR(64) PRIMARY KEY,
  TTL BIGINT NOT NULL,
  login_name VARCHAR(64) NOT NULL
);
```

> 这是 schema 改动。阶段 10 才引入 `golang-migrate`，所以现在先手动改 `schema.sql`，本地库重建（或 `ALTER TABLE sessions MODIFY TTL BIGINT`）。到阶段 10 再把这笔改动固化进第一个迁移文件。

`api/dbops/internal.go` 里去掉字符串转换。`InsertSession`：

```go
func InsertSession(sid string, TTL int64, username string) error {
	stmtIn, err := dbConnection.Prepare("INSERT INTO sessions (session_id, TTL, login_name) VALUES (?,?,?)")
	if err != nil {
		return err
	}
	defer stmtIn.Close()

	_, err = stmtIn.Exec(sid, TTL, username)
	if err != nil {
		return err
	}

	return nil
}
```

删掉 `strconv.FormatInt` 那行，`Exec` 直接传 `TTL`（int64）。

`RetrieveSession` 里去掉 `ParseInt`，直接 `Scan` 进 `int64`；`RetrieveAllSessions` 同理，`Scan` 直接扫 `var TTL int64`，删掉 `strconv.ParseInt` 分支。

### 步骤 3：修 `IsSessionValid` 的三分支逻辑（问题 3）

当前逻辑把「过期」和「不存在」搞混了。正确语义是三分支：

```go
func IsSessionValid(sid string) (string, bool) {
	v, ok := sessionMap.Load(sid)
	if !ok {
		return "", false // 不存在：直接无效，什么都不用做
	}

	s := v.(*defs.SimpleSession)
	if time.Now().UnixMilli() < s.TTL {
		return s.UserName, true // 有效
	}

	DeleteSession(sid) // 存在但过期：删掉，防止堆积
	return "", false
}
```

同时把裸类型断言 `perSimpleSession.(*defs.SimpleSession)` 改成上面这种带 `ok` 的写法，避免潜在 panic。

### 步骤 4：错误不再被吞（问题 5）

`GenerateSessionId` 当前 `sid, _ := utils.NewUUID()` 且 `dbops.InsertSession` 的错误被丢。改成返回错误：

```go
const sessionTTL = 30 * 60 * 1000 // 30 分钟，单位毫秒

func GenerateSessionId(username string) (string, error) {
	sid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}

	ttl := time.Now().UnixMilli() + sessionTTL
	s := &defs.SimpleSession{UserName: username, TTL: ttl}
	sessionMap.Store(sid, s)

	if err := dbops.InsertSession(sid, ttl, username); err != nil {
		return "", err
	}
	return sid, nil
}
```

两个调用方同步改。`api/handlers.go` 的 `CreateUser`：

```go
sid, err := session.GenerateSessionId(userBody.UserName)
if err != nil {
	context.Error(errs.Internal(err))
	return
}
```

`Login` 里同样处理。

### 步骤 5：启动时恢复会话（问题 2）

`LoadSessionsFromDB` 现在吞错误且返回空。改成返回 `error`：

```go
func LoadSessionsFromDB() error {
	r, err := dbops.RetrieveAllSessions()
	if err != nil {
		return err
	}
	r.Range(func(k, v any) bool {
		s := v.(*defs.SimpleSession)
		sessionMap.Store(k, s)
		return true
	})
	return nil
}
```

在 `api/main.go` 的 `main()` 里，`dbops.Init` 之后调用：

```go
if err := session.LoadSessionsFromDB(); err != nil {
	log.Printf("warning: load sessions from db: %v", err)
}
```

启动时恢复失败用 `warning` 而不是 `Fatal`——服务不该因为「拿不到历史会话」就起不来，但必须记录。

### 步骤 6：过期清理（问题 6，最小版）

步骤 3 已经让「过期 session 在被访问到时被删除」。这满足了「不无限堆积」的最低要求。独立的定时全表清理暂不引入（那属于 P6 的异步任务范畴），现在不要为了清理而过早造一个调度器。

## 4. 验收标准

- [ ] `RetrieveSession` 查询 `login_name`，不再查 `user_name`。
- [ ] `sessions.TTL` 是 `BIGINT`，写读全程无字符串转换。
- [ ] 登录后重启 `api`，之前的 session 仍有效（启动恢复生效）。
- [ ] 过期 session 访问受保护接口返回 401，且被从内存删除。
- [ ] `GenerateSessionId` 的错误能传播到 handler 并转成 500。
- [ ] 三件套通过：`gofmt` / `go build ./...` / `go test ./...`。

## 5. 提交建议

```powershell
git add api/session/ops.go api/dbops/internal.go api/handlers.go api/main.go schema.sql
git commit -m "fix: repair session persistence, expiry and error propagation"
```

## 6. 拓展思考题

1. `LoadSessionsFromDB` 从 DB 恢复会话，和「重启后要求用户重新登录」相比，各有什么取舍？什么场景下后者更安全？
2. 为什么启动恢复失败用 `warning` 而不是 `Fatal`？换成 `Fatal` 会发生什么？
3. 内存 map 和 DB 各存了一份 session，这两个数据源会不会不一致？什么情况下会不一致？
4. 裸类型断言 `.(*defs.SimpleSession)` 什么时候会 panic？带 `ok` 的写法是怎么防住的？
