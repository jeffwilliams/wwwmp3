package main

import "github.com/jeffwilliams/statetrc"

type Tracer string

func TraceEnter(name string, props []interface{}) Tracer {
	return Tracer(name).Enter(props)
}

func (r Tracer) Enter(props []interface{}) Tracer {
	statetrc.Enter(string(r), props)
	return r
}

func (r Tracer) Leave() {
	statetrc.Leave(string(r))
}
