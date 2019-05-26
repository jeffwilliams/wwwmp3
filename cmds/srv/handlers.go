package main

import (
	"encoding/json"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// queryVal returns the first value for the GET query variable with name `key`.
func queryVal(r *http.Request, key string) (rs string) {
	v, ok := r.URL.Query()[key]
	if ok {
		rs = v[0]
	}
	return
}

// Respond to requests for mp3 metadata: lists of artists, titles, song paths, etc.
func serveMeta(w http.ResponseWriter, r *http.Request) {
	timer := time.Now()

	// Parse a query value which is a comma separated list. If the value doesn't appear, return the empty list
	queryList := func(key string) (l []string) {
		if s := queryVal(r, key); len(s) > 0 {
			l = strings.Split(s, ",")
		}
		return
	}

	if r.Method == "GET" {
		// Query string Format:
		//  filter:
		//    artist=blah&album=blah&title=blah
		//  paging:
		//    page=0&pagesize=10
		//  fields (not present or the following:)
		//    fields=artist,album,...
		//  order (not present or the following:)
		//    order=artist,album,...

		// Calculate the filter
		filt := make(map[string]string)
		if v := queryVal(r, "artist"); len(v) > 0 {
			filt["artist"] = v
		}
		if v := queryVal(r, "album"); len(v) > 0 {
			filt["album"] = v
		}
		if v := queryVal(r, "title"); len(v) > 0 {
			filt["title"] = v
		}

		page, err := strconv.Atoi(queryVal(r, "page"))
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("400 Bad Request: The 'page' parameter is missing or invalid."))
		}

		pageSize, err := strconv.Atoi(queryVal(r, "pagesize"))
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("400 Bad Request: The 'pagesize' parameter is missing or invalid."))
		}

		fields := queryList("fields")

		order := queryList("order")

		ch := make(chan map[string]string)

		go scan.FindMp3sInDb(
			db,
			fields,
			filt,
			order,
			ch,
			&scan.Paging{PageSize: pageSize, Page: page},
			nil)

		enc := json.NewEncoder(w)
		w.Write([]byte("[\n"))
		j := 0
		for m := range ch {
			if j > 0 {
				w.Write([]byte(",\n"))
			}

			enc.Encode(m)
			j++
		}
		w.Write([]byte("\n]\n"))
	}

	d := time.Now().Sub(timer)
	log.Info("serveMeta completed in %v", d)
}

// Perform functions on the mp3 player like play and pause.
func servePlayer(w http.ResponseWriter, r *http.Request) {
	logPrefix := "servePlayer: " + r.Method + " " + r.URL.Path + " - "
	log.Notice("%s requested", logPrefix)

	if r.Method == "GET" {
		if r.URL.Path == "/player/play" {
			log.Notice("%s play", logPrefix)
			err := player.Play()
			if err != nil {
				log.Error("%s player.Play() returned error: %v", logPrefix, err)
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			}
		} else if r.URL.Path == "/player/pause" {
			player.Pause()
		} else if r.URL.Path == "/player/stop" {
			player.Stop()
		} else if r.URL.Path == "/player/volume" {
			v, err := play.GetVolume()
			if err != nil {
				log.Error("%s play.GetVolume() returned error: %v", logPrefix, err)
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
				return
			}
			w.Write([]byte("{\"volume\": "))
			w.Write([]byte(strconv.Itoa(int(v))))
			w.Write([]byte("}"))
			log.Notice("%s returning %d", logPrefix, int(v))
		}

	} else if r.Method == "POST" {
		decoder := (*json.Decoder)(nil)
		if r.Body != nil {
			decoder = json.NewDecoder(r.Body)
		} else {
			log.Error("%s posted with nil body.", logPrefix)
			w.WriteHeader(400)
			w.Write([]byte("400 Bad Request: no body"))
			return
		}

		decodeReq := func(i interface{}) bool {
			err := decoder.Decode(i)
			if err != nil {
				log.Error("%s decoding request failed: %v", logPrefix, err)
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: invalid JSON"))
				return false
			}
			return true
		}

		if r.URL.Path == "/player/queue.enqueue" {
			req := struct {
				File string
			}{}

			if !decodeReq(&req) {
				return
			}

			queue.Enqueue(req.File)
		} else if r.URL.Path == "/player/queue.move" {
			req := struct {
				Indexes []int
				Delta   int
			}{}

			if !decodeReq(&req) {
				return
			}

			queue.Move(req.Indexes, req.Delta)
		} else if r.URL.Path == "/player/queue.move_to_top" {
			req := struct {
				Indexes []int
			}{}

			if !decodeReq(&req) {
				return
			}

			queue.MoveToTop(req.Indexes)
		} else if r.URL.Path == "/player/queue.remove" {
			req := struct {
				Indexes []int
			}{}

			if !decodeReq(&req) {
				return
			}

			queue.Remove(req.Indexes)
		} else if r.URL.Path == "/player/volume" {
			req := struct {
				Volume int
			}{}

			if !decodeReq(&req) {
				return
			}

			player.SetVolume(byte(req.Volume))
		} else if r.URL.Path == "/player/seek" {
			req := struct {
				Seek int
			}{}

			if !decodeReq(&req) {
				return
			}

			log.Notice("%s seek %v", logPrefix, req.Seek)
			player.Seek(req.Seek)
		} else if r.URL.Path == "/player/repeat_mode" {
			req := struct {
				Mode string
			}{}

			if !decodeReq(&req) {
				return
			}

			r, err := ParseRepeatMode(req.Mode)
			if err != nil {
				log.Warning("%s error parsing repeat mode: %v", logPrefix, err)
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: Bad repeat mode"))
				return
			}

			repeatMode = r

			if repeatMode == RepeatOne {
				player.SetRepeat(true)
			} else {
				player.SetRepeat(false)
			}
			repeatModeTee.In <- struct{}{}
		}
	}
}

