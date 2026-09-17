# 阶段 9：调度器并发模型 —— 修好 channel 状态机

> 改造范围：`scheduler/taskrunner/*`、`scheduler/dbops/*`、`scheduler/main.go`。
> 前置：阶段 7 已完成（`scheduler/dbops` 已迁移为 `Store`，但方法**还没有 `ctx` 参数**——本阶段第一步就是补上）。
>
> **本版路线**：保留「可复用 Runner 框架 + dispatcher/executor 状态机」的设计，逐条修复缺陷。不走「拉平成一个函数」的简化路线。

## 1. 学习目标

1. **channel 状态机**怎么才算是「接通」的——状态令牌谁发、谁收、什么时候结束。
2. **goroutine 生命周期**：WaitGroup 的 Add/Done 配对、errgroup 如何让「忘记 Done」变成结构性不可能、为什么「每 tick go 一个」是竞态。
3. **框架的接口设计**：为什么类型化 channel、为什么必须带 `ctx`、为什么扩展点是函数。

## 2. 现在这套代码有 8 处缺陷

先读一遍 `scheduler/taskrunner/` 下的 `defs.go`、`runner.go`、`task.go`、`tsmain.go`，对照下表：

| # | 缺陷 | 位置 | 后果 |
| --- | --- | --- | --- |
| 1 | `Start()` 调 `StartDispatch()`，从不发初始状态 | `tsmain.go:32` | **select 永久阻塞，调度器从未运行过** |
| 2 | executor 里 `wg.Add(1)` 后没有任何 `Done()` | `task.go:61` | `wg.Wait()` 永久阻塞 → 死锁 |
| 3 | 空批次返回 `error`，被当作 CLOSE 信号 | `task.go:40` | 队列一空，runner 永久退出 |
| 4 | 两个独立 `if` 判断状态，不是互斥分支 | `runner.go:37,46` | 无法确认互斥，加新状态就出错 |
| 5 | `DataChannel` 是 `chan interface{}` | `defs.go:5` | 处处 `id.(string)`，断言失败 panic |
| 6 | 全链路没有 `ctx` | `defs.go:7` | 不能取消、不能超时 |
| 7 | `Worker.ticker` 按值拷贝；每 tick `go` 一个新状态机 | `tsmain.go:10,25` | 重叠运行 → 竞态 |
| 8 | `dataSize` 与 dispatcher 取数条数是两个独立数字 | `tsmain.go:31` / `task.go:33` | 一旦不一致就永久死锁 |

下面按文件逐个改。每个文件都给出「**改前**」和「**改后**」完整代码，最后点出改了什么、为什么。

---

## 3. 逐文件改造

### 步骤 0：`scheduler/dbops` —— 方法补 `ctx`

缺陷 6 的前置。`Function` 要带 `ctx`，dispatcher/executor 调 store 的方法就必须能传 `ctx`。当前 dbops 方法还没有。

**改前** `scheduler/dbops/internal.go`：

```go
package dbops

import (
	"log"
)

func (s *Store) ReadVideoDeletionRecord(count int) ([]string, error) {
	var ids []string
	stmtOut, err := s.db.Prepare("SELECT video_id FROM video_del_rec LIMIT ?")
	if err != nil {
		return ids, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(count)
	if err != nil {
		log.Printf("Query VideoDeletionRecord failed: %v", err)
		return ids, err
	}

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Store) DeleteVideoDeletionRecord(vid string) error {
	stmtDel, err := s.db.Prepare("DELETE FROM video_del_rec WHERE video_id =? ")
	if err != nil {
		return err
	}

	defer stmtDel.Close()

	_, err = stmtDel.Exec(vid)
	if err != nil {
		log.Printf("Delete VideoDeletionRecord failed: %v", err)
		return err
	}

	return nil
}
```

**改后** `scheduler/dbops/internal.go`：

