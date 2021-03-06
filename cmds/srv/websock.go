package main

import (
	"encoding/json"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
	"strconv"
)

// JsonPlayerEvent extracts the information that event asserts has changed from the status
// and returns it as a JSON message. For example, if the event is a VolumeChange, then the volume
// is extracted and encoded as a JSON message.
func jsonPlayerEvent(event play.Event, queue []map[string]string) ([]byte, error) {

	switch event.Type {
	case play.OffsetChange:
		return json.Marshal(map[string]int{"Offset": event.Data.(int)})
	case play.VolumeChange:
		return json.Marshal(map[string]byte{"Volume": event.Data.(byte)})
	case play.StateChange:
		return json.Marshal(map[string]play.PlayerState{"State": event.Data.(play.PlayerState)})
	case play.QueueChange:
		return json.Marshal(map[string][]map[string]string{"Queue": queue})
	case play.Error:
		return json.Marshal(map[string]string{"Error": event.Data.(error).Error()})
	default:
		panic("JsonPlayerEvent doesn't handle event " + strconv.Itoa(int(event.Type)))
	}
}

// JsonPlayerStatus creates a JSON message that contains all the information from the status object
func jsonPlayerStatus(status play.PlayerStatus) ([]byte, error) {
	return json.Marshal(status)
}

// jsonFullStatus creates a JSON message that contains the player status and the meta information
func jsonFullStatus(status play.PlayerStatus, meta map[string]string, queue []map[string]string, recent []map[string]string, repeatMode RepeatMode) ([]byte, error) {
	a := struct {
		play.PlayerStatus
		Meta       map[string]string
		Queue      []map[string]string
		Recent     []map[string]string
		RepeatMode string
	}{status, meta, queue, recent, repeatMode.String()}
	return json.Marshal(a)
}

// jsonMeta creates a JSON message with the current mp3's metadata
func jsonMeta(meta map[string]string) ([]byte, error) {
	a := struct {
		Meta map[string]string
	}{Meta: meta}
	return json.Marshal(a)
}

// jsonScan creates a JSON message with the current scan event's data
func jsonScan(meta *scan.Metadata) ([]byte, error) {
	a := struct {
		Scan *scan.Metadata
	}{Scan: meta}
	return json.Marshal(a)
}

// jsonRecent creates a JSON message with the recently played tracks.
func jsonRecent(recent []map[string]string) ([]byte, error) {
	a := struct {
		Recent []map[string]string
	}{Recent: recent}
	return json.Marshal(a)
}

// jsonRepeat creates a JSON message with the current repeat mode
func jsonRepeat(repeatMode RepeatMode) ([]byte, error) {
	a := struct {
		RepeatMode string
	}{RepeatMode: repeatMode.String()}
	return json.Marshal(a)
}
