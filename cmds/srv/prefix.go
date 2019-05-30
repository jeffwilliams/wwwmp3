package main

import (
	"path"
	"strings"
)

type Prefix string

// apply prepends the prefix to the passed file path
func (p Prefix) apply(filepath string) string {
	if len(p) == 0 {
		return filepath
	}
	return path.Join(string(p), filepath)
}

func (p Prefix) remove(filepath string) string {
	if len(p) > 0 && strings.HasPrefix(filepath, string(p)) {
		return filepath[len(p):]
	}
	return filepath
}
