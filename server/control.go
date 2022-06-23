package server

import "encoding/json"

type ControlCommand string

const (
	CommandStop ControlCommand = "stop"
)

type ControlPacket struct {
	Command ControlCommand `json:"command"`
}

var handleControlMessage = func(s *Server, msg []byte) error {
	var pkt ControlPacket
	err := json.Unmarshal(msg, &pkt)
	if err != nil {
		return err
	}
	switch pkt.Command {
	case CommandStop:
		s.Stop()
	}
	return nil
}
