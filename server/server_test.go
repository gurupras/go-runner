package server

import (
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
	"github.com/gurupras/runner"
	"github.com/gurupras/runner/worker"
	redisTransport "github.com/matryer/vice/v2/queues/redis"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestServerCreation(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	opts := redis.Options{
		Addr: s.Addr(),
	}

	nProcs := int(math.Min(float64(runtime.NumCPU()), 4))
	server := New("test", nProcs, opts)
	require.NotNil(server)
}

func TestServerRegister(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	opts := redis.Options{
		Addr: s.Addr(),
	}

	nProcs := int(math.Min(float64(runtime.NumCPU()), 4))
	server := New("test", nProcs, opts)

	err := server.Register()
	require.Nil(err)

	expected, err := getID()
	require.Nil(err)

	members, err := s.SMembers(getServersKey(server.workQueue))
	require.Nil(err)
	require.Equal(1, len(members))
	require.Equal(expected, members[0])
}

func TestServerStart(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	opts := redis.Options{
		Addr: s.Addr(),
	}

	sleepSeconds := 1
	nProcs := int(math.Min(float64(runtime.NumCPU()), 4))
	server := New("test", nProcs, opts)
	require.NotNil(server)
	server.Start()

	// Now, queue up work
	client := redis.NewClient(&opts)
	transport := redisTransport.New(redisTransport.WithClient(client))

	numTasks := nProcs
	count := 0
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-transport.Done():
				return
			case err := <-transport.ErrChan():
				require.Nil(err)
			case b := <-transport.Receive(runner.GetResultsQueue(server.workQueue)):
				var result worker.Result
				json.Unmarshal(b, &result)
				log.Debugf("%v", result.Duration)
				count++
				if count == numTasks {
					transport.Stop()
				}
			}
		}
	}()

	pkt := worker.WorkPacket{
		Command: "sleep",
		Args:    []string{fmt.Sprintf("%v", sleepSeconds)},
	}
	b, err := json.Marshal(pkt)
	require.Nil(err)

	start := time.Now()
	for idx := 0; idx < numTasks; idx++ {
		transport.Send(server.workQueue) <- b
	}

	wg.Wait()
	end := time.Now()
	// If we ran in parallel, then we should've been done in ~sleepSeconds
	// We give it 3x just in case
	require.WithinDuration(end, start, time.Duration(float64(sleepSeconds)*1000*3)*time.Millisecond)
}

func TestControlCommandStop(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	opts := redis.Options{
		Addr: s.Addr(),
	}

	nProcs := int(math.Min(float64(runtime.NumCPU()), 4))
	server := New("test", nProcs, opts)
	require.NotNil(server)
	server.Start()

	client := redis.NewClient(&opts)
	transport := redisTransport.New(redisTransport.WithClient(client))

	// Send the stop command and make sure the work stops
	pkt := ControlPacket{
		Command: CommandStop,
	}
	b, err := json.Marshal(pkt)
	require.Nil(err)

	transport.Send(GetControlChannel(server.workQueue)) <- b
	server.Wait()
}