```go
package dbops

import (
	"context"
	"log"
)

func (s *Store) ReadVideoDeletionRecord(ctx context.Context, count int) ([]string, error) {
	var ids []string
	stmtOut, err := s.db.PrepareContext(ctx, "SELECT video_id FROM video_del_rec LIMIT ?")
	if err != nil {
		return ids, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.QueryContext(ctx, count)
	if err != nil {
		log.Printf("Query VideoDeletionRecord failed: %v", err)
		return ids, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) DeleteVideoDeletionRecord(ctx context.Context, vid string) error {
	stmtDel, err := s.db.PrepareContext(ctx, "DELETE FROM video_del_rec WHERE video_id =? ")
	if err != nil {
		return err
	}

	defer stmtDel.Close()

	_, err = stmtDel.ExecContext(ctx, vid)
	if err != nil {
		log.Printf("Delete VideoDeletionRecord failed: %v", err)
		return err
	}

	return nil
}
```

**改了什么、为什么**：
- `Prepare` → `PrepareContext`、`Query` → `QueryContext`、`Exec` → `ExecContext`：让数据库调用能被 `ctx` 取消。
- 补 `defer rows.Close()` 和 `rows.Err()`：顺带修掉资源泄漏，属于本文件早该有的收尾。

---

### 步骤 1：`defs.go` —— 类型化 channel + 状态 + ctx

缺陷 5 + 6。

**改前**：

```go
package taskrunner

type ControlChannel chan string

type DataChannel chan interface{}

type Function func(dc DataChannel) error

const (
	READY_TO_DISPATCH = "d"
	READY_TO_EXECUTE  = "e"
	CLOSE             = "c"

	VIDEO_PATH = "./videos/"
)
```

**改后**：

```go
package taskrunner

import "context"

const (
	VIDEO_PATH = "./videos/"

	// BatchSize 是每轮从库里取多少条删除记录，同时决定 Data channel 的缓冲大小。
	// 两个数字合成一个符号，见 runner.go 的 NewRunner。
	BatchSize = 3
)

// State 是状态机在 controller 上传递的状态令牌。
// 用具名类型取代裸字符串 "d"/"e"/"c"：非法状态在编译期就暴露。
type State string

const (
	StateDispatch State = "dispatch"
	StateExecute  State = "execute"
	StateClose    State = "close"
)

type ControlChannel chan State

// DataChannel 只承载视频 id。
// 原为 chan interface{}，每个使用点都要 id.(string)；
// 断言失败在运行时 panic，而这本可以是编译期错误。
type DataChannel chan string

// Function 是 Dispatcher / Executor 的统一签名。
// 加 ctx 是核心：框架必须能取消、能超时，否则装不下真实 IO 任务。
type Function func(ctx context.Context, dc DataChannel) error
```

**改了什么、为什么**：
- `DataChannel chan interface{}` → `chan string`：这一步会让旧代码里所有 `id.(string)` 立刻编译失败，**编译器替你找出所有要改的地方**。
- 状态 `"d"/"e"/"c"` → 具名类型 `State`：非法状态在编译期暴露，而不是运行时收到一个谁也不认识的值。
- `Function` 加 `ctx`：框架能取消、能超时。
- 新增 `BatchSize`：把「取几条」和「缓冲多大」两个数字收敛成一个（缺陷 8 的解药，步骤 2 用）。

---

### 步骤 2：`runner.go` —— 点火 + switch + 去 Error channel

缺陷 1 + 4 + 8。

**改前**：

```go
package taskrunner

type Runner struct {
	Controller ControlChannel
	Error      ControlChannel
	Data       DataChannel
	dataSize   int
	longLived  bool
	Dispatcher Function
	Executor   Function
}

func CreateNewRunner(dataSize int, longLived bool, d Function, e Function) *Runner {
	return &Runner{
		Controller: make(chan string, 1),
		Error:      make(chan string, 1),
		Data:       make(chan interface{}, dataSize),
		dataSize:   dataSize,
		longLived:  longLived,
		Dispatcher: d,
		Executor:   e,
	}
}

func (r *Runner) StartDispatch() {
	defer func() {
		if !r.longLived {
			close(r.Controller)
			close(r.Error)
			close(r.Data)
		}
	}()

	for {
		select {
		case c := <-r.Controller:
			if c == READY_TO_DISPATCH {
				err := r.Dispatcher(r.Data)
				if err != nil {
					r.Error <- CLOSE
				} else {
					r.Controller <- READY_TO_EXECUTE
				}
			}

			if c == READY_TO_EXECUTE {
				err := r.Executor(r.Data)
				if err != nil {
					r.Error <- CLOSE
				} else {
					r.Controller <- READY_TO_DISPATCH
				}
			}

		case e := <-r.Error:
			if e == CLOSE {
				return
			}
		}
	}
}

func (r *Runner) StartAll() {
	r.Controller <- READY_TO_DISPATCH
	r.StartDispatch()
}
```

