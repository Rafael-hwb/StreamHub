package main

import(
	"log"
)

type ConnectionLimiter struct{
	maxConnection int
	bucket chan int
}

func CreateConnectionLimiter(maxCount int) *ConnectionLimiter{
	return &ConnectionLimiter{
		maxConnection: maxCount,
		bucket: make(chan int, maxCount),
	}
}

func (limiter *ConnectionLimiter) GetConnection() bool{
	if len(limiter.bucket) >= limiter.maxConnection{
		log.Printf("Reach the rate limitation.")
		return false
	}

	limiter.bucket <- 1
	return true
}

func (limiter *ConnectionLimiter) ReleaseConnection(){
	c := <- limiter.bucket
	log.Printf("Connection(%d) released.",c)
}