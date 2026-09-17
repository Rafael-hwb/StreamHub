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
//	longLived=false: 单次任务，Run 返回后关闭 channel。
//	longLived=true:  常驻任务，channel 保持打开，Run 可被反复调用。
func NewRunner(longLived bool, d, e Function) *Runner {
	return &Runner{
		controller: make(ControlChannel, 1),
		data:       make(DataChannel, BatchSize),
		longLived:  longLived,
		dispatcher: d,
		executor:   e,
	}
}

// Run 跑完整的一轮：dispatch → execute。
func (r *Runner) Run(ctx context.Context) error {
	r.controller <- StateDispatch

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

// send 把 id 推进 Data channel，缓冲满时返回错误而不是永久阻塞。
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
