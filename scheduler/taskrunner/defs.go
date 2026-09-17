package taskrunner

import "context"

const (
	VIDEO_PATH = "./videos/"

	// BatchSize 既是每轮从库里取多少条删除记录，
	// 也是 Data channel 的缓冲大小（见 NewRunner）。
	BatchSize = 3
)

// State 是状态机在 controller 上传递的状态令牌。
type State string

const (
	StateDispatch State = "dispatch"
	StateExecute  State = "execute"
	StateClose    State = "close"
)

type ControlChannel chan State

// DataChannel 只承载视频 id。
type DataChannel chan string

// Function 是 Dispatcher / Executor 的统一签名。
type Function func(ctx context.Context, dc DataChannel) error
