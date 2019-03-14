package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/donniet/darksky"
)

type Statuser interface {
	Status() int
}
type Getter interface {
	Get(path string) (*json.RawMessage, error)
}
type Deleter interface {
	Delete(path string) error
}
type Putter interface {
	Put(path string, body *json.RawMessage) (string, error)
}
type Poster interface {
	Post(path string, body *json.RawMessage) (string, error)
}

type forecast struct {
	High     float32   `json:"high"`
	Low      float32   `json:"low"`
	Current  float32   `json:"current"`
	Summary  string    `json:"summary"`
	Icon     string    `json:"icon"`
	DateTime time.Time `json:"dateTime"`
	Visible  bool      `json:"visible"`
	key      string
	lat      float32
	long     float32
	timeout  time.Duration
	lock     sync.Locker
	state    *State
}

func (f *forecast) Update() error {
	s := darksky.NewService(f.key)

	if w, err := s.Get(f.lat, f.long); err != nil {
		return err
	} else {
		f.lock.Lock()
		defer f.lock.Unlock()

		f.High = w.Daily.Data[0].TemperatureHigh
		f.Low = w.Daily.Data[0].TemperatureLow
		// f.Icon = w.Daily.Data[0].Icon
		f.Summary = w.Daily.Data[0].Summary

		now := time.Now()
		for _, d := range w.Hourly.Data {
			if time.Time(d.Time).Sub(now) < 0 {
				continue
			}

			f.Current = d.Temperature
			f.DateTime = time.Time(d.Time)
			f.Icon = d.Icon
		}

		if f.state.OnChanged != nil {
			go f.state.OnChanged()
		}
	}
	return nil
}

func (f *forecast) MarshalJSON() ([]byte, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return json.Marshal(map[string]interface{}{
		"high":     f.High,
		"low":      f.Low,
		"icon":     f.Icon,
		"current":  f.Current,
		"summary":  f.Summary,
		"dateTime": f.DateTime,
		"visible":  f.Visible,
	})
}

func (f *forecast) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	f.lock.Lock()
	switch path {
	case "":
		dat = f
	case "high":
		dat = f.High
	case "low":
		dat = f.Low
	case "current":
		dat = f.Current
	case "summary":
		dat = f.Summary
	case "icon":
		dat = f.Icon
	case "dateTime":
		dat = f.DateTime
	case "visible":
		dat = f.Visible
	default:
		f.lock.Unlock()
		return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}
	f.lock.Unlock()

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

func (f *forecast) Post(path string, body *json.RawMessage) (string, error) {
	if body == nil {
		return "", &InvalidMethodError{message: "body is null"}
	}

	switch path {
	case "visible":
		f.lock.Lock()
		defer f.lock.Unlock()

		if err := json.Unmarshal(*body, &(f.Visible)); err != nil {
			return "", err
		}
	default:
		return "", &InvalidMethodError{message: "can only post to 'visible' property"}
	}

	return path, nil
}

type display struct {
	PowerStatus string `json:"powerStatus"`
}

