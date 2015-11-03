package main

import "container/list"

type Recent struct {
	List list.List
	Held string
	Max  int
}

// Hold puts the specified path in a temporary variable. When Commit
// is called, the held path is added to the recent list.
func (r *Recent) Hold(path string) {
	r.Held = path
}

// Commit adds the held path to the list.
func (r *Recent) Commit() {
	if len(r.Held) > 0 {
		r.add(r.Held)
		r.Held = ""
	}
}

func (r *Recent) add(path string) {
	if b, e := r.contains(path); b {
		r.List.MoveToFront(e)
		return
	}

	max := r.Max
	if max == 0 {
		max = 10
	}

	r.List.PushFront(path)
	if r.List.Len() > max {
		r.List.Remove(r.List.Back())
	}
}

func (r Recent) contains(path string) (bool, *list.Element) {
	for e := r.List.Front(); e != nil; e = e.Next() {
		if e.Value == path {
			return true, e
		}
	}
	return false, nil
}

func (r Recent) Slice() []string {
	result := make([]string, r.List.Len())
	i := 0
	for e := r.List.Front(); e != nil; e = e.Next() {
		result[i] = e.Value.(string)
		i += 1
	}
	return result
}
