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
