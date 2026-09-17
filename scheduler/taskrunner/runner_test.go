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

// 状态机必须真的点火并跑完一轮。旧实现下这个测试会永远挂住。
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

// 空批次不是错误。
func TestDispatcherEmptyBatchIsNotAnError(t *testing.T) {
	d := VideoClearDispatcher(&fakeStore{})
	dc := make(DataChannel, BatchSize)
	if err := d(context.Background(), dc); err != nil {
		t.Fatalf("empty batch must not be an error, got %v", err)
	}
}

// 错误必须能传到 Run 的调用方。
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
// 这里的 time.After 是超时上限，不是用 Sleep 等结果。
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
