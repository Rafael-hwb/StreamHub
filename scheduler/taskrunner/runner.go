package taskrunner

type Runner struct{
	Controller ControlChannel
	Error ControlChannel
	Data DataChannel
	dataSize int
	longLived bool
	Dispatcher Function
	Executor Function
}

func CreateNewRunner(dataSize int, longLived bool, d Function, e Function) *Runner {
	return &Runner{
		Controller:  make(chan string, 1),
		Error:       make(chan string, 1),
		Data:        make(chan interface{}, dataSize),
		dataSize:    dataSize,
		longLived:   longLived,
		Dispatcher:  d,
		Executor:    e,
	}
}

func (r *Runner) StartDispatch(){
	defer func(){
		if !r.longLived{
			close(r.Controller)
			close(r.Error)
			close(r.Data)
		}
	}()

	for{
		select{
		case c := <- r.Controller:
			if c == READY_TO_DISPATCH{
				err := r.Dispatcher(r.Data)
				if err != nil{
					r.Error <- CLOSE
				}else{
					r.Controller <- READY_TO_EXECUTE
				}
			}

			if c == READY_TO_EXECUTE{
				err := r.Executor(r.Data)
				if err != nil{
					r.Error <- CLOSE
				}else{
					r.Controller <- READY_TO_DISPATCH
				}
			}
		
		case e := <- r.Error:
			if e == CLOSE{
				return
			}
		}
	}
}

func (r *Runner) StartAll(){
	r.Controller <- READY_TO_DISPATCH
	r.StartDispatch()
}