**改后**：

```go
package taskrunner

import (
	"context"
	"fmt"
)

type Runner struct {
	controller ControlChannel
	data       DataChannel

	longLived bool

	dispatcher Function
	executor   Function
}

// NewRunner 创建一个可复用的任务 Runner。
//
//	longLived=false: 单次任务。Run 返回后关闭 channel。
//	longLived=true:  常驻任务。channel 保持打开，Run 可被反复调用（定时 Worker 用）。
func NewRunner(longLived bool, d, e Function) *Runner {
	return &Runner{
		controller: make(ControlChannel, 1),
		// 缓冲大小由 BatchSize 派生，不再单独传参（消除缺陷 8 的两个数字）。
		data:       make(DataChannel, BatchSize),
		longLived:  longLived,
		dispatcher: d,
		executor:   e,
	}
}

// Run 跑完整的一轮：dispatch → execute。longLived=true 时可反复调用。
func (r *Runner) Run(ctx context.Context) error {
	r.controller <- StateDispatch // 点火

	err := r.loop(ctx)

	if !r.longLived {
		close(r.controller)
		close(r.data)
	}
	return err
}

func (r *Runner) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case s := <-r.controller:
			switch s {
			case StateDispatch:
				if err := r.dispatcher(ctx, r.data); err != nil {
					return fmt.Errorf("dispatch: %w", err)
				}
				r.controller <- StateExecute

			case StateExecute:
				if err := r.executor(ctx, r.data); err != nil {
					return fmt.Errorf("execute: %w", err)
				}
				r.controller <- StateClose

			case StateClose:
				return nil
			}
		}
	}
}

// send 把 id 推进 Data channel。缓冲满时立刻返回错误而不是永久阻塞。
// 把配置错误从「静默挂死」变成「一条明确的错误」。
func send(ctx context.Context, dc DataChannel, id string) error {
	select {
	case dc <- id:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("data channel full (buffer=%d): dispatcher produced too many items", cap(dc))
	}
}
```

**改了什么、为什么**：

1. **`StartAll` 拆成 `Run` 的「点火 → loop」两段**：原 `Start()` 只调 `StartDispatch()`，从不发初始状态，`select` 永远等不到消息（缺陷 1）。`Run` 先 `r.controller <- StateDispatch` 点火，再进 `loop`。
2. **两个独立 `if` → `switch`**：互斥性由语法保证，读者一眼能确认（缺陷 4）。
3. **一轮结束发 `StateClose`、`loop` 返回**：原 `StartDispatch` 是死循环（dispatch→execute→dispatch…永不结束），这就是 Worker 必须 `go` 出去、进而重叠运行的根源。一轮一返回，周期交给 Worker 的 ticker。
4. **错误直接 `return`，不再塞 Error channel**：原来错误只有一个去处——塞 `"c"` 然后自杀，调用方永远不知道发生了什么。现在 `Run` 返回错误，调用方能处理。
5. **去掉 `Error` channel**：它的两个职责（报错、停机）分别被 `Run` 返回值和 `ctx` 接走了。保留它就和 `ctx` 重复，两个机制做同一件事是坏味道。
6. **`dataSize` 不再单独传参**：缓冲大小由 `BatchSize` 派生。原来 `CreateNewRunner(3, …)` 和 `ReadVideoDeletionRecord(3)` 是两个独立数字，一旦改成 5 就死锁（缺陷 8）。现在它们是一个符号。
7. **`send` 缓冲满即报错**：把「缓冲太小」从挂死变成明确错误。

---

### 步骤 3：`task.go` —— 空批次不报错 + errgroup + 窄接口

缺陷 2 + 3。同时把 `DeleteVideo` 合进 `deleteOne`，并用窄接口 `Store` 取代具体类型 `*dbops.Store`。

