package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/donniet/darksky"
)

type forecast struct {
	High     float32   `json:"high"`
	Low      float32   `json:"low"`
	Current  float32   `json:"current"`
	Summary  string    `json:"summary"`
	Icon     string    `json:"icon"`
	DateTime time.Time `json:"dateTime"`
	Visible  bool      `json:"visible"`
	Updated  time.Time `json:"updated"`

	Darksky *darksky.Response `json:"darksky,omitempty"`
}

type display struct {
	PowerStatus string `json:"powerStatus"`
}
type faces struct {
	Detections    []FaceDetection `json:"detections" api:"maximum=50"`
	People        People          `json:"people"`
	MaxDetections int             `json:"maxDetections"`
}

type FaceDetection struct {
	DateTime   time.Time `json:"dateTime"`
	Confidence float32   `json:"confidence"`
	Name       string    `json:"name"`
	Image      DataURI   `json:"image"`
}

type DataURI struct {
	contentType string
	data        []byte
}

func (d *DataURI) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		d.contentType = ""
		d.data = []byte{}
		return nil
	}

	str := ""
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}

	semicolon := strings.IndexRune(str, ';')

	if semicolon < 0 {
		return fmt.Errorf("semicolon missing from data-uri")
	}

	// look for "base64," after semicolon
	if len(str) < semicolon+8 || str[semicolon+1:semicolon+8] != "base64," {
		return fmt.Errorf("only base64 encoded data-uri is supported")
	}

	if dat, err := base64.StdEncoding.DecodeString(str[semicolon+8:]); err != nil {
		return err
	} else {
		d.data = dat
	}
	d.contentType = str[:semicolon]

	return nil
}

func (d DataURI) MarshalJSON() ([]byte, error) {
	str := d.contentType + ";base64," + base64.StdEncoding.EncodeToString(d.data)

	return json.Marshal(str)
}

type People map[string]Person
type Person struct {
	Distance  float32   `json:"distance"`
	Embedding Embedding `json:"embedding"`
}

type Embedding []float32

type motion struct {
	Detections    []motionDetection `json:"detections"`
	MaxDetections int               `json:"maxDetections"`
}

type motionDetection struct {
	DateTime  time.Time `json:"dateTime"`
	Magnitude float32   `json:"magnitude"`
}

type streams []Stream

type Stream struct {
	URL       string    `json:"url"`
	Name      string    `json:"name"`
	Visible   bool      `json:"visible"`
	ErrorTime time.Time `json:"errorTime"`
}

type State struct {
	Forecast forecast `json:"forecast"`
	Display  display  `json:"display"`
	Motion   motion   `json:"motion"`
	Faces    faces    `json:"faces"`
	Streams  streams  `json:"streams"`
}

func (s *State) Save(statePath string) error {
	if b, err := json.MarshalIndent(s, "", "  "); err != nil {
		return err
	} else if err := ioutil.WriteFile(statePath, b, 0660); err != nil {
		return err
	}
	return nil
}

func (s *State) Load(statePath string) error {
	if b, err := ioutil.ReadFile(statePath); err != nil {
		return err
	} else if err := json.Unmarshal(b, s); err != nil {
		return err
	}
	return nil
}
