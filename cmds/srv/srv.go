// Command srv implements a webserver that implements an mp3 player and hosts a UI for it.
//
// This command expects an mp3 database to be set up and filled.
package main

import (
	//"code.google.com/p/go.net/websocket"
	"github.com/gorilla/websocket"
	"database/sql"
	"flag"
	"fmt"
	"github.com/jeffwilliams/wwwmp3/play"
	"github.com/jeffwilliams/wwwmp3/scan"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
  "github.com/jeffwilliams/go-logging"
  "github.com/jeffwilliams/wwwmp3/tee"
)

var (
  helpFlag = flag.Bool("help", false, "Print help")
  dbflag = flag.String("db", "mp3.db", "database containing mp3 info")
  allVolFlag = flag.Bool("allvol", false, "If set to true, changing the volume affects all ALSA cards, not just the default.")
  db scan.Mp3Db

  // The mp3 player
  player play.Player = play.NewPlayer()

  // Metadata for the currently playing mp3
  meta map[string]string

  // When the metadata is changed a bool is written to this Tee.
  metaChangedTee = tee.New()

  upgrader websocket.Upgrader

  // Player events are written to this Tee
  eventTee = tee.New()

  log = logging.MustGetLogger("server")
)

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

	writeMapAsJson := func(w io.Writer, m map[string]string) {
		w.Write([]byte("  {"))
		i := 0
		for k, v := range m {
			if i > 0 {
				w.Write([]byte(", "))
			}
			w.Write([]byte(`"` + k + `": "` + strings.Replace(v, `"`, `\"`, -1) + `"`))
			i++
		}
		w.Write([]byte("}"))
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
			&scan.Paging{PageSize: pageSize, Page: page})

		w.Write([]byte("[\n"))
		j := 0
		for m := range ch {
			if j > 0 {
				w.Write([]byte(",\n"))
			}
			writeMapAsJson(w, m)
			j++
		}
		w.Write([]byte("\n]\n"))
	}

	d := time.Now().Sub(timer)
	log.Info("serveMeta completed in %v", d)
}

// findMp3ByPath returns the mp3 information for the mp3 with the specified path.
func findMp3ByPath(path string) map[string]string {
	ch := make(chan map[string]string)

	go scan.FindMp3sInDb(
		db,
		nil,
		map[string]string{"path": path},
		nil,
		ch,
		&scan.Paging{PageSize: 1, Page: 0})

	r := <-ch

	// Drain channel
	for _ = range ch {
	}

	return r
}

