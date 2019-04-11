package state

import (
	"encoding/json"
	"net/http"
	"testing"
)

type TestStruct struct {
	Integer int `json:"integer,omitempty"`
	String  string
	Modify  string
	Struct  InnerStruct
}

type InnerStruct struct {
	Bool bool
}

var tester *TestStruct

func init() {
	tester = &TestStruct{
		Integer: -6,
		String:  "blah",
		Modify:  "empty",
		Struct: InnerStruct{
			Bool: true,
		},
	}
}

func TestPost(t *testing.T) {
	s := NewServer(tester)

	tester.Modify = "original"

	b := []byte("\"test\"")
	p, err := s.Post("Modify", (*json.RawMessage)(&b))

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
	s := NewServer(tester)

	b, err := s.Get("integer")

	if err != nil {
		t.Error(err)
	} else if string(*b) != "-6" {
		t.Errorf("got '%s', expected '%s'", string(*b), "-6")
	}

	b, err = s.Get("Struct/Bool")

	if err != nil {
		t.Error(err)
	} else if string(*b) != "true" {
		t.Errorf("got '%s' expected '%s'", string(*b), "true")
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
