package play

/*
#include <stdlib.h>
#include "play.h"

#cgo LDFLAGS: -lmpg123 -lao -lasound -lid3
*/
import "C"

import (
  "unsafe"
  "strings"
  "errors"
  "fmt"
  "time"
)

// For development
var _ = fmt.Println

// Package init function
func init() {
	C.play_init()
}

/*
Play the specified file. Return when playback is complete.
*/
func Play(filename string) {
	n := C.CString(filename)
	C.play_play(n)
	C.free(unsafe.Pointer(n))
}

/*
Set the volume as a percentage between 0 and 100 inclusive.
*/
func SetVolume(pct byte) {
	C.play_setvolume(C.uchar(pct))
}

/*
Get the volume as a percentage between 0 and 100 inclusive.
*/
func GetVolume() byte {
	return byte(C.play_getvolume())
}

type Metadata struct {
	Title  string
	Artist string
	Album  string
}

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

type PlayerState int

type Player chan command

const (
  CmdLoad = iota
  CmdPlay
  CmdPause
  CmdStop
  CmdSeek
)

type command struct {
  id int
  path string
  offset int
  err chan error
  // The player writes offsets to this channel periodically, if it is not nil
  offchan chan int
  // on load, this channel is written the size of the mp3 in samples
  size chan int
}

func NewPlayer() Player {

  p := make(chan command)

  go func(){
    var state PlayerState = Empty
    var reader *C.play_reader_t
    var writer *C.ao_device
    var offchan chan int
    var lastofftime time.Time
    lastoff := -1

    stop := func(){
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

    load := func(cmd command){
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

    play := func(){
      if state != Empty {
        state = Playing
      }
    }

    pause := func(){
      if state != Empty {
        state = Paused
      }
    }

    seek := func(cmd command){
      if state == Empty {
        return
      }

      C.play_seek(reader, C.int(cmd.offset))
      // Zero out lastofftime
      var zero time.Time
      lastofftime = zero
    }

    for {
      // Check for commands
      select{
      case cmd := <-p:
        switch cmd.id {
        case CmdLoad:
          load(cmd)
        case CmdPlay:
          play()
        case CmdPause:
          pause()
        case CmdStop:
          stop()
        case CmdSeek:
          seek(cmd)
        }
      default:
      }

      // If we are playing, copy a buffer of data to the output device
      if state == Playing {
        n, err := C.play_read(reader)
        if err != nil {
          // We're done
          C.play_delete_reader(reader)
          C.play_delete_writer(writer)
          state = Empty
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
  }()

  return p
}

func (p Player) Load(filename string, offchan chan int) (size int, err error) {
  ch := make(chan error)
  sizech := make(chan int)

  p <- command{id: CmdLoad, path: filename, err: ch, offchan: offchan, size: sizech}

  size = <-sizech
  err = <-ch

  return
}

func (p Player) Play() {
  p <- command{id: CmdPlay}
}

func (p Player) Pause() {
  p <- command{id: CmdPause}
}

func (p Player) Stop() {
  p <- command{id: CmdStop}
}

func (p Player) Seek(offset int) {
  p <- command{id: CmdSeek, offset: offset}
}
