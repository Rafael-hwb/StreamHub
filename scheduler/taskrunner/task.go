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