**改前**：

```go
package taskrunner

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sync"

	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validVideoID(vid string) bool {
	return uuidRe.MatchString(vid)
}

func DeleteVideo(vid string) error {
	if !validVideoID(vid) {
		return fmt.Errorf("invalid video id %q", vid)
	}

	if err := os.Remove(VIDEO_PATH + vid); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func VideoClearDispatcher(store *dbops.Store) Function {
	return func(dc DataChannel) error {
		ids, err := store.ReadVideoDeletionRecord(3)
		if err != nil {
			log.Printf("VideoClearDispatcher error: %v", err)
			return err
		}

		if len(ids) == 0 {
			return errors.New("VideoClearDispatcher is empty.")
		}

		for _, id := range ids {
			dc <- id
		}
		return nil
	}
}

func VideoClearExecutor(store *dbops.Store) Function {
	return func(dc DataChannel) error {
		errMap := &sync.Map{}
		var wg sync.WaitGroup
		var firstErr error

	forloop:
		for {
			select {
			case id := <-dc:
				wg.Add(1)
				go func(id interface{}) {
					if err := DeleteVideo(id.(string)); err != nil {
						errMap.Store(id, err)
						return
					}

					if err := store.DeleteVideoDeletionRecord(id.(string)); err != nil {
						errMap.Store(id, err)
						return
					}
				}(id)

			default:
				break forloop
			}
		}
		wg.Wait()

		errMap.Range(func(k, v interface{}) bool {
			if err := v.(error); err != nil && firstErr == nil {
				firstErr = err
			}
			return true
		})
		return firstErr
	}
}
```

**改后**：

```go
package taskrunner

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"golang.org/x/sync/errgroup"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validVideoID(vid string) bool {
	return uuidRe.MatchString(vid)
}

// Store 是 Runner 真正需要的能力，而不是整个 dbops.Store。
// Go 接口是隐式的：*dbops.Store 方法签名匹配就自动满足，无需声明。
// 接口定义在消费方，这是 Go 的惯例；也解决了「注入具体类型无法在测试里替换」的问题。
type Store interface {
	ReadVideoDeletionRecord(ctx context.Context, count int) ([]string, error)
	DeleteVideoDeletionRecord(ctx context.Context, vid string) error
}

func VideoClearDispatcher(store Store) Function {
	return func(ctx context.Context, dc DataChannel) error {
		ids, err := store.ReadVideoDeletionRecord(ctx, BatchSize)
		if err != nil {
			return fmt.Errorf("read deletion records: %w", err)
		}

		// 空批次是正常情况，不是错误。
		// 原代码在此返回 error，而 error 被当作 CLOSE 信号 → 队列一空 runner 永久退出。
		if len(ids) == 0 {
			return nil
		}

		for _, id := range ids {
			if err := send(ctx, dc, id); err != nil {
				return err
			}
		}
		return nil
	}
}

func VideoClearExecutor(store Store) Function {
	return func(ctx context.Context, dc DataChannel) error {
		g, ctx := errgroup.WithContext(ctx)

		for {
			select {
			case id := <-dc:
				// Go 1.22 起循环变量每轮独立，不再需要 id := id。
				g.Go(func() error {
					return deleteOne(ctx, store, id)
				})
			default:
				// Data 已空。安全：状态机保证 dispatch 先于 execute，
				// dispatcher 此时已完整跑完，不会再有新数据进来。
				return g.Wait()
			}
		}
	}
}

// deleteOne 做幂等的「删文件 + 删记录」。
func deleteOne(ctx context.Context, store Store, vid string) error {
	if !validVideoID(vid) {
		return fmt.Errorf("invalid video id %q", vid)
	}

	// 文件已不存在是可接受的结果（幂等），其他错误如实返回。
	if err := os.Remove(VIDEO_PATH + vid); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", vid, err)
	}

	if err := store.DeleteVideoDeletionRecord(ctx, vid); err != nil {
		return fmt.Errorf("delete record %s: %w", vid, err)
	}
	return nil
}
```

**改了什么、为什么**：

