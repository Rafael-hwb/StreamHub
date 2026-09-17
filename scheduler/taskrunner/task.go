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
// *dbops.Store 方法签名匹配即自动满足，无需声明。
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
				g.Go(func() error {
					return deleteOne(ctx, store, id)
				})
			default:
				// Data 已空。安全的前提是 dispatcher 已完整跑完
				// （状态机保证 dispatch 先于 execute）。
				// 若改成并发流水线，这个分支必须重写。
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

	// 文件已不存在是可接受的结果（幂等）。
	if err := os.Remove(VIDEO_PATH + vid); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", vid, err)
	}

	if err := store.DeleteVideoDeletionRecord(ctx, vid); err != nil {
		return fmt.Errorf("delete record %s: %w", vid, err)
	}
	return nil
}
