package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-redis/redis"
	"github.com/google/shlex"
	"github.com/gurupras/runner/server"
	"github.com/gurupras/runner/worker"
	gonanoid "github.com/matoous/go-nanoid/v2"
	redisTransport "github.com/matryer/vice/v2/queues/redis"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	app       = kingpin.New("runctl", "Control a runner server")
	verbose   = app.Flag("verbose", "Enable verbose logs").Short('v').Default("false").Bool()
	workQueue = app.Flag("work-queue", "Use a specific work-queue").Short('w').Required().String()
	redisAddr = app.Flag("redis-addr", "Redis address").Short('R').Default("127.0.0.1:6379").String()

	stop = app.Command("stop", "Stop a runner")

	submit     = app.Command("submit", "Submit a job")
	submitCmd  = submit.Arg("command", "command-line").Required().String()
	submitEnv  = submit.Flag("env", "Environment variables to set. Expected to be of the form KEY=VAR. Multiple values are separated by semi-colon").Short('e').Default("").String()
	submitCwd  = submit.Flag("cwd", "Current working directory for the command").Default("").String()
	submitWait = submit.Flag("wait", "Wait for result").Short('W').Default("true").Bool()
	numJobs    = submit.Flag("num-jobs", "Number of times to submit the job").Short('j').Default("1").Int()
)

func main() {
	command, err := app.Parse(os.Args[1:])
	if err != nil {
		log.Fatalf("Invalid usage")
	}
	if len(*workQueue) == 0 {
		log.Fatalf("Must specify a work-queue")
	}

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	client := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	transport := redisTransport.New(redisTransport.WithClient(client))
	defer transport.Stop()

	controlChannel := server.GetControlChannel(*workQueue)
	switch kingpin.MustParse(command, err) {
	case stop.FullCommand():
		pkt := server.ControlPacket{
			Command: server.CommandStop,
		}
		b, _ := json.Marshal(pkt)
		transport.Send(controlChannel) <- b
	case submit.FullCommand():
		*submitEnv = strings.ReplaceAll(*submitEnv, "; ", ";")
		envEntries := strings.Split(*submitEnv, ";")
		env := make(map[string]string)
		for _, entry := range envEntries {
			tokens := strings.SplitN(entry, "=", 1)
			if len(tokens) == 2 {
				k := tokens[0]
				v := tokens[1]
				env[k] = v
			}
		}
		tokens, err := shlex.Split(*submitCmd)
		if err != nil {
			log.Fatalf("Invalid command: %v", err)
		}

		resultChan := make(chan *worker.Result)

		publishResult := false
		wg := sync.WaitGroup{}

		var ids []string
		if *submitWait {
			ids = make([]string, *numJobs)
			for idx := 0; idx < *numJobs; idx++ {
				id, _ := gonanoid.New(8)
				ids[idx] = id
			}
			wg.Add(1)
			publishResult = true
			// Set up listener before submitting work
			go func() {
				defer wg.Done()
				for result := range resultChan {
					if result.Code == 0 {
						fmt.Printf("%v\n", strings.TrimSpace(result.Stdout))
					} else {
						fmt.Printf("%v\n", strings.TrimSpace(result.Stderr))
					}
				}
			}()
			go func() {
				defer close(resultChan)
				for idx := 0; idx < *numJobs; idx++ {
					id := ids[idx]
					var result worker.Result
					b := <-transport.Receive(fmt.Sprintf("%v:results:%v", *workQueue, id))
					json.Unmarshal(b, &result)
					resultChan <- &result
				}
			}()
		}
		for idx := 0; idx < *numJobs; idx++ {
			id := ""
			if *submitWait {
				id = ids[idx]
			}
			pkt := worker.WorkPacket{
				Id:            id,
				Environment:   env,
				Command:       tokens[0],
				Args:          tokens[1:],
				Dir:           *submitCwd,
				PublishResult: &publishResult,
			}
			b, _ := json.Marshal(pkt)
			transport.Send(*workQueue) <- b
		}

		if *submitWait {
			wg.Wait()
		}
	}
}