1. **空批次返回 `nil` 而不是 `error`**：原来的 `errors.New("VideoClearDispatcher is empty.")` 会触发 `r.Error <- CLOSE`，队列一空整个 runner 就永久退出（缺陷 3）。
2. **executor 改用 `errgroup`**：原代码 `wg.Add(1)` 后两个出口都没 `Done()`，`wg.Wait()` 永久阻塞（缺陷 2）。`g.Go(f)` 内部自己管 Add/Done，**你没有机会忘记**。
3. **`id.(string)` 消失**：`DataChannel` 已经是 `chan string`，断言整个删除。
4. **`DeleteVideo` 合进 `deleteOne`**：原来「删文件」和「删记录」分在两个函数里，`deleteOne` 把「一个视频的完整删除」做成一个原子动作，幂等且错误能带上下文。
5. **定义窄接口 `Store`**：取代具体类型 `*dbops.Store`，测试可以注入假实现（Go 只能 mock 接口，不能 mock 具体类型）。

---

### 步骤 4：`tsmain.go` —— Worker 定时驱动、不重叠、可停止

缺陷 7 + `interval * time.Second` 单位问题。

**改前**：

```go
package taskrunner

import (
	"time"

	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
)

type Worker struct {
	ticker time.Ticker
	runner Runner
}

func CreateNewWorker(interval time.Duration, runner Runner) *Worker {
	return &Worker{
		ticker: *time.NewTicker(interval * time.Second),
		runner: runner,
	}
}

func (w *Worker) StartWorker() {
	for {
		select {
		case <-w.ticker.C:
			go w.runner.StartAll()
		}
	}
}

func Start(store *dbops.Store) {
	r := CreateNewRunner(3, false, VideoClearDispatcher(store), VideoClearExecutor(store))
	go r.StartDispatch()
}
```

**改后**：

```go
package taskrunner

import (
	"context"
	"log"
	"time"
)

type Worker struct {
	ticker *time.Ticker // 指针：Ticker 持有运行时资源，按值拷贝是错的（go vet 会报）
	runner *Runner
}

func NewWorker(interval time.Duration, runner *Runner) *Worker {
	return &Worker{
		// interval 已经是 time.Duration，直接用，不再乘 time.Second。
		ticker: time.NewTicker(interval),
		runner: runner,
	}
}

// Start 阻塞运行，直到 ctx 被取消。
func (w *Worker) Start(ctx context.Context) {
	defer w.ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.ticker.C:
			// 顺序执行，不 go 出去。
			// 原代码每 tick go 一个新的 StartAll()：上一轮没跑完就叠加下一轮，
			// 两轮同时读写同一 Data channel 和同一批记录 —— 竞态。
			if err := w.runner.Run(ctx); err != nil {
				log.Printf("taskrunner: %v", err)
			}
		}
	}
}

func Start(ctx context.Context, store Store, interval time.Duration) {
	// longLived=true：channel 保持打开，Run 可以被反复调用。
	runner := NewRunner(true, VideoClearDispatcher(store), VideoClearExecutor(store))
	NewWorker(interval, runner).Start(ctx)
}
```

**改了什么、为什么**：

1. **`time.Ticker` → `*time.Ticker`**：Ticker 持有运行时资源，按值拷贝是错的。
2. **`defer w.ticker.Stop()`**：拿到需要释放的资源立刻 defer 释放，和 `defer resp.Body.Close()` 同理。
3. **不在 tick 里 `go`**：定时任务重叠运行是并发 bug 的经典来源。
4. **`ctx.Done()` 是唯一退出路径**：优雅停机的雏形。
5. **`interval` 不再乘 `time.Second`**：参数已经是 `time.Duration`，乘一次就把单位表达了两次。
6. **命名统一**：`CreateNewRunner`/`CreateNewWorker` → `NewRunner`/`NewWorker`，和 `dbops.NewStore`、`NewHandler` 一致。

---

### 步骤 5：`scheduler/main.go` —— 用 signal 建立 ctx

**改前**（相关部分）：

```go
func main() {
	// ...
	store := dbops.NewStore(db)

	go taskrunner.Start(store)

	router := RegisterRouter(store)
	router.Run(":9001")
}
```

**改后**：

```go
func main() {
	// ...
	store := dbops.NewStore(db)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go taskrunner.Start(ctx, store, 10*time.Second)

	router := RegisterRouter(store)
	router.Run(":9001")
}
```

