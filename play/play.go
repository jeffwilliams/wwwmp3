// Package play implements a simple mp3 player. In general, to use this package first create a Player using NewPlayer,
// start a goroutine that listens for events on Player.Event, Load an mp3 using Player.Load, and play it using Player.Play().
// GetMetadata can be used to retrieve ID3 information from mp3 files.
package play

/*
#include <stdlib.h>
#include "play.h"

#cgo LDFLAGS: -lmpg123 -lao -lasound -lid3
*/
import "C"

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// For development
var _ = fmt.Println

const debug = false

// Package init function
func init() {
	C.play_init()
}

// Make an instance of error using the from the passed prefix and the C.play_get_last_error() return value.
func makePlayError(prefix string) error {
	return errors.New(prefix + C.GoString(C.play_get_last_error()))
}

// initialNum returns a string containing the initial sequence of digits
// at the beginning of s.
func initialNum(s string) string {
	nondigit := false

	f := func(r rune) rune {
		if nondigit || r < '0' || r > '9' {
			nondigit = true
			return -1
		} else {
			return r
		}
	}

	return strings.Map(f, s)
}

// Play the specified file. Return when playback is complete.
func Play(filename string) {
	n := C.CString(filename)
	C.play_play(n)
	C.free(unsafe.Pointer(n))
}

// Set the volume as a percentage between 0 and 100 inclusive.
// This method sets the volume on the default ALSA card.
func SetVolume(pct byte) (err error) {
	err = nil
	if C.play_setvolume(C.uchar(pct), C.CString("default")) < 0 {
		err = makePlayError("Setting volume failed: ")
	}

	return
}

// Set the volume as a percentage between 0 and 100 inclusive.
// This method sets the volume on all ALSA cards.
func SetVolumeAll(pct byte) (err error) {
	err = nil
	if C.play_setvolume_all(C.uchar(pct)) < 0 {
		err = makePlayError("Setting volume failed: ")
	}

	return
}

// Get the volume as a percentage between 0 and 100 inclusive.
func GetVolume() (volume byte, err error) {
	v := int8(C.play_getvolume())
	if v >= 0 {
		volume = byte(v)
	} else {
		// Error occurred. Return 0.
		volume = 0
		err = makePlayError("Getting volume failed: ")
	}

	return
}

// Metadata is information about an mp3 stored in id3 tags.
type Metadata struct {
	Title    string
	Artist   string
	Album    string
	Tracknum int
}

// Information about an mp3 determined once the mp3 is loaded.
type Info struct {
	// The bitrate of the mp3, in kbps
	BitRate int

	// Sampling rate in Hz
	Rate int

	// Duration of mp3 in seconds (if available) or zero otherwise
	Duration float64

	// Seconds per sample (if available) or zero otherwise
	Sps float64
}

// GetMetadata extracts the id3 information from the mp3 file `filename`.
// For integer fields (like Tracknum) for which there is no data the field
// is set to -1.
func GetMetadata(filename string) Metadata {
	meta := C.play_meta(C.CString(filename))

	tracknum := -1
	digits := initialNum(C.GoString(meta.tracknum))
	if len(digits) > 0 {
		if i, err := strconv.Atoi(digits); err == nil {
			tracknum = i
		}
	}

	r := Metadata{
		Title:    strings.Trim(C.GoString(meta.title), " "),
		Artist:   strings.Trim(C.GoString(meta.artist), " "),
		Album:    strings.Trim(C.GoString(meta.album), " "),
		Tracknum: tracknum,
	}
	C.play_delete_meta(meta)
	return r
}

// Print debugging information about the metadata in `filename` to stdout.
func DebugMetadata(filename string) {
	C.play_debug_meta(C.CString(filename))
}

// PlayerState represents the current state of a Player. Must be one of Empty, Playing, or Paused.
type PlayerState int

const (
	// The player has no mp3 loaded
	Empty PlayerState = iota
	// The player has an mp3 loaded and is playing it
	Playing
	// The player has an mp3 loaded and it is paused
	Paused
)

func (s PlayerState) String() string {
	switch s {
	case Empty:
		return "empty"
	case Playing:
		return "playing"
	case Paused:
		return "paused"
	default:
		panic("PlayerState.String() needs to be updated. Missing case for " + strconv.Itoa(int(s)))
	}
}

