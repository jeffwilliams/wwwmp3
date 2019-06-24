package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jeffwilliams/statetrc"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
)

// queryVal returns the first value for the GET query variable with name `key`.
func queryVal(r *http.Request, key string) (rs string) {
	v, ok := r.URL.Query()[key]
	if ok {
		rs = v[0]
	}
	return
}

// prependPrefix prepents the configured prefix to the MP3 path
func prependPrefix(fields map[string]string) {
	if v, ok := fields["path"]; ok {
		fields["path"] = prefix.apply(v)
	}
}

// Respond to requests for mp3 metadata: lists of artists, titles, song paths, etc.
func serveMeta(w http.ResponseWriter, r *http.Request) {
	timer := time.Now()

	trc := TraceEnter("/serveMeta", nil)
	defer trc.Leave()

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

		t := TraceEnter("/serveMeta/scan.FindMp3sInDb", nil)
		go scan.FindMp3sInDb(
			db,
			fields,
			filt,
			order,
			ch,
			&scan.Paging{PageSize: pageSize, Page: page},
			nil)
		t.Leave()

		enc := json.NewEncoder(w)
		w.Write([]byte("[\n"))
		j := 0
		for m := range ch {
			if j > 0 {
				w.Write([]byte(",\n"))
			}

			prependPrefix(m)

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

	trc := TraceEnter("/servePlayer", nil)
	defer trc.Leave()

	if r.Method == "GET" {
		if r.URL.Path == "/player/play" {
			log.Notice("%s play", logPrefix)

			t := TraceEnter("/servePlayer/player.Play", nil)
			err := player.Play()
			t.Leave()

			if err != nil {
				log.Error("%s player.Play() returned error: %v", logPrefix, err)
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			}
		} else if r.URL.Path == "/player/pause" {
			t := TraceEnter("/servePlayer/player.Pause", nil)
			player.Pause()
			t.Leave()
		} else if r.URL.Path == "/player/stop" {
			t := TraceEnter("/servePlayer/player.Stop", nil)
			player.Stop()
			t.Leave()
		} else if r.URL.Path == "/player/volume" {
			t := TraceEnter("/servePlayer/play.GetVolume", nil)
			v, err := play.GetVolume()
			t.Leave()
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

			log.Notice("Enqueuing file %s", req.File)

			t := TraceEnter("/servePlayer/queue.Enqueue", nil)
			queue.Enqueue(req.File)
			t.Leave()
		} else if r.URL.Path == "/player/queue.move" {
			req := struct {
				Indexes []int
				Delta   int
			}{}

			if !decodeReq(&req) {
				return
			}

			t := TraceEnter("/servePlayer/queue.Move", nil)
			queue.Move(req.Indexes, req.Delta)
			t.Leave()
		} else if r.URL.Path == "/player/queue.move_to_top" {
			req := struct {
				Indexes []int
			}{}

			if !decodeReq(&req) {
				return
			}

			t := TraceEnter("/servePlayer/queue.MoveToTop", nil)
			queue.MoveToTop(req.Indexes)
			t.Leave()
		} else if r.URL.Path == "/player/queue.remove" {
			req := struct {
				Indexes []int
			}{}

			if !decodeReq(&req) {
				return
			}

			t := TraceEnter("/servePlayer/queue.Remove", nil)
			queue.Remove(req.Indexes)
			t.Leave()
		} else if r.URL.Path == "/player/volume" {
			req := struct {
				Volume int
			}{}

			if !decodeReq(&req) {
				return
			}

			t := TraceEnter("/servePlayer/player.SetVolume", nil)
			player.SetVolume(byte(req.Volume))
			t.Leave()
		} else if r.URL.Path == "/player/seek" {
			req := struct {
				Seek int
			}{}

			if !decodeReq(&req) {
				return
			}

			log.Notice("%s seek %v", logPrefix, req.Seek)
			t := TraceEnter("/servePlayer/play.Seek", nil)
			player.Seek(req.Seek)
			t.Leave()
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

			t := TraceEnter("/servePlayer/player.SetRepeat", nil)
			if repeatMode == RepeatOne {
				player.SetRepeat(true)
			} else {
				player.SetRepeat(false)
			}
			t.Leave()

			t = TraceEnter("/servePlayer/repeatModeTee.In.write", nil)
			repeatModeTee.In <- struct{}{}
			t.Leave()
		}
	}
}

func enterScanning() bool {
	t := TraceEnter("/enterScanning", nil)
	scanMutex.Lock()
	defer t.Leave()
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
	trc := TraceEnter("/serveWebsock", nil)
	defer trc.Leave()

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

	traceId := fmt.Sprintf("/serveWebsock/%s", ws.RemoteAddr())
	t := TraceEnter(traceId, nil)
	defer t.Leave()

	defer log.Notice("Websocket handler for %s exiting", ws.RemoteAddr())
	defer ws.Close()

	// Send the full status to the browser
	d, err := jsonFullStatus(player.GetStatus(), meta, listQueue(queue), pathsToMetadatas(recent.Slice()), repeatMode)
	if err != nil {
		log.Error("Websock %v: Error encoding Player event as JSON: %v", ws.RemoteAddr(), err)
		return
	}
	log.Notice("Websock %v: Writing complete status to websocket", ws.RemoteAddr())
	err = websockWrite(ws, d)
	if err != nil {
		log.Error("Websock %v: Error writing to websocket: %v. Handler for socket is terminating.", ws.RemoteAddr(), err)
		// Client is probably gone. Close our channel and exit.
		return
	}

	// Add a new channel to the tee that we will use to detect changes
	c := make(chan interface{})
	t2 := TraceEnter("/eventTee.Add", nil)
	eventTee.Add(c)
	t2.Leave()

	defer func() {
		t := TraceEnter("/eventTee.Del", nil)
		eventTee.Del(c)
		t.Leave()
	}()

	scanEvents := make(chan interface{})
	t2 = TraceEnter("/scanTee.Add", nil)
	scanTee.Add(scanEvents)
	t2.Leave()

	defer func() {
		t := TraceEnter("/scanTee.Del", nil)
		scanTee.Del(scanEvents)
		t.Leave()
	}()

	repeatModeChanged := make(chan interface{})
	t2 = TraceEnter("/repeatModeTee.Add", nil)
	repeatModeTee.Add(repeatModeChanged)
	t2.Leave()

	defer func() {
		t := TraceEnter("/repeatModeTee.Del", nil)
		repeatModeTee.Del(repeatModeChanged)
		t.Leave()
	}()

	//ws.SetReadDeadline(time.Now().Add(10 * time.Millisecond))

loop:
	for {
		select {
		case e, ok := <-c:
			if !ok {
				// Player died...
				log.Error("Websock %v: Player closed event channel.", ws.RemoteAddr())
				break loop
			}

			if !websockHandlePlayerEvent(ws, e.(play.Event)) {
				break loop
			}

		case e, ok := <-scanEvents:
			if !ok {
				// Something closed the scanner channel.
				log.Error("Websock %v: Scanner channel was closed.", ws.RemoteAddr())
				ws.Close()
				break loop
			}

			d, err = jsonScan(e.(*scan.Metadata))
			if err != nil {
				log.Error("Websock %v: Error encoding metadata as JSON: %v", ws.RemoteAddr(), err)
				// Hopefully the next one works...
				continue loop
			}
			err = websockWrite(ws, d)
			if err != nil {
				log.Error("Websock %v: Error writing to websocket: %v", ws.RemoteAddr(), err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		case _ = <-repeatModeChanged:
			m, err := jsonRepeat(repeatMode)
			if err != nil {
				log.Error("Websock %v: Error encoding metadata as JSON: %v", ws.RemoteAddr(), err)
				// Hopefully the next one works...
				continue loop
			}
			err = websockWrite(ws, m)
			if err != nil {
				log.Error("Websock %v: Error writing to websocket: %v", ws.RemoteAddr(), err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		}
	}

	eventTee.Del(c)
	scanTee.Del(scanEvents)
	repeatModeTee.Del(repeatModeChanged)
}

// Respond to requests for internal debug tracing info
func serveTrace(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain")

	fmt.Fprintf(w, "%s", statetrc.List(statetrc.ById))
}
