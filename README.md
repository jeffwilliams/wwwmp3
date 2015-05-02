# wwwmp3: A Web-based mp3 player

An mp3 player for Linux with a Web frontend. Written in [Go](https://golang.org/). It can also be used as a simple Go library for playing mp3s. See the API documentation on [godoc](http://godoc.org/github.com/jeffwilliams/wwwmp3).

## Dependencies

wwwmp3 contains a bit of C and C++ code for using existing libraries. To compile you'll need:

  * libmpg123-dev
  * libao-dev
  * libasound-dev
  * libid3-dev