// Player is an mp3 player.
type Player struct {
	cmds chan interface{}
	// Events is a channel to which Player events are written, such as state changes, volume changes, or offset changes.
	// When an event is recieved the user should call GetStatus() to get the current player information.
	Events chan Event
}

// Internal command used by the Player
type loadCmd struct {
	path string
	err  chan error
	// on load, this channel is written the size of the mp3 in samples
	size chan int
}

// Internal command used by the Player
type playCmd chan error

// Internal command used by the Player
type pauseCmd bool

// Internal command used by the Player
type stopCmd bool

// The offset to seek to
type seekCmd int

// The percentage to set to
type setVolumeCmd byte

// Internal command used by the Player
type setVolumeAllCmd byte

// Internal command used by the Player
type getStatusCmd chan PlayerStatus

// Internal command used by the Player
type getInfoCmd chan *Info

// Internal command used by the Player
type setRepeatCmd bool

type EventType int

// Event represents events sent by the Player.
type Event struct {
	// Type is the type of event
	Type EventType
	// Data is the event data.
	// For OffsetChange, it is an int representing the offset.
	// For StateChange, it is a PlayerState.
	// For VolumeChange, it is a byte in the range 0-100 representing the volume.
	// For QueueChange, it is not set.
	// For Error, its an error.
	Data interface{}
}

const (
	OffsetChange EventType = iota
	StateChange
	VolumeChange
	QueueChange
	Error
)

func (e EventType) String() string {
	switch e {
	case OffsetChange:
		return "OffsetChange"
	case StateChange:
		return "StateChange"
	case VolumeChange:
		return "VolumeChange"
	case QueueChange:
		return "QueueChange"
	case Error:
		return "Error"
	default:
		return "Unknown"
	}
}

// PlayerStatus contains information about the current status of the player
type PlayerStatus struct {
	// Offset is the offset within the current track in samples
	Offset int
	// Size of the current track in samples
	Size int
	// State of the Player
	State PlayerState
	// Volume
	Volume byte
	// Path to current mp3
	Path string
}

