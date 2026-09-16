# 阶段 9：调度器并发模型 —— 修 channel 状态机、错误传播与确定性测试

> 改造范围：`scheduler/taskrunner/*`（`runner.go`、`task.go`、`tsmain.go`、`runner_test.go`）。
> 前置：阶段 7（显式依赖）已提交，`taskrunner` 已能拿到 `*dbops.Store`。

## 1. 学习目标

理解 channel 状态机、错误传播、goroutine 生命周期这三件事，以及「确定性测试」为什么不能靠 `time.Sleep`。做完后，调度器从「一个跑不起来的 channel 状态机 + 靠 Sleep 的测试」变成「一个 ticker 驱动的 worker 循环，错误能汇总返回，测试确定」。

## 2. 现状与问题

先读 `scheduler/taskrunner/` 下四个文件，对照下表：

| # | 问题 | 位置 | 后果 |
| --- | --- | --- | --- |
| 1 | `StartDispatch` 用两个独立 `if` 判断状态，不是 `else if` | [runner.go:34-52](scheduler/taskrunner/runner.go) | 读者无法确认两个分支互斥，脆弱 |
| 2 | `Start()` 只 `go r.StartDispatch()`，从不发 `READY_TO_DISPATCH`，`Worker` 也从未被使用 | [tsmain.go:28-31](scheduler/taskrunner/tsmain.go) | **调度器从未真正运行过** |
| 3 | executor 里 goroutine 忘了 `wg.Done()`，`wg.Wait()` 永远阻塞 | [task.go:38-63](scheduler/taskrunner/task.go) | 死锁 |
| 4 | `DeleteVideo` 吞掉 `os.Remove` 错误，总是返回 `nil` | [task.go:11-19](scheduler/taskrunner/task.go) | 错误永远不被发现 |
| 5 | `CreateNewWorker` 的 `interval * time.Second` 单位隐晦 | [tsmain.go:12-17](scheduler/taskrunner/tsmain.go) | 传参容易写错 |
| 6 | 测试靠 `time.Sleep(3s)` 等结果 | [runner_test.go:35](scheduler/taskrunner/runner_test.go) | 慢且不稳定 |

其中 #2 是致命的：`Start()` 创建了 runner 却没有任何东西往 `Controller` 发信号，`StartDispatch` 永久阻塞在 `select` 上，`Worker.StartWorker` 也从来没有被调用。也就是说，**视频删除的定时清理实际上从未跑起来过**（阶段 6 之前你可能一直没注意到，因为删除链路里「通知 scheduler 写记录」和「真正删文件」是解耦的）。

## 3. 分步骤改法

### 步骤 1：换掉 channel 状态机，改成一个简单的「拉一批 → 处理一批」循环

原来的 dispatcher/executor 本质是顺序的「读一批 → 删一批」，不需要三个字符串在 channel 里来回传。直接拉平成：

```go
// scheduler/taskrunner/runner.go
package taskrunner

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type Runner struct {
	store      Store            // 窄接口，见步骤 3
	batchSize  int
	processOne func(ctx context.Context, id string) error
}

func NewRunner(store Store, batchSize int) *Runner {
	return &Runner{
		store:      store,
		batchSize:  batchSize,
		processOne: processVideoDeletion,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	ids, err := r.store.ReadVideoDeletionRecord(ctx, r.batchSize)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)
	for _, id := range ids {
		id := id // 捕获循环变量
		g.Go(func() error {
			return r.processOne(ctx, id)
		})
	}
	return g.Wait()
}
```

要点：`errgroup` 帮你管理 `WaitGroup` 和第一个错误的收集，不用手写 `sync.Map` + `firstErr`；`g.Go` 里的 goroutine 生命周期由 `errgroup` 负责，不存在漏 `Done()` 的问题。

### 步骤 2：`processOne` 做幂等的「删文件 + 删记录」

```go
// scheduler/taskrunner/task.go
func processVideoDeletion(ctx context.Context, store Store, vid string) error {
	if !validVideoID(vid) { // 阶段 6 引入的校验
		return fmt.Errorf("invalid video id %q", vid)
	}
	if err := os.Remove(VIDEO_PATH + vid); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", vid, err)
	}
	if err := store.DeleteVideoDeletionRecord(ctx, vid); err != nil {
		return fmt.Errorf("delete record %s: %w", vid, err)
	}
	return nil
}
```