需要补的 import：`context`、`os/signal`、`syscall`、`time`。

**改了什么、为什么**：`signal.NotifyContext` 让 Ctrl-C 时 `ctx` 被取消 → Worker 的 `select` 走 `ctx.Done()` → 干净退出，而不是被硬杀在删文件的中途。

---

### 步骤 6：`runner_test.go` —— 用假实现替换 Sleep

**改前**：

```go
package taskrunner

import (
	"errors"
	"log"
	"testing"
	"time"
)

func TestRunner(t *testing.T) {
	d := func(dc DataChannel) error {
		for i := 0; i < 30; i++ {
			dc <- i
			log.Printf("Dispatch send: %v", i)
		}
		return nil
	}

	e := func(dc DataChannel) error {
	forloop:
		for {
			select {
			case data := <-dc:
				log.Printf("Execute recieve: %v", data)

			default:
				break forloop
			}
		}
		return errors.New("execute")
	}

	runner := CreateNewRunner(30, false, d, e)
	go runner.StartAll()
	time.Sleep(3 * time.Second)
}
```

**改后**：

```go
package taskrunner

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu      sync.Mutex
	ids     []string
	deleted []string
	readErr error
}

func (f *fakeStore) ReadVideoDeletionRecord(ctx context.Context, count int) ([]string, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return f.ids, nil
}

func (f *fakeStore) DeleteVideoDeletionRecord(ctx context.Context, vid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, vid)
	return nil
}

func (f *fakeStore) deletedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.deleted)
}

// 缺陷 1 的回归测试：状态机必须真的点火。旧实现下这个测试会永远挂住。
func TestRunnerRunsOneCycle(t *testing.T) {
	var dispatched, executed int

	d := func(ctx context.Context, dc DataChannel) error {
		dispatched++
		return send(ctx, dc, "a")
	}
	e := func(ctx context.Context, dc DataChannel) error {
		for {
			select {
			case <-dc:
				executed++
			default:
				return nil
			}
		}
	}

	r := NewRunner(false, d, e)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("dispatcher ran %d times, want 1", dispatched)
	}
	if executed != 1 {
		t.Fatalf("executor consumed %d items, want 1", executed)
	}
}

// 缺陷 3：空批次不是错误。
func TestDispatcherEmptyBatchIsNotAnError(t *testing.T) {
	d := VideoClearDispatcher(&fakeStore{})
	dc := make(DataChannel, BatchSize)
	if err := d(context.Background(), dc); err != nil {
		t.Fatalf("empty batch must not be an error, got %v", err)
	}
}

// 缺陷 4：错误必须能传到 Run 的调用方。
func TestDispatcherErrorPropagates(t *testing.T) {
	want := errors.New("db down")
	d := VideoClearDispatcher(&fakeStore{readErr: want})
	noop := func(ctx context.Context, dc DataChannel) error { return nil }

	r := NewRunner(false, d, noop)
	err := r.Run(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("want error wrapping %v, got %v", want, err)
	}
}

// 完整链路：假 Store + 真 dispatcher/executor。
// 文件不存在是可接受的（幂等），所以不碰真实磁盘也能跑。
func TestVideoClearEndToEnd(t *testing.T) {
	fs := &fakeStore{ids: []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	}}

	r := NewRunner(false, VideoClearDispatcher(fs), VideoClearExecutor(fs))
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n := fs.deletedCount(); n != 2 {
		t.Fatalf("want 2 records deleted, got %d", n)
	}
}

// ctx 取消后必须退出。
// 注意 time.After 是「超时上限」，不是「用 Sleep 等结果」——
// 前者失败说明有 bug，后者只是白等。
func TestRunnerStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	block := func(ctx context.Context, dc DataChannel) error {
		<-ctx.Done()
		return ctx.Err()
	}

	r := NewRunner(true, block, block)
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}
```

另外，`task_test.go` 里的 `TestDeleteVideoRejectsInvalidID` 因为 `DeleteVideo` 被 `deleteOne` 取代而失效，改成：

