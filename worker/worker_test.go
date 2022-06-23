package worker

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

// Test that submitted work is being handled
func TestWorkerStart(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	worker, err := New(redis.Options{
		Addr: s.Addr(),
	}, "", "test")
	require.Nil(err)
	require.NotNil(worker)

	worker.Start()

	wg := sync.WaitGroup{}
	wg.Add(1)
	expected := []byte("{}")
	// Mock out handleWork so we can test that the work was received
	h := handleWork
	defer func() {
		handleWork = h
	}()
	handleWork = func(s string, got []byte) (*Result, *WorkPacket) {
		require.Equal(expected, got)
		defer wg.Done()
		var result Result
		var pkt WorkPacket
		return &result, &pkt
	}

	// Submit some work
	worker.transport.Send(worker.workQueue) <- expected
	wg.Wait()
}

func TestHandleWorkErrorResultOnBadJSON(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	worker, err := New(redis.Options{
		Addr: s.Addr(),
	}, "", "test")
	require.Nil(err)
	require.NotNil(worker)

	worker.Start()

	wg := sync.WaitGroup{}
	wg.Add(1)
	work := []byte("bad")

	result, _ := handleWork("", work)
	require.NotNil(result)
	require.Equal(-1, result.Code)
	require.NotEmpty(result.Error)
}

func TestHandleWorkErrorResultOnNoCommand(t *testing.T) {
	require := require.New(t)

	pkt := WorkPacket{
		Environment: make(map[string]string),
		Args:        []string{},
	}
	b, err := json.Marshal(pkt)
	require.Nil(err)

	r := runWork
	defer func() {
		runWork = r
	}()
	runWork = func(pkt *WorkPacket, result *Result) {
		require.FailNow("Should not have called runWork")
	}
	result, _ := handleWork("", b)
	require.Equal(-1, result.Code)
	require.NotEmpty(result.Error)
}

func TestRunWorkWithArgs(t *testing.T) {
	require := require.New(t)

	expected := "hello"
	pkt := WorkPacket{
		Environment: make(map[string]string),
		Command:     "/bin/echo",
		Args:        []string{expected},
	}
	var result Result
	runWork(&pkt, &result)
	require.Equal(0, result.Code)
	require.Empty(result.Error)
	require.Equal(fmt.Sprintf("%v\n", expected), result.Stdout)
	require.Empty(result.Stderr)
}

func TestRunWorkWithNoArgs(t *testing.T) {
	require := require.New(t)

	pkt := WorkPacket{
		Environment: make(map[string]string),
		Command:     "/bin/true",
	}
	var result Result
	runWork(&pkt, &result)
	require.Equal(0, result.Code)
	require.Empty(result.Error)
	require.Empty(result.Stdout)
	require.Empty(result.Stderr)
}

func TestRunWorkWithNoEnvironment(t *testing.T) {
	require := require.New(t)

	pkt := WorkPacket{
		Command: "/bin/true",
	}
	var result Result
	runWork(&pkt, &result)
	require.Equal(0, result.Code)
	require.Empty(result.Error)
	require.Empty(result.Stdout)
	require.Empty(result.Stderr)
}

func TestRunWorkProcessesEnvironment(t *testing.T) {
	require := require.New(t)

	expected := "hello"

	cmdline := "/bin/bash -c 'echo $ENV_VAR'"
	tokens, err := shlex.Split(cmdline)
	require.Nil(err)

	pkt := WorkPacket{
		Environment: map[string]string{
			"ENV_VAR": expected,
		},
		Command: tokens[0],
		Args:    tokens[1:],
	}
	var result Result
	runWork(&pkt, &result)
	require.Equal(0, result.Code)
	require.Empty(result.Error)
	require.Equal(fmt.Sprintf("%v\n", expected), result.Stdout)
	require.Empty(result.Stderr)
}
