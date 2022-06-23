package main

import (
	"fmt"
	"runtime"

	"github.com/go-redis/redis"
	"github.com/gurupras/runner/server"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	verbose   = kingpin.Flag("verbose", "Enable verbose logs").Short('v').Default("false").Bool()
	workQueue = kingpin.Flag("work-queue", "Use a specific work-queue").Short('w').Required().String()
	numProcs  = kingpin.Flag("num-procs", "Number of processes to run in parallel").Short('j').Default(fmt.Sprintf("%v", runtime.NumCPU())).Int()
	redisAddr = kingpin.Flag("redis-addr", "Redis address").Short('R').Required().String()
)

func main() {
	kingpin.Parse()

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	if len(*workQueue) == 0 {
		log.Fatalf("Must specify a work-queue")
	}

	if *numProcs < 0 || *numProcs > runtime.NumCPU()*3 {
		log.Fatalf("Invalid value for num-procs. Must be between 0-%v", runtime.NumCPU()*3)
	}

	server := server.New(*workQueue, *numProcs, redis.Options{Addr: *redisAddr})
	err := server.Register()
	if err != nil {
		log.Fatalf("Failed to register server: %v", err)
	}

	server.Start()
	server.Wait()
}
