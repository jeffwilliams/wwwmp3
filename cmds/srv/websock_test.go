package main

import (
	"github.com/jeffwilliams/wwwmp3/play"
	"testing"
)

func TestJsonPlayerStatus(t *testing.T) {
	s := play.PlayerStatus{
		Offset: 0,
		Size:   1,
		State:  play.Empty,
		Volume: 2,
	}

	j, _ := jsonPlayerStatus(s)

	t.Log(string(j))
}

func TestJsonPlayerEvent(t *testing.T) {
	s := play.PlayerStatus{
		Offset: 0,
		Size:   1,
		State:  play.Empty,
		Volume: 2,
	}

	j, _ := jsonPlayerEvent(s, play.OffsetChange)
	t.Log(string(j))
	j, _ = jsonPlayerEvent(s, play.StateChange)
	t.Log(string(j))
	j, _ = jsonPlayerEvent(s, play.VolumeChange)
	t.Log(string(j))
}
