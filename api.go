package main

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/donniet/darksky"
)

type forecast struct {
	High     float64   `json:"high"`
	Low      float64   `json:"low"`
	Current  float64   `json:"current"`
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
	Detections    []FaceDetection `json:"detections",api:"maximum=50"`
	People        People          `json:"people"`
	MaxDetections int             `json:"maxDetections"`
}

type FaceDetection struct {
	DateTime   time.Time `json:"dateTime"`
	Confidence float32   `json:"confidence"`
	Name       string    `json:"name"`
	Image      string    `json:"image"`
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
	URL     string `json:"url"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
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
