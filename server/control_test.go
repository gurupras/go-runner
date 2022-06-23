package server

import (
	"encoding/json"
	"math"
	"runtime"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/require"
)

func TestControlStop(t *testing.T) {
	require := require.New(t)

	s := miniredis.RunT(t)

	opts := redis.Options{
		Addr: s.Addr(),
	}

	nProcs := int(math.Min(float64(runtime.NumCPU()), 4))
	server := New("test", nProcs, opts)
	server.Start()

	pkt := ControlPacket{
		Command: CommandStop,
	}
	b, err := json.Marshal(pkt)
	require.Nil(err)

	err = handleControlMessage(server, b)
	require.Nil(err)

	server.Wait()
	require.True(true)
}