// Perform functions on the mp3 player like play and pause.
func servePlayer(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Query string Format:
		// Load an mp3:
		//  load=<path>
		// Play:
		//  play=play
		if v := queryVal(r, "load"); len(v) > 0 {
	    log.Notice("servePlayer: load %s", v)
			size, err := player.Load(v)
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
			} else {
				w.Write([]byte("{\"size\": "))
				w.Write([]byte(strconv.Itoa(size)))
				w.Write([]byte("}"))
			}
			// Set the current mp3 metadata
			meta = findMp3ByPath(v)
			if meta == nil {
				fmt.Println("Loaded mp3, but can't find metainformation for it...")
			} else {
        metaChangedTee.In <- true
			}
		} else if _, ok := r.URL.Query()["play"]; ok {
      log.Notice("servePlayer: play")
			player.Play()
		} else if _, ok := r.URL.Query()["pause"]; ok {
      log.Notice("servePlayer: pause")
			player.Pause()
		} else if _, ok := r.URL.Query()["stop"]; ok {
      log.Notice("servePlayer: stop")
			player.Stop()
			meta = nil
      metaChangedTee.In <- true
		} else if _, ok := r.URL.Query()["getvolume"]; ok {
			v := play.GetVolume()
			w.Write([]byte("{\"volume\": "))
			w.Write([]byte(strconv.Itoa(int(v))))
			w.Write([]byte("}"))
      log.Notice("servePlayer: getvolume: returning %d", int(v))
		} else if s := queryVal(r, "setvolume"); len(s) > 0 {
      log.Notice("servePlayer: setvolume %s", s)
			v, err := strconv.Atoi(s)
			if err != nil {
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: invalid numeric volume"))
			} else {
				if *allVolFlag {
					player.SetVolumeAll(byte(v))
				} else {
					player.SetVolume(byte(v))
				}
			}
		} else if s := queryVal(r, "seek"); len(s) > 0 {
      log.Notice("servePlayer: seek %s", s)
			v, err := strconv.Atoi(s)
			if err != nil {
				w.WriteHeader(400)
				w.Write([]byte("400 Bad Request: invalid numeric offset"))
			} else {
				player.Seek(v)
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
//func playerEvents(ws *websocket.Conn) {
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

  write := func(payload []byte) error {
    ws.SetWriteDeadline(time.Now().Add(10*time.Millisecond))
    return ws.WriteMessage(websocket.TextMessage, payload)
  }

	defer log.Notice("Websocket handler for %s exiting", ws.RemoteAddr())

	// Send the full status to the browser
	d, err := jsonFullStatus(player.GetStatus(), meta)
	if err != nil {
		log.Error("Error encoding Player event as JSON: %v", err)
		return
	}
	log.Notice("Writing complete status to websocket %s", ws.RemoteAddr())
	err = write(d)
	if err != nil {
		log.Error("Error writing to websocket %s: %v. Handler for socket is terminating.", ws.RemoteAddr(), err)
		// Client is probably gone. Close our channel and exit.
		return
	}

	// Add a new channel to the tee that we will use to detect changes
	c := make(chan interface{})
	eventTee.Add(c)
	defer eventTee.Del(c)

  metaChanged := make(chan interface{})
  metaChangedTee.Add(metaChanged)
  defer metaChangedTee.Del(metaChanged)

	//ws.SetReadDeadline(time.Now().Add(10 * time.Millisecond))

loop:
	for {
		select {
		case _ = <-metaChanged:
			d, err = jsonMeta(meta)
			if err != nil {
				log.Error("Error encoding metadata as JSON: %v", err)
				// Hopefully the next one works...
				continue loop
			}
	    log.Notice("Writing status to websocket %v", ws.RemoteAddr())
			err = write(d)
			if err != nil {
				log.Error("Error writing to websocket %v: %v", ws.RemoteAddr(), err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		case e, ok := <-c:
			if !ok {
				// Player died...
				ws.Close()
				break loop
			}

			// Write the player information relevant to the event to the browser
      s := player.GetStatus()
      if e.(play.Event) == play.StateChange && s.State == play.Playing {
        // If we just changed to Paused then we may have loaded a new song, so send all the information.
	      d, err = jsonFullStatus(s, meta)
      } else {
        d, err = jsonPlayerEvent(s, e.(play.Event))
      }

      /*
      if e.(play.Event) == play.StateChange && s.State == play.Empty {
        // Track stopped.
        // TODO: This variable is accessed by multiple goroutines; should be protected.
        // TODO: Also, this should be set to nil outside of the websocket handler, so in case
        // there are no browsers when the track stops, we would still set metainfo to nil.
        // TODO: Deadlock here: we write to Tee, but Tee is writing to us.
        meta = nil
        metaChangedTee.In <- true
      }
      */

			if err != nil {
				log.Error("Error encoding Player event as JSON: %v", err)
				// Hopefully the next one works...
				continue loop
			}
	    log.Notice("Writing status to websocket %v", ws.RemoteAddr())
			err = write(d)
			if err != nil {
				log.Error("Error writing to websocket %v", ws.RemoteAddr(), ":", err)
				// Client is probably gone. Close our channel and exit.
				break loop
			}
		}
	}
}

func openDb(path string) (mp3db scan.Mp3Db, err error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return
	}

	mp3db, err = scan.OpenMp3Db(db)
	if err != nil {
		return
	}

	return
}

func initLogging(){
  var format = logging.MustStringFormatter(
    "%{color}%{time:2006-01-02 15:04:05.000000} %{module}: %{level:.4s} %{color:reset} %{message}",
  )
  logging.SetFormatter(format)
}

func main() {
	flag.Parse()

	if *helpFlag {
		flag.PrintDefaults()
		os.Exit(0)
	}

  initLogging()

	var err error

	// Open database
	db, err = openDb(*dbflag)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	// Read events from the player and pass them to the tee
	go func() {
		for e := range player.Events {
			eventTee.In <- e
		}
	}()

	// Setup http server
	http.HandleFunc("/songmeta", serveMeta)
	http.HandleFunc("/player", servePlayer)
	//http.Handle("/playerEvents", websocket.Handler(playerEvents))
	http.HandleFunc("/playerEvents", serveWebsock)
	http.Handle("/", http.FileServer(http.Dir("src/github.com/jeffwilliams/wwwmp3/www")))

	log.Notice("Listening on http://localhost:2001/")

	err = http.ListenAndServe(":2001", nil)
	if err != nil {
		log.Fatal("ListenAndServe failed: %v", err)
		os.Exit(1)
	}
}