func (d *display) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	switch path {
	case "":
		dat = d
	case "powerStatus":
		dat = d.PowerStatus
	default:
		return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

type faces struct {
	Detections    []FaceDetection `json:"detections"`
	MaxDetections int             `json:"maxDetections"`
	lock          sync.Locker
}

func (s *faces) Put(path string, body *json.RawMessage) (string, error) {
	d := FaceDetection{}

	if path != "detections" {
		return "", NewNotFoundError(path)
	} else if body == nil {
		return "", &BadRequestError{message: "body is nil"}
	} else if err := json.Unmarshal(*body, &d); err != nil {
		return "", err
	}

	if d.DateTime == (time.Time{}) {
		d.DateTime = time.Now()
	}
	if d.Confidence == 0. {
		d.Confidence = 1.0
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if len(s.Detections) >= s.MaxDetections {
		s.Detections = s.Detections[0 : s.MaxDetections-1]
	}
	s.Detections = append([]FaceDetection{d}, s.Detections...)

	return "detections/0", nil
}

func (f *faces) MarshalJSON() ([]byte, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return json.Marshal(map[string]interface{}{
		"detections":    f.Detections,
		"maxDetections": f.MaxDetections,
	})
}

func (f *faces) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	first, rest := chompPath(path)
	next := ""

	var err error

	f.lock.Lock()
	switch first {
	case "":
		dat = f
	case "detections":
		next, rest = chompPath(rest)
		if dex, err := strconv.Atoi(next); err != nil || dex < 0 || dex >= len(f.Detections) {
			err = NewNotFoundError(path)
		} else if rest == "" {
			dat = f.Detections[dex]
		} else {
			det := f.Detections[dex]
			f.lock.Unlock()
			return det.Get(rest)
		}
	case "maxDetections":
		dat = f.MaxDetections
	default:
		err = NewNotFoundError(path)
	}
	f.lock.Unlock()

	if err != nil {
		return nil, err
	} else if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

type FaceDetection struct {
	DateTime   time.Time `json:"dateTime"`
	Confidence float32   `json:"confidence"`
	Name       string    `json:"name"`
	Image      string    `json:"image"`
}

func (d FaceDetection) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	first, rest := chompPath(path)

	if rest != "" {
		return nil, NewNotFoundError(path)
	}

	switch first {
	case "":
		dat = d
	case "dateTime":
		dat = d.DateTime
	case "confidence":
		dat = d.Confidence
	case "name":
		dat = d.Name
	default:
		return nil, NewNotFoundError(path)
	}

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

type motion struct {
	Detections    []motionDetection `json:"detections"`
	MaxDetections int               `json:"maxDetections"`
	lock          sync.Locker
}

func (m *motion) Detected(magnitude float32) {
	detection := motionDetection{
		DateTime:  time.Now(),
		Magnitude: magnitude,
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	m.Detections = append([]motionDetection{detection}, m.Detections...)

	if len(m.Detections) >= m.MaxDetections {
		m.Detections = m.Detections[0:m.MaxDetections]
	}
}

func (m *motion) MarshalJSON() ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return json.Marshal(map[string]interface{}{
		"detections":    m.Detections,
		"maxDetections": m.MaxDetections,
	})
}

func (m *motion) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	first, rest := chompPath(path)

	m.lock.Lock()
	switch first {
	case "":
		if rest == "" {
			dat = m
		} else {
			m.lock.Unlock()
			return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
		}
	case "detections":
		if rest == "" {
			dat = m.Detections
		} else {
			next, rest := chompPath(rest)
			if dex, err := strconv.Atoi(next); err != nil || dex < 0 || dex >= len(m.Detections) {
				m.lock.Unlock()
				return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
			} else if rest == "" {
				dat = m.Detections[dex]
			} else {
				m.lock.Unlock()
				return m.Detections[dex].Get(rest)
			}
		}
	case "maxDetections":
		dat = m.MaxDetections
	default:
		m.lock.Unlock()
		return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}
	m.lock.Unlock()

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

func (m *motion) Put(path string, body *json.RawMessage) (string, error) {
	switch path {
	case "detections":
	default:
		return "", NewNotFoundError(path)
	}

	if body == nil {
		return "", &BadRequestError{message: "body is nil"}
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	d := motionDetection{}
	if err := json.Unmarshal(*body, &d); err != nil {
		return "", &BadRequestError{message: err.Error()}
	}

	if d.DateTime == (time.Time{}) {
		d.DateTime = time.Now()
	}

	m.Detections = append([]motionDetection{d}, m.Detections...)

	if len(m.Detections) >= m.MaxDetections {
		m.Detections = m.Detections[0:m.MaxDetections]
	}

	return "detections/0", nil
}

func (m *motion) Post(path string, body *json.RawMessage) (string, error) {
	switch path {
	case "maxDetections":
	default:
		return "", NewNotFoundError(path)
	}

	if body == nil {
		return "", &BadRequestError{message: "body is nil"}
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if err := json.Unmarshal(*body, &m.MaxDetections); err != nil {
		return "", &BadRequestError{message: err.Error()}
	}
	return path, nil
}

type motionDetection struct {
	DateTime  time.Time `json:"dateTime"`
	Magnitude float32   `json:"magnitude"`
}

func (m *motionDetection) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	switch path {
	case "":
		dat = m
	case "dateTime":
		dat = m.DateTime
	case "magnitude":
		dat = m.Magnitude
	default:
		return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

type streams struct {
	streams []*Stream
	lock    sync.Locker
}

func (s *streams) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	if path == "" {
		dat = s
	} else {
		first, rest := chompPath(path)

		s.lock.Lock()
		if dex, err := strconv.Atoi(first); err != nil || dex < 0 || dex > len(s.streams) {
			s.lock.Unlock()
			return nil, NewNotFoundError(path)
		} else if rest != "" {
			stream := s.streams[dex]
			s.lock.Unlock()
			return stream.Get(rest)
		} else {
			dat = s.streams[dex]
		}

		s.lock.Unlock()
	}

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

func (s *streams) Delete(path string) error {
	if path == "" {
		return &InvalidMethodError{message: "DELETE not allowed"}
	}

	first, rest := chompPath(path)

	if rest != "" {
		return &InvalidMethodError{message: "DELETE not allowed"}
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if dex, err := strconv.Atoi(first); err != nil || dex < 0 || dex > len(s.streams) {
		return NewNotFoundError(path)
	} else {
		s.streams = append(s.streams[:dex], s.streams[dex+1:]...)
	}
	return nil
}

func (s *streams) Put(path string, body *json.RawMessage) (string, error) {
	if path != "" {
		return "", NewNotFoundError(path)
	} else if body == nil {
		return "", &BadRequestError{message: "body is nil"}
	}

	str := Stream{}

	if err := json.Unmarshal(*body, &str); err != nil {
		return "", err
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.streams = append(s.streams, &str)

	return fmt.Sprintf("%d", len(s.streams)-1), nil
}

func (s *streams) Post(path string, body *json.RawMessage) (string, error) {
	if path == "" {
		return "", &InvalidMethodError{"POST not allowed"}
	} else if body == nil {
		return "", &BadRequestError{message: "body is nil"}
	}

	first, rest := chompPath(path)

	s.lock.Lock()
	defer s.lock.Unlock()

	if dex, err := strconv.Atoi(first); err != nil || dex < 0 || dex >= len(s.streams) {
		return "", NewNotFoundError(path)
	} else if rest != "" {
		if loc, err := s.streams[dex].Post(rest, body); err != nil {
			return "", err
		} else {
			return appendPath(path, loc), nil
		}
	} else if err := json.Unmarshal(*body, &s.streams[dex]); err != nil {
		return "", &BadRequestError{message: err.Error()}
	}

	return path, nil
}

func (s *streams) MarshalJSON() ([]byte, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return json.Marshal(s.streams)
}

func (s *streams) UnmarshalJSON(b []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	return json.Unmarshal(b, &s.streams)
}

type Stream struct {
	URL     string `json:"url"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
}

func (s *Stream) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	switch dat {
	case "":
		dat = s
	case "url":
		dat = s.URL
	case "name":
		dat = s.Name
	case "visible":
		dat = s.Visible
	default:
		return nil, NewNotFoundError(path)
	}

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

func (s *Stream) Post(path string, body *json.RawMessage) (string, error) {
	var dat interface{}

	switch path {
	case "":
		dat = s
	case "url":
		dat = &s.URL
	case "name":
		dat = &s.Name
	case "visible":
		dat = &s.Visible
	default:
		return "", NewNotFoundError(path)
	}

	if err := json.Unmarshal(*body, dat); err != nil {
		return "", &BadRequestError{message: err.Error()}
	} else {
		return path, nil
	}
}

type State struct {
	Forecast  *forecast `json:"forecast"`
	Display   *display  `json:"display"`
	Motion    *motion   `json:"motion"`
	Faces     *faces    `json:"faces"`
	Streams   *streams  `json:"streams"`
	OnChanged func()    `json:"-"`
}

func NewState(weatherKey string, weatherUpdateInterval time.Duration, lat float32, long float32, stopper <-chan struct{}) *State {
	s := &State{
		Forecast: &forecast{
			key:     weatherKey,
			lat:     lat,
			long:    long,
			lock:    &sync.Mutex{},
			timeout: 1 * time.Minute,
		},
		Display: &display{
			PowerStatus: "on",
		},
		Motion: &motion{
			MaxDetections: 4,
			lock:          &sync.Mutex{},
		},
		Faces: &faces{
			MaxDetections: 10,
			lock:          &sync.Mutex{},
		},
		Streams: &streams{
			lock: &sync.Mutex{},
		},
	}
	s.Forecast.state = s

	go s.background(weatherUpdateInterval, stopper)

	return s
}

func logPrintIf(err error) {
	if err != nil {
		log.Print(err)
	}
}

func (s *State) background(weatherUpdateInterval time.Duration, stopper <-chan struct{}) {
	// background thread
	ticker := time.NewTicker(weatherUpdateInterval)

	logPrintIf(s.Forecast.Update())

	for {
		select {
		case <-ticker.C:
			logPrintIf(s.Forecast.Update())
		case <-stopper:
			log.Printf("shutting down worker thread")
			return
		}
	}
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

func chompPath(path string) (first string, rest string) {
	if dex := strings.Index(path, "/"); dex < 0 {
		return path, ""
	} else {
		return path[:dex], path[dex+1:]
	}
}
func appendPath(path string, next string) string {
	if path == "" {
		return next
	}

	if path[len(path)-1:len(path)] == "/" {
		return path + next
	} else {
		return path + "/" + next
	}
}

func (s *State) Get(path string) (*json.RawMessage, error) {
	if path == "" {
		if b, err := json.Marshal(s); err != nil {
			return nil, err
		} else {
			return (*json.RawMessage)(&b), nil
		}
	}

	first, rest := chompPath(path)

	switch first {
	case "forecast":
		return s.Forecast.Get(rest)
	case "display":
		return s.Display.Get(rest)
	case "motion":
		return s.Motion.Get(rest)
	case "streams":
		return s.Streams.Get(rest)
	case "faces":
		return s.Faces.Get(rest)
	default:
		return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}
}

func (s *State) Post(path string, body *json.RawMessage) (string, error) {
	if path == "" {
		return "", &InvalidMethodError{message: "POST not supported"}
	}

	first, rest := chompPath(path)

	p := ""
	var err error

	switch first {
	case "forecast":
		p, err = s.Forecast.Post(rest, body)
	case "motion":
		p, err = s.Motion.Post(rest, body)
	case "streams":
		p, err = s.Streams.Post(rest, body)
	case "display":
		err = &InvalidMethodError{message: "POST not supported"}
	default:
		err = &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}

	if err == nil && s.OnChanged != nil {
		go s.OnChanged()
	}

	return appendPath(first, p), err
}

func (s *State) Put(path string, body *json.RawMessage) (string, error) {
	if path == "" {
		return "", &InvalidMethodError{message: "PUT not supported"}
	}

	first, rest := chompPath(path)

	var err error

	switch first {
	case "streams":
		rest, err = s.Streams.Put(rest, body)
	case "motion":
		rest, err = s.Motion.Put(rest, body)
	case "faces":
		rest, err = s.Faces.Put(rest, body)
	default:
		err = &InvalidMethodError{message: "PUT not supported"}
	}

	if err == nil && s.OnChanged != nil {
		go s.OnChanged()
	}

	return appendPath(first, rest), err
}

func (s *State) Delete(path string) error {
	first, rest := chompPath(path)

	var err error

	switch first {
	case "streams":
		err = s.Streams.Delete(rest)
	default:
		err = &InvalidMethodError{message: "DELETE not allowed"}
	}

	if err == nil && s.OnChanged != nil {
		go s.OnChanged()
	}

	return err
}

func (s *State) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// log.Printf("Serving HTTP: %#v", r)

	path := r.URL.Path[1:] // strip leading slash

	var body *json.RawMessage

	if r.Body == nil {
		// do nothing
	} else if b, err := ioutil.ReadAll(r.Body); err != nil {
		log.Printf("error reading body: %v", err)
	} else {
		body = (*json.RawMessage)(&b)
	}

	var res *json.RawMessage
	var loc string
	var err error

	switch r.Method {
	case http.MethodGet:
		res, err = s.Get(path)
	case http.MethodPost:
		loc, err = s.Post(path, body)
	case http.MethodPut:
		loc, err = s.Put(path, body)
	case http.MethodDelete:
		err = s.Delete(path)
	default:
		err = &InvalidMethodError{message: "method not supported"}
	}

	if err != nil {
		status := http.StatusInternalServerError

		if s, ok := err.(Statuser); ok {
			status = s.Status()
		}

		http.Error(w, err.Error(), status)
		return
	}

	if res != nil {
		w.Header().Add("Content-Type", "application/json")
		if n, err := w.Write(*res); err != nil {
			log.Printf("error writing to response: %v", err)
		} else if n < len(*res) {
			log.Printf("could not write all bytes to response, wrote: %d expected %d", n, len(*res))
		}
	} else if loc != "" {
		w.Header().Add("Location", loc)
	}
}

type NotFoundError struct {
	message string
}

func (e *NotFoundError) Error() string {
	return e.message
}
func (e *NotFoundError) Status() int {
	return http.StatusNotFound
}
func NewNotFoundError(path string) *NotFoundError {
	return &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
}

type InvalidMethodError struct {
	message string
}

func (e *InvalidMethodError) Error() string {
	return e.message
}
func (e *InvalidMethodError) Status() int {
	return http.StatusMethodNotAllowed
}

type BadRequestError struct {
	message string
}

func (e *BadRequestError) Error() string {
	return e.message
}
func (e *BadRequestError) Status() int {
	return http.StatusBadRequest
}
