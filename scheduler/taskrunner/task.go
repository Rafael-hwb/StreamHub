package taskrunner

import (
	"errors"
	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
	"log"
	"os"
	"sync"
)

func DeleteVideo(vid string) error {
	err := os.Remove(VIDEO_PATH + vid)

	if err != nil && os.IsNotExist(err) {
		log.Printf("Deleting video error: %v", err)
	}

	return nil
}

func VideoClearDispatcher(dc DataChannel) error {
	ids, err := dbops.ReadVideoDeletionRecord(3)
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

func VideoClearExecutor(dc DataChannel) error {
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

				if err := dbops.DeleteVideoDeletionRecord(id.(string)); err != nil {
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
