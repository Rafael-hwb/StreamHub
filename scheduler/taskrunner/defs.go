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
