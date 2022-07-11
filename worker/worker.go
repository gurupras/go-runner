package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/matryer/vice/v2"
	redisTransport "github.com/matryer/vice/v2/queues/redis"
	log "github.com/sirupsen/logrus"
)

type Worker struct {
	client    *redis.Client
	transport vice.Transport
	id        string
	workQueue string
	doneChan  chan struct{}
}

type WorkPacket struct {
	Id            string            `json:"id"`
	Environment   map[string]string `json:"environment,omitempty"`
	Command       string            `json:"command"`
	Args          []string          `json:"args,omitempty"`
	Dir           string            `json:"dir"`
	PublishResult *bool             `json:"publishResult"`
}

type Result struct {
	Source   string           `json:"string"`
	Code     int              `json:"code"`
	Error    string           `json:"error,omitempty"`
	Stdout   string           `json:"stdout"`
	Stderr   string           `json:"stderr"`
	Duration map[string]int64 `json:"duration"`
}

func getID(suffix string) (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("unable to get current user: %v", err)
	}
	username := user.Username
	_ = username
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("unable to get hostname: %v", err)
	}
	return fmt.Sprintf("%v:%v", hostname, suffix), nil
}

// Global method so it can be mocked during tests
var handleWork = func(source string, workBytes []byte) (*Result, *WorkPacket) {
	var pkt WorkPacket
	result := &Result{
		Source:   source,
		Duration: make(map[string]int64),
	}
	jsonStart := time.Now()
	err := json.Unmarshal(workBytes, &pkt)
	jsonEnd := time.Now()
	result.Duration["jsonUnmarshal"] = jsonEnd.Sub(jsonStart).Milliseconds()
	if err != nil {
		result.Code = -1
		result.Error = err.Error()
		return result, nil
	}
	if pkt.Command == "" {
		result.Code = -1
		result.Error = "must specify command"
		return result, nil
	}
	// Add source to environment so logs/etc can be directed accordingly
	if pkt.Environment == nil {
		pkt.Environment = make(map[string]string)
	}
	pkt.Environment["RUNNER_WORKER_ID"] = source
	if pkt.PublishResult == nil {
		tmp := true
		pkt.PublishResult = &tmp
	}
	workStart := time.Now()
	log.Debugf("[%v]: Starting work: %v", source, pkt.Command)
	runWork(&pkt, result)
	workEnd := time.Now()
	result.Duration["work"] = workEnd.Sub(workStart).Milliseconds()
	log.Debugf("[%v]: Finished work: %v", source, pkt.Command)
	return result, &pkt
}

var runWork = func(work *WorkPacket, result *Result) {
	cmd := exec.Command(work.Command, work.Args...)
	// Get all current environment variables and add them
	cmd.Env = append(cmd.Env, os.Environ()...)
	for k, v := range work.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf(`%v=%v`, k, v))
	}
	cmd.Dir = work.Dir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()
	result.Code = 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.Code = exitError.ExitCode()
		} else {
			result.Code = -1
			result.Error = err.Error()
		}
	}
}

func New(opts redis.Options, uid string, workQueue string) (*Worker, error) {
	client := redis.NewClient(&opts)
	return NewWithClient(client, uid, workQueue)
}

func NewWithClient(client *redis.Client, uid string, workQueue string) (*Worker, error) {
	id, err := getID(uid)
	if err != nil {
		return nil, err
	}
	return &Worker{
		client:    client,
		transport: redisTransport.New(redisTransport.WithClient(client)),
		id:        id,
		workQueue: workQueue,
		doneChan:  make(chan struct{}),
	}, nil
}

func (w *Worker) Start() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	var once sync.Once
	go func() {
		for {
			select {
			case <-w.transport.Done():
				return
			case work := <-w.transport.Receive(w.workQueue):
				log.Debugf("[%v]: Got work", w.id)
				result, pkt := handleWork(w.id, work)
				b, _ := json.Marshal(result)
				if *pkt.PublishResult {
					w.transport.Send(fmt.Sprintf("%v:results", w.workQueue)) <- b
				}
				log.Debugf("packet id=%v", pkt.Id)
				if strings.Compare(pkt.Id, "") != 0 && *pkt.PublishResult {
					// Send result down specific channel as well
					log.Debugf("[%v]: Sent results down packet-result channel", w.id)
					w.transport.Send(fmt.Sprintf("%v:results:%v", w.workQueue, pkt.Id)) <- b
				}
				log.Debugf("[%v]: Sent results down channel", w.id)
			case <-w.doneChan:
				return
			default:
				once.Do(func() {
					wg.Done()
				})
			}
		}
	}()
	// Wait for goroutine to start
	wg.Wait()
	log.Debugf("[%v]: Ready", w.id)
}

func (w *Worker) Stop() {
	w.doneChan <- struct{}{}
	w.transport.Stop()
}
