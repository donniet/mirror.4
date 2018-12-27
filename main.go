package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"
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

type Forecast struct {
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Icon      string    `json:"icon"`
	DateTime  time.Time `json:"dateTime"`
	Visible   bool      `json:"visible"`
	updateUrl string
	timeout   time.Duration
	locker    sync.Locker
}

func (f *Forecast) Update() error {
	svc := WeatherService{
		URL:     f.updateUrl,
		Timeout: f.timeout,
	}

	if w, err := svc.GetWeather(); err != nil {
		return err
	} else {
		f.locker.Lock()
		defer f.locker.Unlock()

		f.High = w.High
		f.Low = w.Low
		f.Icon = w.Icon
		f.DateTime = w.DateTime
	}
	return nil
}

func (f *Forecast) MarshalJSON() ([]byte, error) {
	f.locker.Lock()
	defer f.locker.Unlock()

	return json.Marshal(map[string]interface{}{
		"high":     f.High,
		"low":      f.Low,
		"icon":     f.Icon,
		"dateTime": f.DateTime,
		"visible":  f.Visible,
	})
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

func (f *Forecast) Get(path string) (*json.RawMessage, error) {
	var dat interface{}

	f.locker.Lock()
	switch path {
	case "":
		dat = f
	case "high":
		dat = f.High
	case "low":
		dat = f.Low
	case "icon":
		dat = f.Icon
	case "dateTime":
		dat = f.DateTime
	case "visible":
		dat = f.Visible
	default:
		f.locker.Unlock()
		return nil, &NotFoundError{message: fmt.Sprintf("path not found '%s'", path)}
	}
	f.locker.Unlock()

	if b, err := json.Marshal(dat); err != nil {
		return nil, err
	} else {
		return (*json.RawMessage)(&b), nil
	}
}

func (f *Forecast) Post(path string, body *json.RawMessage) (string, error) {
	if body == nil {
		return "", &InvalidMethodError{message: "body is null"}
	}

	switch path {
	case "visible":
		f.locker.Lock()
		defer f.locker.Unlock()

		if err := json.Unmarshal(*body, &(f.Visible)); err != nil {
			return "", err
		}
	default:
		return "", &InvalidMethodError{message: "can only post to 'visible' property"}
	}

	return path, nil
}

type Display struct {
	PowerStatus string `json:"powerStatus"`
}

func (d *Display) Get(path string) (*json.RawMessage, error) {
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

type Faces struct {
	Detections    []FaceDetection `json:"detections"`
	MaxDetections int             `json:"maxDetections"`
	lock          sync.Locker
}

func (f *Faces) MarshalJSON() ([]byte, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	return json.Marshal(map[string]interface{}{
		"detections":    f.Detections,
		"maxDetections": f.MaxDetections,
	})
}

func (f *Faces) Get(path string) (*json.RawMessage, error) {
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

type Motion struct {
	Detections    []MotionDetection `json:"detections"`
	MaxDetections int               `json:"maxDetections"`
	lock          sync.Locker
}

func (m *Motion) Detected(magnitude float32) {
	detection := MotionDetection{
		DateTime:  time.Now(),
		Magnitude: magnitude,
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	m.Detections = append([]MotionDetection{detection}, m.Detections...)

	if len(m.Detections) >= m.MaxDetections {
		m.Detections = m.Detections[0:m.MaxDetections]
	}
}

func (m *Motion) MarshalJSON() ([]byte, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return json.Marshal(map[string]interface{}{
		"detections":    m.Detections,
		"maxDetections": m.MaxDetections,
	})
}

func (m *Motion) Get(path string) (*json.RawMessage, error) {
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

func (m *Motion) Post(path string, body *json.RawMessage) (string, error) {
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

type MotionDetection struct {
	DateTime  time.Time `json:"dateTime"`
	Magnitude float32   `json:"magnitude"`
}

func (m *MotionDetection) Get(path string) (*json.RawMessage, error) {
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

type Streams struct {
	streams []*Stream
	lock    sync.Locker
}

func (s *Streams) Get(path string) (*json.RawMessage, error) {
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

func (s *Streams) Delete(path string) error {
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

func (s *Streams) Put(path string, body *json.RawMessage) (string, error) {
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

func (s *Streams) Post(path string, body *json.RawMessage) (string, error) {
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

func (s *Streams) MarshalJSON() ([]byte, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return json.Marshal(s.streams)
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
	Forecast *Forecast `json:"forecast"`
	Display  *Display  `json:"display"`
	Motion   *Motion   `json:"motion"`
	Streams  *Streams  `json:"streams"`
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
	default:
		err = &InvalidMethodError{message: "PUT not supported"}
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

	return err
}

func (s *State) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[1:] // strip leading slash
	var body *json.RawMessage

	if b, err := ioutil.ReadAll(r.Body); err != nil {
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

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	state := &State{
		Forecast: &Forecast{
			updateUrl: "http://api.wunderground.com/api/52a3d65a04655627/forecast/q/MN/Minneapolis.json",
			locker:    &sync.Mutex{},
			timeout:   1 * time.Minute,
		},
		Display: &Display{
			PowerStatus: "on",
		},
		Motion: &Motion{
			Detections: []MotionDetection{
				MotionDetection{
					DateTime:  time.Now(),
					Magnitude: 3,
				},
				MotionDetection{
					DateTime:  time.Now().Add(-1 * time.Minute),
					Magnitude: 4,
				},
			},
			MaxDetections: 4,
			lock:          &sync.Mutex{},
		},
		Streams: &Streams{
			streams: []*Stream{
				&Stream{
					URL:     "http://www.google.com/",
					Name:    "google",
					Visible: true,
				},
			},
			lock: &sync.Mutex{},
		},
	}

	s := http.Server{
		Addr:    ":8081",
		Handler: state,
	}

	interrupt := make(chan os.Signal)
	stopper := make(chan struct{})

	signal.Notify(interrupt, os.Interrupt)

	go func() {
		ticker := time.NewTicker(1 * time.Hour)

		if err := state.Forecast.Update(); err != nil {
			log.Print(err)
		}

		for {
			select {
			case <-ticker.C:
				if err := state.Forecast.Update(); err != nil {
					log.Print(err)
				}
			case <-stopper:
				log.Printf("shutting down worker thread")
				return
			}
		}
	}()

	// graceful shutdown on interrupt
	go func() {
		<-interrupt

		log.Println("shutting down")
		close(stopper)
		s.Close()
	}()

	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
