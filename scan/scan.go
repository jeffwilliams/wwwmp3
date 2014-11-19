package scan

import (
	"bytes"
	"fmt"
	"github.com/jeffwilliams/wwwmp3/play"
	"os"
	"regexp"
	"strings"
)

type Metadata struct {
	play.Metadata
	Path string
}

func (m Metadata) String() string {
	var b bytes.Buffer
	b.Write([]byte("path: '"))
	b.Write([]byte(m.Path))
	b.Write([]byte("' artist: '"))
	b.Write([]byte(m.Artist))
	b.Write([]byte("' album: '"))
	b.Write([]byte(m.Album))
	b.Write([]byte("' title: '"))
	b.Write([]byte(m.Title))
	b.Write([]byte("'"))
	return b.String()
}

/* Scan a directory structure for files. Pass all files to the files chan */
func Scan(basedir string, files chan string) {
	defer func() {
		close(files)
	}()

	var scan func(dir string)

	scan = func(dir string) {
		file, err := os.Open(dir)
		if err != nil {
			fmt.Println("Opening directory", basedir, "failed:", err)
			return
		}

		for {
			fi, err := file.Readdir(10)

			if err != nil {
				break
			}

			for _, fin := range fi {
				if fin.IsDir() {
					scan(dir + "/" + fin.Name())
				} else {
					files <- dir + "/" + fin.Name()
				}
			}
		}
	}

	scan(basedir)
}

var mp3Regexp *regexp.Regexp = regexp.MustCompile(`\.[mM][pP]3$`)

func ScanMp3s(basedir string, meta chan Metadata) {
	c := make(chan string)

	defer func() {
		close(meta)
	}()

	go Scan(basedir, c)

	for f := range c {
		if mp3Regexp.MatchString(f) {
			m := play.GetMetadata(f)
			rectify(&m)
			f = strings.Replace(f, "//", "/", -1)
			meta <- Metadata{m, f}
		}
	}
}

func rectify(m *play.Metadata) {
	if len(m.Title) == 0 {
		m.Title = "Unknown"
	}
	if len(m.Artist) == 0 {
		m.Artist = "Unknown"
	}
	if len(m.Album) == 0 {
		m.Album = "Unknown"
	}
}