```go
func TestDeleteOneRejectsInvalidID(t *testing.T) {
	if err := deleteOne(context.Background(), nil, "../etc/passwd"); err == nil {
		t.Fatal("expected error for invalid video id")
	}
}
```

（`deleteOne` 先校验 vid，非法时在碰 store 之前就返回，所以传 `nil` store 是安全的。）

**改了什么、为什么**：

- **假 `Function` 注入**：框架的 `Dispatcher`/`Executor` 本就是可注入的，测试塞自己的实现，不碰数据库和磁盘。
- **`fakeStore` 里的 `sync.Mutex` 不是多余**：`VideoClearExecutor` 会并发调 `DeleteVideoDeletionRecord`，不加锁 `go test -race` 会报数据竞争。
- **`TestVideoClearEndToEnd` 跳过真实磁盘**：因为 `deleteOne` 对「文件不存在」是幂等的——幂等性让测试变简单，这是设计红利。
- **全程无 `time.Sleep`**：`Run` 是同步的，返回时活已干完。

跑的时候带上 `-race`：

```powershell
go test -race ./scheduler/taskrunner/
```

---

## 4. 验收标准

- [ ] `Run(ctx)` 会发送初始状态并跑完一轮；`TestRunnerRunsOneCycle` 通过（旧实现下它会挂住）。
- [ ] 状态分支用 `switch`，不再是两个独立 `if`。
- [ ] `DataChannel` 是 `chan string`，代码中没有 `.(string)` 断言。
- [ ] `Function` 签名带 `ctx`；`Run` 在 `ctx` 取消后能返回。
- [ ] dispatcher 空批次返回 `nil`。
- [ ] executor 的 goroutine 全部纳入 `errgroup`，无手写 WaitGroup，`go test -race` 无报告。
- [ ] 错误能通过 `Run` 的返回值传播，`errors.Is` 可解。
- [ ] `dataSize` 不再是独立参数；`send` 在缓冲满时报错而非挂起。
- [ ] `Worker` 用 `*time.Ticker`，`defer Stop()`，不在 tick 里 `go`，`interval` 不再乘 `time.Second`。
- [ ] `scheduler/main.go` 用 `signal.NotifyContext` 建立 ctx 并传给 `taskrunner.Start`。
- [ ] 测试无 `time.Sleep`，用假 Store / 假 Function，全部确定性。
- [ ] 三件套通过：`gofmt -l` 干净、`go build ./...`、`go test -race ./...`。

## 5. 提交建议

拆成多个可运行提交：

```text
1. feat: add ctx to scheduler dbops methods
2. refactor: type the taskrunner channels and add ctx to Function
3. fix: fire the initial state and make state branches exclusive
4. fix: propagate taskrunner errors and derive data buffer from batch size
5. fix: manage executor goroutines with errgroup
6. fix: drive the state machine from a non-overlapping ticker worker
7. test: replace sleeping runner test with deterministic fakes
```

每步之间跑 `go build ./scheduler` 保持可编译。

## 6. 拓展思考题

1. `Run` 里「先发 `StateDispatch`，再进 `loop`」和原代码「先发状态，再直接调 `StartDispatch`」看起来只差一行——为什么一个能跑、一个永久阻塞？用 `select` 的语义解释。
2. `errgroup.WithContext` 在某个 goroutine 返回错误后会做什么？既然它会取消其余的删除，为什么这里仍然安全？（提示：`deleteOne` 的幂等性）
3. `VideoClearExecutor` 里的 `default` 分支依赖一个不变量——「dispatcher 已经完整跑完」。如果将来把 dispatch 和 execute 改成真正并发（流水线），这个 `default` 会怎样出错？那时该怎么改？
4. 本版去掉了 `Error` channel。如果保留它，它能表达什么 `ctx` 表达不了的东西？如果表达不了，为什么「两个机制做同一件事」是坏味道？
5. `send` 用 `default` 把「缓冲满」从挂死变成报错（快速失败）。有没有场景下你宁可让它阻塞？那时的缓冲策略应该是什么？
6. 你的框架现在能承载任意「批量读 → 批量处理」任务。假设下一个任务需要三个阶段（读 → 转换 → 上传），状态机要怎么扩展？如果扩展需要改动 `loop` 本体，那它还算「框架」吗？
