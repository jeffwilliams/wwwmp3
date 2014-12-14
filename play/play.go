// Package play implements a simple mp3 player.
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
	"strings"
	"time"
	"unsafe"
)

// For development
var _ = fmt.Println

// Package init function
func init() {
	C.play_init()
}

// Play the specified file. Return when playback is complete.
func Play(filename string) {
	n := C.CString(filename)
	C.play_play(n)
	C.free(unsafe.Pointer(n))
}

// Set the volume as a percentage between 0 and 100 inclusive.
// This method sets the volume on the default ALSA card.
func SetVolume(pct byte) {
	C.play_setvolume(C.uchar(pct), C.CString("default"))
}

// Set the volume as a percentage between 0 and 100 inclusive.
// This method sets the volume on all ALSA cards.
func SetVolumeAll(pct byte) {
	C.play_setvolume_all(C.uchar(pct))
}

// Get the volume as a percentage between 0 and 100 inclusive.
func GetVolume() byte {
	return byte(C.play_getvolume())
}

// Metadata is information about an mp3.
type Metadata struct {
	Title  string
	Artist string
	Album  string
}

// GetMetadata extracts the id3 information from the mp3 file `filename`.
func GetMetadata(filename string) Metadata {
	meta := C.play_meta(C.CString(filename))
	r := Metadata{
		Title:  strings.Trim(C.GoString(meta.title), " "),
		Artist: strings.Trim(C.GoString(meta.artist), " "),
		Album:  strings.Trim(C.GoString(meta.album), " "),
	}
	C.play_delete_meta(meta)
	return r
}

// Print debugging information about the metadata in `filename` to stdout.
func DebugMetadata(filename string) {
	C.play_debug_meta(C.CString(filename))
}

const (
	// The player has no mp3 loaded
	Empty = iota
	// The player has an mp3 loaded and is playing it
	Playing
	// The player has an mp3 loaded and it is paused
	Paused
)

// PlayerState represents the current state of a Player. Must be one of Empty, Playing, or Paused.
type PlayerState int

// Player is an mp3 player.
type Player chan command

const (
	cmdLoad = iota
	cmdPlay
	cmdPause
	cmdStop
	cmdSeek
)

type command struct {
	id     int
	path   string
	offset int
	err    chan error
	// The player writes offsets to this channel periodically, if it is not nil
	offchan chan int
	// on load, this channel is written the size of the mp3 in samples
	size chan int
}

// Create a new Player.
func NewPlayer() Player {

	p := make(chan command)

  // This goroutine implements the player.
	go func() {
		var reader *C.play_reader_t
		var writer *C.ao_device
		var offchan chan int
		var lastofftime time.Time
		lastoff := -1

		var state PlayerState = Empty
    var states [3]func()

    // Stop the currently playing mp3 if it's playing, and unload it.
		stop := func() {
			if state != Empty {
				C.play_delete_reader(reader)
				reader = nil
				C.play_delete_writer(writer)
				writer = nil

				if offchan != nil {
					close(offchan)
				}

				state = Empty
			}
		}

    // Load a new mp3. Returns true if the state has changed.
		load := func(cmd command) {
			if state != Empty {
				stop()
			}

			lastoff = -1

			reader = C.play_new_reader(C.CString(cmd.path))
			if reader == nil {
				close(cmd.size)
				cmd.err <- errors.New("Creating reader failed")
				return
			}

			writer = C.play_new_writer(reader)
			if writer == nil {
				close(cmd.size)
				cmd.err <- errors.New("Creating writer failed")
				C.play_delete_reader(reader)
				reader = nil
				return
			}

			cmd.size <- int(C.play_length(reader))
			offchan = cmd.offchan

			cmd.err <- nil
			state = Paused
		}

    // Play the loaded mp3.
		play := func() {
			if state != Empty {
				state = Playing
			}
		}

    // Pause the playing mp3.
		pause := func() {
			if state != Empty {
				state = Paused
			}
		}

    // Seek to a position in the loaded mp3.
		seek := func(cmd command) {
			if state == Empty {
				return
			}

			C.play_seek(reader, C.int(cmd.offset))
			// Zero out lastofftime
			var zero time.Time
			lastofftime = zero
		}

    states[Empty] = func(){
      // Only the load command is not ignored.
      for {
			  select {
        case cmd := <-p:
          switch cmd.id {
          case cmdLoad:
					  load(cmd)
          }
        }

        if state != Empty {
          break
        }
      }
    }

    states[Paused] = func(){
      for {
			  select {
        case cmd := <-p:
          switch cmd.id {
          case cmdLoad:
					  load(cmd)
          case cmdPlay:
            play()
          case cmdStop:
            stop()
          case cmdSeek:
            seek(cmd)
          }
        }

        if state != Paused {
          break
        }
      }
    }

    states[Playing] = func(){
      for {
			  select {
        case cmd := <-p:
          switch cmd.id {
          case cmdLoad:
					  load(cmd)
				  case cmdPause:
					  pause()
          case cmdStop:
            stop()
          case cmdSeek:
            seek(cmd)
          }
          default:
        }

        if state != Playing {
          break
        }

        // Copy a buffer of data to the output device
        n, err := C.play_read(reader)
        if err != nil {
          // We're done
          stop()
          break
        }

        C.play_write(writer, reader.buffer, n)

        if offchan != nil {
          if lastofftime.IsZero() || time.Now().Sub(lastofftime) > time.Millisecond*100 {
            if o := int(C.play_offset(reader)); o != lastoff {
              select {
              case offchan <- o:
                lastofftime = time.Now()
              default:
              }
            }
          }
        }
      }
    }

    // Player state machine:
		for {
      states[state]()
		}

	}()

	return p
}

// Load the specified file into the Player. Call Play to play the file.
// Returns the size of the file in samples. On failure, err is non nil.
// On success, the player will write the current playing offset (in samples) to offchan 
// periodically.
func (p Player) Load(filename string, offchan chan int) (size int, err error) {
	ch := make(chan error)
	sizech := make(chan int)

	p <- command{id: cmdLoad, path: filename, err: ch, offchan: offchan, size: sizech}

	size = <-sizech
	err = <-ch

	return
}

// Play the currently loaded file. The player must have an mp3 loaded and not currently playing.
func (p Player) Play() {
	p <- command{id: cmdPlay}
}

// Pause the currently playing file. 
func (p Player) Pause() {
	p <- command{id: cmdPause}
}

// Stop the currently playing file if it's playing, and unload the file from the mp3 player.
func (p Player) Stop() {
	p <- command{id: cmdStop}
}

// Seek to the specified sample in the loaded mp3 file.
func (p Player) Seek(offset int) {
	p <- command{id: cmdSeek, offset: offset}
}
