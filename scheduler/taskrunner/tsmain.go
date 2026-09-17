package taskrunner

import (
	"context"
	"log"
	"time"
)

type Worker struct {
	ticker *time.Ticker
	runner *Runner
}

func NewWorker(interval time.Duration, runner *Runner) *Worker {
	return &Worker{
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
			// 顺序执行，不 go 出去 —— 重叠运行会并发读写同一批记录。
			if err := w.runner.Run(ctx); err != nil {
				log.Printf("taskrunner: %v", err)
			}
		}
	}
}

func Start(ctx context.Context, store Store, interval time.Duration) {
	runner := NewRunner(true, VideoClearDispatcher(store), VideoClearExecutor(store))
	NewWorker(interval, runner).Start(ctx)
}