// Create a new Player.
func NewPlayer() (p Player) {

	p.cmds = make(chan interface{})
	p.Events = make(chan Event, 1000)

	// This goroutine implements the player.
	go func() {
		var path string
		var reader *C.play_reader_t
		var writer *C.ao_device
		var lastofftime time.Time
		lastoff := -1
		// When a new track is loaded, this Info struct is loaded with the details.
		var currentTrackInfo *Info

		var state PlayerState = Empty
		var states [3]func()

		// When we reach the end, should we start over?
		var repeat bool = false

		debugCmd := func(cmd interface{}) {
			if debug {
				fmt.Printf("player: In state %s, got command %s\n", state, reflect.TypeOf(cmd))
			}
		}

		makeWriter := func() error {
			if reader == nil {
				return errors.New("Creating writer failed: reader was nil")
			}
			writer = C.play_new_writer(reader)
			if writer == nil {
				return makePlayError("Creating writer failed: ")
			}
			return nil
		}

		deleteWriter := func() {
			if writer != nil {
				C.play_delete_writer(writer)
				writer = nil
			}
		}

		sendEvent := func(e Event) {
			// Do not block when sending events. We buffer some and then drop the rest.
			// This is to prevent deadlocks: users of the client need to read events, but
			// they also send commands to the player. If the player is sending an event
			// while the event reader is sending a command to the player we have a deadlock.
			select {
			case p.Events <- e:
			default:
			}
		}

		getInfo := func() *Info {
			if state != Empty {
				i, err := C.play_getinfo(reader)
				if err != nil {
					sendEvent(Event{Type: Error, Data: makePlayError("Getting track info failed: ")})
					return nil
				}

				info := &Info{BitRate: int(i.bitrate), Rate: int(i.rate)}
				d, err := C.play_seconds_per_sample(reader)
				size := int(C.play_length(reader))
				if err == nil {
					info.Sps = float64(d)
					if size > 0 {
						info.Duration = float64(d) * float64(size)
					}
				} else {
					sendEvent(Event{Type: Error, Data: makePlayError("Getting seconds-per-sample failed: ")})
				}

				return info
			} else {
				return nil
			}
		}

		// Stop the currently playing mp3 if it's playing, and unload it.
		stop := func() {
			if state != Empty {
				path = ""
				C.play_delete_reader(reader)
				reader = nil
				deleteWriter()

				state = Empty
				currentTrackInfo = nil
			}
		}

		// Load a new mp3. Returns true if the state has changed.
		load := func(cmd loadCmd) {
			if state != Empty {
				stop()
			}

			lastoff = -1

			path = cmd.path
			reader = C.play_new_reader(C.CString(path))
			if reader == nil {
				close(cmd.size)
				cmd.err <- makePlayError("Creating reader failed: ")
				return
			}

			err := makeWriter()
			if err != nil {
				close(cmd.size)
				C.play_delete_reader(reader)
				reader = nil
				cmd.err <- err
				return
			}

			cmd.size <- int(C.play_length(reader))

			cmd.err <- nil
			state = Paused

			// Cache the info about the current track
			currentTrackInfo = getInfo()
		}

		// Play the loaded mp3.
		play := func(cmd playCmd) {
			if state != Empty {
				if writer == nil {
					// If we were paused we need to recreate the writer.
					err := makeWriter()
					if err != nil {
						cmd <- err
						return
					}
				}
				state = Playing
			}
			cmd <- nil
		}

		// Pause the playing mp3.
		pause := func() {
			if state != Empty {
				state = Paused

				// We delete the writer here to release libao. This is so we don't have
				// the audio device locked so that other applications that require sound
				// can play sound.
				if writer != nil {
					deleteWriter()
				}
			}
		}

		// Seek to a position in the loaded mp3.
		seek := func(cmd seekCmd) {
			if state == Empty {
				return
			}

			rc := C.play_seek(reader, C.int(cmd))
			if rc < 0 {
				sendEvent(Event{Type: Error, Data: makePlayError("Seeking failed: ")})
				return
			}

			// Zero out lastofftime
			var zero time.Time
			lastofftime = zero

			sendEvent(Event{Type: OffsetChange, Data: int(cmd)})
		}

		handleCommonCmds := func(cmd interface{}) bool {
			switch cmd.(type) {
			case setVolumeCmd:
				err := SetVolume(byte(cmd.(setVolumeCmd)))
				if err == nil {
					sendEvent(Event{Type: VolumeChange, Data: byte(cmd.(setVolumeCmd))})
					if debug {
						fmt.Println("player: sending VolumeChange event")
					}
				} else {
					sendEvent(Event{Type: Error, Data: err})
				}
				return true
			case setVolumeAllCmd:
				err := SetVolumeAll(byte(cmd.(setVolumeAllCmd)))
				if err == nil {
					sendEvent(Event{Type: VolumeChange, Data: byte(cmd.(setVolumeCmd))})
					if debug {
						fmt.Println("player: sending VolumeChange event")
					}
				} else {
					sendEvent(Event{Type: Error, Data: err})
				}
				return true
			case getStatusCmd:
				timer := time.Now()
				offset := 0
				size := 0
				if state != Empty {
					offset = int(C.play_offset(reader))
					if offset < 0 {
						sendEvent(Event{Type: Error, Data: makePlayError("Seeking failed: ")})
					}
					size = int(C.play_length(reader))
					if size < 0 {
						sendEvent(Event{Type: Error, Data: makePlayError("Length failed: ")})
					}
				}
				volume, err := GetVolume()
				if err != nil {
					sendEvent(Event{Type: Error, Data: err})
				}
				cmd.(getStatusCmd) <- PlayerStatus{Offset: offset, Size: size, State: state, Volume: volume, Path: path}
				if debug {
					fmt.Printf("player: Generating status took %v\n", time.Now().Sub(timer))
				}
				return true
			case getInfoCmd:
				if state != Empty {
					timer := time.Now()

					cmd.(getInfoCmd) <- currentTrackInfo
					fmt.Printf("player: sending getInfoCmd took %v\n", time.Now().Sub(timer))
				} else {
					cmd.(getInfoCmd) <- nil
				}
				return true
			case setRepeatCmd:
				repeat = bool(cmd.(setRepeatCmd))
			}
			return false
		}

		states[Empty] = func() {
			// Only the load command is not ignored.
			for {
				select {
				case cmd := <-p.cmds:
					debugCmd(cmd)
					if handleCommonCmds(cmd) {
						break
					}

					switch cmd.(type) {
					case loadCmd:
						load(cmd.(loadCmd))
					}
				}

				if state != Empty {
					break
				}
			}
		}

		states[Paused] = func() {
			for {
				select {
				case cmd := <-p.cmds:
					debugCmd(cmd)
					if handleCommonCmds(cmd) {
						break
					}

					switch cmd.(type) {
					case loadCmd:
						load(cmd.(loadCmd))
					case playCmd:
						play(cmd.(playCmd))
					case stopCmd:
						stop()
					case seekCmd:
						seek(cmd.(seekCmd))
					}
				}

				if state != Paused {
					break
				}
			}
		}

		states[Playing] = func() {
		outer:
			for {
				// Handle all commands before copying song frames since copying is slow.
				wasCmd := true
				for {
					select {
					case cmd := <-p.cmds:
						debugCmd(cmd)
						if handleCommonCmds(cmd) {
							break
						}

						switch cmd.(type) {
						case loadCmd:
							load(cmd.(loadCmd))
						case pauseCmd:
							pause()
						case stopCmd:
							stop()
						case seekCmd:
							seek(cmd.(seekCmd))
						}
					default:
						wasCmd = false
					}

					if state != Playing {
						break outer
					}

					if !wasCmd {
						break
					}
				}

				timer := time.Now()

				// Copy a buffer of data to the output device
				n, err := C.play_read(reader)
				if err != nil {
					// We're done
					if repeat {
						seek(0)
						continue
					} else {
						stop()
						break
					}
				}

				rc := C.play_write(writer, reader.buffer, n)
				if rc < 0 {
					sendEvent(Event{Type: Error, Data: makePlayError("Writing to soundcard failed: ")})
				}

				if debug {
					fmt.Printf("player: copying a buffer of data took %v\n", time.Now().Sub(timer))
				}

				if lastofftime.IsZero() || time.Now().Sub(lastofftime) > time.Millisecond*250 {
					if o := int(C.play_offset(reader)); o != lastoff {
						timer = time.Now()
						sendEvent(Event{Type: OffsetChange, Data: o})
						if debug {
							fmt.Printf("player: sending offsetchange event for offset %v took %v\n", o, time.Now().Sub(timer))
						}
					}
				}
			}
		}

		// Player state machine:
		for {
			if debug {
				fmt.Println("player: Entering state", state)
			}
			lastState := state
			states[state]()
			if lastState != state {
				sendEvent(Event{Type: StateChange, Data: state})
			}
		}

	}()

	return p
}

