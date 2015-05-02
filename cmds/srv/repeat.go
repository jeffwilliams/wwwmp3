package main

import "errors"

type RepeatMode int

const (
	DontRepeat RepeatMode = iota
	RepeatOne
	RepeatAll
)

func ParseRepeatMode(s string) (RepeatMode, error) {
	if s == "DontRepeat" {
		return DontRepeat, nil
	} else if s == "RepeatOne" {
		return RepeatOne, nil
	} else if s == "RepeatAll" {
		return RepeatAll, nil
	} else {
		return DontRepeat, errors.New("Invalid RepeatMode " + s)
	}
}

func (r RepeatMode) String() string {
	switch r {
	case DontRepeat:
		return "DontRepeat"
	case RepeatOne:
		return "RepeatOne"
	case RepeatAll:
		return "RepeatAll"
	default:
		return "DontRepeat"
	}
}