func enterScanning() bool {
	scanMutex.Lock()
	defer scanMutex.Unlock()

	if scanning {
		return false
	} else {
		scanning = true
		return true
	}
}

// Perform functions related to mp3 scanning
func serveScan(w http.ResponseWriter, r *http.Request) {
	logPrefix := "serveScan: " + r.Method + " " + r.URL.Path + " - "
	log.Notice("%s requested", logPrefix)

	if r.Method == "GET" {
		if r.URL.Path == "/scan/all" {
			if mp3Dirs != nil {
				scanMutex.Lock()
				defer scanMutex.Unlock()
				if !scanning {
					scanning = true
					go scanDirs(mp3Dirs)
				}
			} else {
				log.Notice("serveScan: scan requested but no directories to scan")
				w.WriteHeader(404)
				w.Write([]byte("404 Not Found: no directories to scan"))
			}
		}
	} else if r.Method == "POST" {
		decoder := (*json.Decoder)(nil)
		if r.Body != nil {
			decoder = json.NewDecoder(r.Body)
		} else {
			log.Error("%s posted with nil body.", logPrefix)
			w.WriteHeader(400)
			w.Write([]byte("400 Bad Request: no body"))
			return
		}

		decodeReq := func(i interface{}) bool {
			err := decoder.Decode(i)
			if err != nil {
				log.Error("%s decoding request failed: %v", logPrefix, err)
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: invalid JSON"))
				return false
			}
			return true
		}

		if r.URL.Path == "/scan/path" {
			req := struct {
				Path string
			}{}

			if !decodeReq(&req) {
				return
			}

			if enterScanning() {
				go scanDirs([]string{req.Path})
			}
		}
	}
}

/*
serveWebsock implements a websocket connection with the browser, over which events
generated by the player are sent.

The messages sent to the browser are JSON and have the form:

{
  Offset: 0,
  Size: 0,
  State: 0,
  Volume: 0,
  Meta: {
    artist: "s",
    album: "s",
    title: "s",
    path: "s"
    }
}

State is an enumeration with the values:
  0 = empty
  1 = playing
  2 = paused

Meta may be null if there is no loaded mp3.
*/
func serveWebsock(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	userAgent := r.Header.Get("User-Agent")

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("Error upgrading connection to websock: %v", err)
		return
	}

	log.Notice("Websocket connection from %s %v", ws.RemoteAddr(), userAgent)

	defer log.Notice("Websocket handler for %s exiting", ws.RemoteAddr())

	// Send the full status to the browser
	d, err := jsonFullStatus(player.GetStatus(), meta, listQueue(queue), pathsToMetadatas(recent.Slice()), repeatMode)
	if err != nil {
		log.Error("Error encoding Player event as JSON: %v", err)
		return
	}
	log.Notice("Writing complete status to websocket %s", ws.RemoteAddr())
	err = websockWrite(ws, d)
	if err != nil {
		log.Error("Error writing to websocket %s: %v. Handler for socket is terminating.", ws.RemoteAddr(), err)
		// Client is probably gone. Close our channel and exit.
		return
	}

	// Add a new channel to the tee that we will use to detect changes
	c := make(chan interface{})
	eventTee.Add(c)
	defer eventTee.Del(c)

	scanEvents := make(chan interface{})
	scanTee.Add(scanEvents)
	defer scanTee.Del(scanEvents)

	repeatModeChanged := make(chan interface{})
	repeatModeTee.Add(repeatModeChanged)
	defer repeatModeTee.Del(repeatModeChanged)

	//ws.SetReadDeadline(time.Now().Add(10 * time.Millisecond))

loop:
	for {
		select {
		case e, ok := <-c:
			if !ok {
				// Player died...
				log.Error("Player closed event channel. Closing websocket %v", ws.RemoteAddr())
				ws.Close()
				break loop
			}

			if !websockHandlePlayerEvent(ws, e.(play.Event)) {
				break loop
			}

		case e, ok := <-scanEvents:
			if !ok {
				// Something closed the scanner channel.
				log.Error("Scanner channel was closed. Closing websocket %v", ws.RemoteAddr())
				ws.Close()
				break loop
			}

			d, err = jsonScan(e.(*scan.Metadata))
			if err != nil {
				log.Error("Error encoding metadata as JSON: %v", err)
				// Hopefully the next one works...
				continue loop
			}
			err = websockWrite(ws, d)
			if err != nil {
				log.Error("Error writing to websocket %v: %v", ws.RemoteAddr(), err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		case _ = <-repeatModeChanged:
			m, err := jsonRepeat(repeatMode)
			if err != nil {
				log.Error("Error encoding metadata as JSON: %v", err)
				// Hopefully the next one works...
				continue loop
			}
			err = websockWrite(ws, m)
			if err != nil {
				log.Error("Error writing to websocket %v: %v", ws.RemoteAddr(), err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		}
	}
}
