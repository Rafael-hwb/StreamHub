package taskrunner

import (
	"time"
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

func Start() {
	r := CreateNewRunner(3, false, VideoClearDispatcher, VideoClearExecutor)
	go r.StartDispatch()
}
