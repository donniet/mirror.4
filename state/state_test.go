package state

import (
	"net/http"
	"testing"
)

type TestStruct struct {
	Integer     int    `json:"integer,omitempty"`
	String      string `json:"String"`
	Modify      string
	Struct      InnerStruct
	Slice       []int
	Map         map[string]int
	SlicePtr    []*int
	MapPtr      map[string]*int
	Ptr         *int
	nonExported string
}

type InnerStruct struct {
	Bool bool
}

func TestPointers(t *testing.T) {
	tester := &TestStruct{
		Ptr:      new(int),
		SlicePtr: make([]*int, 0),
		MapPtr:   make(map[string]*int),
	}
	x := 5
	tester.Ptr = &x
	tester.SlicePtr = append(tester.SlicePtr, &x)
	tester.MapPtr["five"] = &x

	s := NewServer(tester)

	if p, err := s.Get("Ptr"); err != nil {
		t.Error(err)
	} else if p == nil {
		t.Errorf("response is nil")
	} else if string(p) != "5" {
		t.Errorf("expected 5")
	}

	if p, err := s.Get("SlicePtr/0"); err != nil {
		t.Error(err)
	} else if p == nil {
		t.Errorf("response is nil")
	} else if string(p) != "5" {
		t.Errorf("expected 5")
	}

	if p, err := s.Get("MapPtr/five"); err != nil {
		t.Error(err)
	} else if p == nil {
		t.Errorf("response is nil")
	} else if string(p) != "5" {
		t.Errorf("expected 5")
	}

	body := []byte("6")
	if p, err := s.Post("Ptr", body); err != nil {
		t.Error(err)
	} else if p != "Ptr" {
		t.Errorf("expected Ptr")
	} else if *tester.Ptr != 6 {
		t.Errorf("tester not set to 6")
	}

	if p, err := s.Post("SlicePtr/0", body); err != nil {
		t.Error(err)
	} else if p != "SlicePtr/0" {
		t.Errorf("expected SlicePtr/0")
	} else if len(tester.SlicePtr) != 1 || tester.SlicePtr[0] == nil || *tester.SlicePtr[0] != 6 {
		t.Errorf("tester.SlicePtr not correct after post")
	}

	if p, err := s.Post("MapPtr/five", body); err != nil {
		t.Error(err)
	} else if p != "MapPtr/five" {
		t.Errorf("expected MapPtr/five")
	} else if len(tester.MapPtr) != 1 || tester.MapPtr["five"] == nil || *tester.MapPtr["five"] != 6 {
		t.Errorf("unexpected MapPtr values")
	}

	body = []byte("7")
	if p, err := s.Put("SlicePtr", body); err != nil {
		t.Error(err)
	} else if p != "SlicePtr/1" {
		t.Errorf("unexpected put return value")
	} else if len(tester.SlicePtr) != 2 || tester.SlicePtr[1] == nil || *tester.SlicePtr[1] != 7 {
		t.Errorf("unexpected slice put value")
	}

	if p, err := s.Put("MapPtr/seven", body); err != nil {
		t.Error(err)
	} else if p != "MapPtr/seven" {
		t.Errorf("unexpected path on return of put; %s", p)
	} else if len(tester.MapPtr) != 2 || tester.MapPtr["seven"] == nil || *tester.MapPtr["seven"] != 7 {
		t.Errorf("unexpceted tester.MapPtr['seven'] value")
	}
}

func TestDelete(t *testing.T) {
	tester := &TestStruct{
		Slice: []int{1, 2, 3},
		Map:   map[string]int{"one": 1, "two": 2},
	}

	s := NewServer(tester)

	err := s.Delete("Slice/0")
	if err != nil {
		t.Error(err)
	} else if len(tester.Slice) != 2 {
		t.Errorf("slice should have len 2")
	} else if tester.Slice[0] != 2 || tester.Slice[1] != 3 {
		t.Errorf("slice dosn't have the right values %v", tester.Slice)
	}

	err = s.Delete("Map/one")
	if err != nil {
		t.Error(err)
	} else if len(tester.Map) != 1 {
		t.Errorf("map should have length 1")
	} else if _, ok := tester.Map["one"]; ok {
		t.Errorf("'one' should have been deleted")
	}

	err = s.Delete("Map/three")
	if err == nil {
		t.Errorf("shouldn't be able to delete 'three'")
	}
}

func TestPut(t *testing.T) {
	tester := &TestStruct{
		Slice: []int{1, 2, 3},
		Map:   map[string]int{"one": 1},
	}

	s := NewServer(tester)

	b := []byte("4")
	p, err := s.Put("Slice", b)

	if err != nil {
		t.Error(err)
	} else if len(tester.Slice) != 4 {
		t.Errorf("expected %d got %d", 4, len(tester.Slice))
	} else if tester.Slice[3] != 4 {
		t.Errorf("expected tester.Slice[3] == %d got %d", 4, tester.Slice[3])
	} else if p != "Slice/3" {
		t.Errorf("expected %s got %s", "Slice/3", p)
	}

	b = []byte("2")
	p, err = s.Put("Map/two", b)

	if err != nil {
		t.Error(err)
	} else if len(tester.Map) != 2 {
		t.Errorf("expected %d got %d", 2, len(tester.Map))
	} else if tester.Map["two"] != 2 {
		t.Errorf("tester.Map[\"two\"] == %d but expected %d", tester.Map["two"], 2)
	}

}

func TestPost(t *testing.T) {
	tester := &TestStruct{
		Integer: -6,
		String:  "blah",
		Modify:  "empty",
		Struct: InnerStruct{
			Bool: true,
		},
	}

	s := NewServer(tester)

	tester.Modify = "original"

	b := []byte("\"test\"")
	p, err := s.Post("Modify", b)

	if err != nil {
		t.Error(err)
	} else if p != "Modify" {
		t.Errorf("path doesn't match after post")
	}

	if tester.Modify != "test" {
		t.Errorf("incorrect value")
	}

}

func TestGet(t *testing.T) {
	tester := &TestStruct{
		Integer: -6,
		String:  "blah",
		Modify:  "empty",
		Struct: InnerStruct{
			Bool: true,
		},
	}
	s := NewServer(tester)

	b, err := s.Get("integer")

	if err != nil {
		t.Error(err)
	} else if string(b) != "-6" {
		t.Errorf("got '%s', expected '%s'", string(b), "-6")
	}

	b, err = s.Get("Struct/Bool")

	if err != nil {
		t.Error(err)
	} else if string(b) != "true" {
		t.Errorf("got '%s' expected '%s'", string(b), "true")
	}

	b, err = s.Get("Blah")

	if err == nil {
		t.Errorf("expected error, got none")
	} else if s, ok := err.(Statuser); !ok {
		t.Errorf("error should be a statuser")
	} else if s.Status() != http.StatusNotFound {
		t.Errorf("expected status %v got %v", http.StatusNotFound, s.Status())
	}
}
