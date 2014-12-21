package main

import (
	"encoding/json"
	"github.com/jeffwilliams/wwwmp3/play"
	"strconv"
)

type WebsockMsgType int

const (
	Offset WebsockMsgType = iota
	State
	Volume
	Info
)

type WebsockMsg struct {
	Type WebsockMsgType
}

type WebsockOffsetMsg struct {
	WebsockMsg
	Offset int
}
type WebsockStateMsg struct {
	WebsockMsg
	State string
}

type WebsockVolumeMsg struct {
	WebsockMsg
	Volume int
}

type WebsockInfoMsg struct {
	WebsockMsg
	Offset int
	State  string
	Volume int
}

//func MsgFromPlayerStatus(play.PlayerStatus struct {

// JsonPlayerEvent extracts the information that event asserts has changed from the status
// and returns it as a JSON message. For example, if the event is a VolumeChange, then the volume
// is extracted and encoded as a JSON message.
func jsonPlayerEvent(status play.PlayerStatus, event play.Event) ([]byte, error) {

	switch event {
	case play.OffsetChange:
		return json.Marshal(map[string]int{"Offset": status.Offset})
	case play.VolumeChange:
		return json.Marshal(map[string]byte{"Volume": status.Volume})
	case play.StateChange:
		return json.Marshal(map[string]play.PlayerState{"State": status.State})
	default:
		panic("JsonPlayerEvent doesn't handle event " + strconv.Itoa(int(event)))
	}
}

// JsonPlayerStatus creates a JSON message that contains all the information from the status object
func jsonPlayerStatus(status play.PlayerStatus) ([]byte, error) {
	return json.Marshal(status)
}

// jsonFullStatus creates a JSON message that contains the player status and the meta information
func jsonFullStatus(status play.PlayerStatus, meta map[string]string) ([]byte, error) {
	a := struct {
		play.PlayerStatus
		Meta map[string]string
	}{status, meta}
	return json.Marshal(a)
}

// jsonMeta creates a JSON message with the current mp3's metadata
func jsonMeta(meta map[string]string) ([]byte, error) {
	a := struct {
		Meta map[string]string
	}{Meta: meta}
	return json.Marshal(a)
}
