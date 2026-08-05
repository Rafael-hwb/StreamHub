package taskrunner

import (
	"errors"
	"sync"
	"log"
	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
	"os"
)

var err error

func DeleteVideo(vid string) error{
	err := os.Remove(VEDIO_PATH+ vid)

	if err != nil && os.IsNotExist(err){
		log.Printf("Deleting video error: %v", err)
	}

	return nil
}


func VedioClearDispatcher(dc DataChannel, count int) error{
	ids, err := dbops.ReadVideoDeletionRecord(count)
	if err != nil {
		log.Printf("VideoClearDispatcher error: %v", err)
		return err
	}

	if len(ids) == 0{
		return errors.New("VideoClearDispatcher is empty.")
	}

	for _, id := range(ids){
		dc <- id
	}
	return nil
}

func VideoClearExecuter(dc DataChannel) error{
	errMap := &sync.Map{}
	forloop:
		for{
			select{
			case id := <- dc:
				go func(id interface{}){
					if err := DeleteVideo(id.(string)); err != nil{
						errMap.Store(id, err)
						return
					}

					if err := dbops.DeleteVideoDeletionRecord(id.(string)); err != nil{
						errMap.Store(id, err)
						return
					}
				}(id)
			
			default:
				break forloop
			}
		}
	errMap.Range(func(k, v interface{}) bool{
		err = v.(error)
		if err != nil{
			return false
		}
		return true
	})
	return err
	}

