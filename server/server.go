package server

import (
	"fmt"
	"os"
	"os/user"
	"sync"
	"time"

	"github.com/go-redis/redis"
	"github.com/gurupras/runner/worker"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/matryer/vice"
	redisTransport "github.com/matryer/vice/v2/queues/redis"
	log "github.com/sirupsen/logrus"
)

var (
	SERVERS_KEY_EXPIRE = 24 * 2 * time.Hour
)

type Server struct {
	client    *redis.Client
	workQueue string
	NumProcs  int
	Workers   []*worker.Worker
	transport vice.Transport
	doneChan  chan struct{}
	wg        sync.WaitGroup
}

func New(workQueue string, numProcs int, opts redis.Options) *Server {
	// Create workers
	client := redis.NewClient(&opts)
	workers := make([]*worker.Worker, numProcs)
	for idx := 0; idx < numProcs; idx++ {
		uid, _ := gonanoid.New(8)
		worker, err := worker.NewWithClient(client, uid, workQueue)
		if err != nil {
			log.Fatalf("Failed to create worker")
		}
		workers[idx] = worker
		// worker.Register()
	}
	return &Server{
		client:    client,
		workQueue: workQueue,
		NumProcs:  numProcs,
		Workers:   workers,
		transport: redisTransport.New(redisTransport.WithClient(client)),
		doneChan:  make(chan struct{}),
		wg:        sync.WaitGroup{},
	}
}

func (s *Server) Start() {
	for _, worker := range s.Workers {
		worker.Start()
	}
	startedWG := sync.WaitGroup{}
	startedWG.Add(1)
	var once sync.Once
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		controlChannel := GetControlChannel(s.workQueue)
		for {
			select {
			case <-s.transport.Done():
				return
			case msg := <-s.transport.Receive(controlChannel):
				go handleControlMessage(s, msg)
			case <-s.doneChan:
				s.transport.Stop()
			default:
				once.Do(func() {
					startedWG.Done()
				})

			}
		}
	}()
	startedWG.Wait()
}

func (s *Server) Stop() {
	for _, worker := range s.Workers {
		worker.Stop()
	}
	s.doneChan <- struct{}{}
}

func (s *Server) Wait() {
	s.wg.Wait()
}

func getID() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("unable to get current user: %v", err)
	}
	username := user.Username
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("unable to get hostname: %v", err)
	}
	return fmt.Sprintf("%v@%v", username, hostname), nil
}

func (s *Server) Register() error {
	id, err := getID()
	if err != nil {
		return err
	}
	key := getServersKey(s.workQueue)
	err = s.client.SAdd(key, id).Err()
	if err != nil {
		return err
	}
	// Expire the key in 24 hours
	s.client.Expire(key, SERVERS_KEY_EXPIRE)
	return nil
}

func getServersKey(workQueue string) string {
	return fmt.Sprintf("%v:servers", workQueue)
}

func GetControlChannel(workQueue string) string {
	return fmt.Sprintf("%v:control", workQueue)
}