// Load the specified file into the Player. Call Play to play the file.
// Returns the size of the file in samples. On failure, err is non nil.
// On success, the player will write the current playing offset (in samples) to offchan
// periodically.
func (p Player) Load(filename string) (size int, err error) {
	ch := make(chan error)
	sizech := make(chan int)

	p.cmds <- loadCmd{path: filename, err: ch, size: sizech}

	size = <-sizech
	err = <-ch

	return
}

// Play the currently loaded file. The player must have an mp3 loaded and not currently playing.
func (p Player) Play() (err error) {
	playCmd := make(playCmd)

	p.cmds <- playCmd

	err = <-playCmd

	return
}

// Pause the currently playing file.
func (p Player) Pause() {
	p.cmds <- pauseCmd(true)
}

// Stop the currently playing file if it's playing, and unload the file from the mp3 player.
func (p Player) Stop() {
	p.cmds <- stopCmd(true)
}

// Seek to the specified sample in the loaded mp3 file.
func (p Player) Seek(offset int) {
	p.cmds <- seekCmd(offset)
}

// SetVolume calls the regular SetVolume function but also writes a VolumeChange event to the
// Player's Event channel.
func (p Player) SetVolume(pct byte) {
	p.cmds <- setVolumeCmd(pct)
}

// SetVolumeAll calls the regular SetVolumeAll function but also writes a VolumeChange event to the
// Player's Event channel.
func (p Player) SetVolumeAll(pct byte) {
	p.cmds <- setVolumeAllCmd(pct)
}

// GetStatus gets the player status
func (p Player) GetStatus() PlayerStatus {
	cmd := make(chan PlayerStatus)
	p.cmds <- getStatusCmd(cmd)

	return <-cmd
}

// GetInfo returns an Info object about the loaded mp3. If an mp3 is not loaded, nil is returned
func (p Player) GetInfo() *Info {
	cmd := make(chan *Info)
	p.cmds <- getInfoCmd(cmd)

	return <-cmd
}

// SetRepeat sets whether the track should be repeated when we reach the end or not.
func (p Player) SetRepeat(r bool) {
	p.cmds <- setRepeatCmd(r)
}
