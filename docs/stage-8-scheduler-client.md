# 阶段 8：跨服务调用可靠性 —— 封装 scheduler client

> 改造范围：新增 `api` 下的 scheduler 客户端、`internal/config`（加调度器地址）、`api/handlers.go`（`DeleteVideoHandler`）、`api/main.go`。
> 前置：阶段 7（显式依赖）已提交。

## 1. 学习目标

理解「调用另一个服务」和「调用本地函数」的本质区别：网络会失败、会超时、会返回非 200 的状态码。这些都必须显式处理，地址不能硬编码，调用要带超时，失败要如实向上返回。

## 2. 现状与问题

[api/handlers.go](api/handlers.go) 的 `DeleteVideoHandler` 里：

```go
resp, err := http.Get("http://localhost:9001/video-del-rec/" + vid)
if err != nil {
	log.Printf("Notify scheduler error: %v", err)
} else {
	resp.Body.Close()
}
// 无论如何都返回成功
```

四个问题：

| # | 问题 | 后果 |
| --- | --- | --- |
| 1 | 无超时 | scheduler 挂起时，请求会无限等待 |
| 2 | 地址硬编码 `localhost:9001` | 换环境 / 容器化后地址变了就要改代码 |
| 3 | 不检查 HTTP 状态码 | scheduler 返回 500，这里也当成功 |
| 4 | 失败仍返回成功 | 元数据删了、文件没删，产生孤儿文件 |

## 3. 分步骤改法

### 步骤 1：把调度器地址加进配置

`internal/config/config.go` 的 `Config` 结构加一项：

```go
type Config struct {
	MySQL        MySQL
	SchedulerURL string
}

func Load() (*Config, error) {
	mysql, err := loadMySQL()
	if err != nil {
		return nil, err
	}
	return &Config{
		MySQL:        mysql,
		SchedulerURL: envOr("SCHEDULER_URL", "http://localhost:9001"),
	}, nil
}
```

`.env` 里加一行 `SCHEDULER_URL=http://localhost:9001`。

### 步骤 2：封装一个 scheduler client

新建 `api/schedulerclient/client.go`（独立小包，方便复用与测试）：

```go
package schedulerclient

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) NotifyVideoDeleted(ctx context.Context, vid string) error {
	url := c.baseURL + "/video-del-rec/" + vid
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("notify scheduler: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scheduler returned %d", resp.StatusCode)
	}
	return nil
}
```

关键点：`http.NewRequestWithContext` 让调用能随 `ctx` 取消；`http.Client{Timeout}` 是硬超时；状态码非 200 返回错误而不是静默。

### 步骤 3：注入 client，改 `DeleteVideoHandler`

`Handler` 增加字段：

```go
type Handler struct {
	store    *dbops.Store
	scheduler *schedulerclient.Client
}

func NewHandler(store *dbops.Store, scheduler *schedulerclient.Client) *Handler {
	return &Handler{store: store, scheduler: scheduler}
}
```

`DeleteVideoHandler` 里替换那段裸 `http.Get`：

```go
if err := h.scheduler.NotifyVideoDeleted(context.Request.Context(), vid); err != nil {
	log.Printf("notify scheduler: %v", err)
	context.Error(errs.Internal(fmt.Errorf("notify scheduler: %w", err)))
	return
}
```

用 `context.Request.Context()` 传递请求上下文，`httpx.Success` 只在通知成功后才返回。

### 步骤 4：装配

`api/main.go`：

```go
db, err := dbconn.Open(cfg.MySQL)
if err != nil {
	log.Fatalf("open db: %v", err)
}
store := dbops.NewStore(db)
scheduler := schedulerclient.NewClient(cfg.SchedulerURL)
h := NewHandler(store, scheduler)
router := RegisterHandlers(h)
```

## 4. 语义决策（本阶段只记录，不硬做）

删除已经删掉了元数据，但通知 scheduler 失败——此时「元数据已删、文件还在」是跨服务一致性的经典难题。本阶段先做到**如实返回**：通知失败返回 500 并记录，而不是假装成功。

真正的可靠投递（任务写进 outbox、消费失败重试、保证至少一次投递）留给 **P6（异步任务与并发健壮性）**，那是专门解决这个问题的阶段。现在不要提前引入消息队列或 outbox，否则 P1 会膨胀成一个无法验收的怪物。

## 5. 验收标准

- [ ] scheduler 调用有超时（`http.Client.Timeout`）。
- [ ] 地址来自配置 `SCHEDULER_URL`，无硬编码 `localhost:9001`。
- [ ] 非 200 状态码被当作错误返回。
- [ ] 通知失败时 `DeleteVideoHandler` 返回 500，不再假装成功。
- [ ] 请求使用 `context.Request.Context()`，可随请求取消。
- [ ] 新增 `Client` 单测：用 `httptest.Server` 模拟 200 / 500 / 超时三种情况。
- [ ] 三件套通过。

## 6. 提交建议

```powershell
git add internal/config/config.go api/schedulerclient/client.go api/handlers.go api/main.go .env.example
git commit -m "feat: add scheduler client with timeout and status check"
```

> 顺带：如果仓库还没有 `.env.example`，现在补一个（只含键名，不含真实密码），`.env` 本身仍在 `.gitignore` 里。

## 7. 拓展思考题

1. `http.Client{Timeout}` 的超时覆盖了整个请求生命周期，还是只覆盖连接建立？如果只想限制「读响应体」的时间，该怎么办？
2. `http.NewRequestWithContext(ctx, ...)` 和直接 `http.Get` 的区别是什么？`ctx` 取消后请求会怎样？
3. 删除元数据成功、通知失败，此时返回 500，客户端重试会怎样？这个接口具备幂等性吗？
4. 为什么「通知失败如实返回 500」比「假装成功」更好，但依然不完美？完美方案需要什么（提示：P6 的 outbox）？
