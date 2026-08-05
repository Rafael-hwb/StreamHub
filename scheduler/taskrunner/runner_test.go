package taskrunner

import (
	"testing"
	"log"
	"time"
	"errors"
)

func TestRunner(t *testing.T){
	d := func(dc DataChannel) error{
		for i := 0; i < 30 ;i++{
			dc <- i;
			log.Printf("Dispatch send: %v", i)
		}
		return nil
	}

	e := func(dc DataChannel) error{
		forloop:
			for{
				select{
				case data := <- dc:
					log.Printf("Execute recieve: %v", data)

				default:
					break forloop
				}
			}
		return errors.New("execute")
	}

	runner := CreateNewRunner(30, false, d, e)
	go runner.StartAll()
	time.Sleep(3 * time.Second)
}