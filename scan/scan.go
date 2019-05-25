// Package scan implements a simple scanner and database for mp3 id3 metainformation.
package scan

import (
	"bytes"
	"github.com/jeffwilliams/wwwmp3/play"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Metadata represents mp3 id3 metainformation as well as a filesystem path to the mp3 file.
type Metadata struct {
	play.Metadata
	Path string
}

// Return a human readable version of the metadata.
func (m Metadata) String() string {
	var b bytes.Buffer
	b.Write([]byte("path: '"))
	b.Write([]byte(m.Path))
	b.Write([]byte("' artist: '"))
	b.Write([]byte(m.Artist))
	b.Write([]byte("' album: '"))
	b.Write([]byte(m.Album))
	b.Write([]byte("' tracknum: '"))
	b.Write([]byte(strconv.Itoa(m.Tracknum)))
	b.Write([]byte("' title: '"))
	b.Write([]byte(m.Title))
	b.Write([]byte("'"))
	return b.String()
}

// Scan scans a directory tree for files and passes the full path of all files to the `files` chan.
// If reading basedir fails, an error is returned.
func Scan(path string, files chan string) error {
	defer func() {
		close(files)
	}()

	// Special case: user passed a file
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		files <- path
		return nil
	}

	var scan func(dir string) error

	scan = func(dir string) error {

		file, err := os.Open(dir)
		if err != nil {
			//return fmt.Errorf("Opening directory %v failed: %v", dir, err)
			// TODO: Log a message here.
		}

		for {
			fi, err := file.Readdir(10)

			if err != nil {
				break
			}

			for _, fin := range fi {
				if fin.IsDir() {
					// Ignore errors reading subdirectories
					scan(dir + "/" + fin.Name())
				} else {
					files <- dir + "/" + fin.Name()
				}
			}
		}

		return nil
	}

	return scan(path)
}

var mp3Regexp *regexp.Regexp = regexp.MustCompile(`\.[mM][pP]3$`)

// ScanMp3s scans a directory tree for mp3 files. It passes the mp3 Metadata to the chan `meta`.
// As a special case, if path is a file it alone is scanned.
func ScanMp3s(path string, meta chan Metadata) {
	c := make(chan string)

	defer func() {
		close(meta)
	}()

	go Scan(path, c)

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