注意 `os.IsNotExist` 是可以接受的（文件已经被删过了，幂等），其他错误要如实返回。这一步同时修掉 #4。

### 步骤 3：定义窄接口，解耦对 `dbops.Store` 的依赖

`Runner` 只依赖「读删除记录」和「删记录」两个能力，不依赖整个 `dbops.Store`：

```go
type Store interface {
	ReadVideoDeletionRecord(ctx context.Context, count int) ([]string, error)
	DeleteVideoDeletionRecord(ctx context.Context, vid string) error
}
```

`*dbops.Store` 只要方法签名匹配，就自动满足这个接口（Go 的结构化接口）。这让 `Runner` 的测试可以注入一个假的 Store。

> 这需要给 `dbops.Store` 的两个方法加上 `ctx` 参数（阶段 7 之后你已经在逐步引入 context）。如果还没加，这一步一并补上。

### 步骤 4：让 worker 真正跑起来

```go
// scheduler/taskrunner/tsmain.go
func Start(ctx context.Context, store Store, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runner := NewRunner(store, batchSize)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := runner.Run(ctx); err != nil {
				log.Printf("runner: %v", err)
			}
		}
	}
}
```

`interval` 现在就是 `time.Duration`，调用方写 `taskrunner.Start(ctx, store, 10*time.Second)`，单位一目了然，修掉 #5。

`scheduler/main.go` 里：

```go
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
defer stop()

go taskrunner.Start(ctx, store, 10*time.Second)

// 优雅停机：见阶段 11，这里先让 ctx 能传给 worker
```

### 步骤 5：确定性测试

不再用 `Sleep`。用 `Runner` 的窄接口注入假 Store，直接调用 `Run` 断言结果：

```go
type fakeStore struct {
	ids     []string
	deleted []string
	err     error
}

func (f *fakeStore) ReadVideoDeletionRecord(ctx context.Context, count int) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ids, nil
}

func (f *fakeStore) DeleteVideoDeletionRecord(ctx context.Context, vid string) error {
	f.deleted = append(f.deleted, vid)
	return nil
}

func TestRunDeletesAll(t *testing.T) {
	fs := &fakeStore{ids: []string{"a", "b", "c"}}
	r := NewRunner(fs, 10)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fs.deleted) != 3 {
		t.Fatalf("expected 3 deletions, got %d", len(fs.deleted))
	}
}
```

「删文件」这个动作需要真实文件系统，把它隔离到 `processVideoDeletion` 内部；`Run` 的单测通过注入 `processOne` 或用假 Store 覆盖「记录删除」这条路径，不碰真实磁盘。

## 4. 验收标准

- [ ] `rg "READY_TO_DISPATCH|READY_TO_EXECUTE|ControlChannel" scheduler` 无结果（旧状态机已删）。
- [ ] `Start(ctx, store, interval)` 被 `scheduler/main.go` 真正调用，worker 会周期性执行。
- [ ] 并发错误通过 `errgroup` 汇总返回，不被 goroutine 吞掉。
- [ ] `DeleteVideo` 如实返回 `os.Remove` 错误（除「文件不存在」）。
- [ ] 测试不依赖 `time.Sleep`，是确定性的（注入假 Store 直接调 `Run`）。
- [ ] worker 收到 `ctx.Done()` 能退出（优雅停机的雏形）。
- [ ] 三件套通过。

## 5. 提交建议

```powershell
go get golang.org/x/sync/errgroup
git add scheduler/taskrunner scheduler/main.go go.mod go.sum
git commit -m "refactor: replace scheduler channel state machine with errgroup worker"
```

## 6. 拓展思考题

1. 原来的 `Start()` 为什么「从未真正运行」？trace 一下：谁往 `Controller` 发信号了？
2. `errgroup.WithContext` 在某个 goroutine 返回错误后，会对其他 goroutine 做什么？它怎么保证「第一个错误」被返回？
3. 循环变量捕获 `id := id` 是干什么的？去掉它会发生什么（Go 1.22 之后还必要吗）？
4. `processVideoDeletion` 里「文件已不存在」算成功，这体现了什么性质？为什么这个性质对重试很重要（提示：P6 的 outbox 会用到）？
