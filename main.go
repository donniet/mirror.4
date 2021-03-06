package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/donniet/darksky"

	"github.com/donniet/mirror.4/state"
)

const (
	defaultLat  = 44.8881782
	defaultLong = -93.2280129
)

var (
	addr       = "localhost:8081"
	weatherKey = ""
	lat        = defaultLat
	long       = defaultLong
	statePath  = "state.json"
)

func init() {
	flag.StringVar(&addr, "addr", addr, "address to run webserver")
	flag.StringVar(&weatherKey, "weatherKey", weatherKey, "darksky api key")
	flag.Float64Var(&lat, "lat", lat, "lattitude")
	flag.Float64Var(&long, "long", long, "longitude")
	flag.StringVar(&statePath, "statePath", statePath, "path to save state")
}

func mustExecuteTemplate(fileName string, templateName string, dat interface{}) []byte {
	buf := &bytes.Buffer{}

	if tmpl, err := template.New("index.html").Delims("[[", "]]").ParseFiles(fileName); err != nil {
		log.Fatalf("could not parse template: %v", err)
	} else if err := tmpl.ExecuteTemplate(buf, templateName, dat); err != nil {
		log.Fatalf("error executing template: %v", err)
	}

	return buf.Bytes()
}

func updateWeather(state *State) *StateMessage {
	log.Printf("starting weather updator")

	service := darksky.NewService(weatherKey)
	res, err := service.Get(float32(lat), float32(long))
	if err != nil {
		log.Printf("error getting weather %v", err)
		return nil
	}

	// log.Printf("updating weather: %v", res)

	// should do locked...
	state.Forecast.Updated = time.Now()
	state.Forecast.DateTime = time.Time(res.Currently.Time)
	if res.Currently.TemperatureHigh != nil {
		state.Forecast.High = *res.Currently.TemperatureHigh
	}
	if res.Currently.TemperatureLow != nil {
		state.Forecast.Low = *res.Currently.TemperatureLow
	}
	state.Forecast.Icon = res.Currently.Icon
	state.Forecast.Summary = res.Currently.Summary
	state.Forecast.Darksky = &res

	if res.Daily != nil && len(res.Daily.Data) > 0 {
		// log.Printf("hourly")
		if res.Daily.Data[0].TemperatureHigh != nil {
			state.Forecast.High = *res.Daily.Data[0].TemperatureHigh
		}
		if res.Daily.Data[0].TemperatureLow != nil {
			state.Forecast.Low = *res.Daily.Data[0].TemperatureLow
		}
		state.Forecast.Icon = res.Daily.Data[0].Icon
	}

	b, err := json.Marshal(state.Forecast)
	if err != nil {
		// why would this error?
		panic(err)
	}

	return &StateMessage{
		Method: http.MethodPost,
		Path:   "forecast",
		Body:   (*json.RawMessage)(&b),
	}

}

func weatherUpdator(apiServer *state.Server, state *State, stopper <-chan struct{}, messages chan<- StateMessage) {
	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	if msg := updateWeather(state); msg != nil {
		messages <- *msg
	}

	for {
		select {
		case <-ticker.C:
			if msg := updateWeather(state); msg != nil {
				messages <- *msg
			}
		case <-stopper:
			return
		}
	}
}

type StateServer struct {
	messages chan<- StateMessage
	server   *state.Server
}

func (s *StateServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	var res []byte
	var path string

	if r.Body != nil {
		body, err = ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		res, err = s.server.Get(r.URL.Path)
	case http.MethodPost:
		path, err = s.server.Post(r.URL.Path, body)
	case http.MethodPut:
		path, err = s.server.Put(r.URL.Path, body)
	case http.MethodDelete:
		err = s.server.Delete(r.URL.Path)
	default:
		http.Error(w, "method not supported", http.StatusMethodNotAllowed)
		return
	}

	if err != nil {
		msg, _ := json.Marshal(map[string]string{"error": err.Error()})

		if s, ok := err.(state.Statuser); ok {
			http.Error(w, string(msg), s.Status())
		} else {
			http.Error(w, string(msg), http.StatusInternalServerError)
		}
		return
	}

	if r.Method != http.MethodGet {
		s.messages <- StateMessage{
			Body:   (*json.RawMessage)(&body),
			Method: r.Method,
			Path:   r.URL.Path,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if path != "" {
		w.Header().Set("Location", path)
	}
	if len(res) > 0 {
		w.Write(res)
	}
}

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.Parse()

	stopper := make(chan struct{})
	messages := make(chan StateMessage)
	interrupt := make(chan os.Signal)
	signal.Notify(interrupt, os.Interrupt)

	local := new(State)
	apiServer := state.NewServer(local)
	stateServer := &StateServer{
		messages: messages,
		server:   apiServer,
	}

	if err := local.Load(statePath); err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}

	go weatherUpdator(apiServer, local, stopper, messages)

	sockets := NewSockets(stateServer, stopper)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", stateServer))
	mux.Handle("/websocket", sockets)
	mux.Handle("/client/", http.StripPrefix("/client/", http.FileServer(http.Dir("client"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexBytes := mustExecuteTemplate("client/index.html", "index.html", map[string]interface{}{
			"WebsocketURL": template.URL(fmt.Sprintf("ws://%s/websocket", addr)),
		})

		w.Write(indexBytes)
	})

	s := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		for {
			select {
			case msg := <-messages:
				log.Printf("got message: %#v", msg)
				if err := local.Save(statePath); err != nil {
					log.Fatal(err)
				}
				sockets.Write(msg)
			case <-stopper:
				return
			}
		}
	}()

	// graceful shutdown on interrupt
	go func() {
		<-interrupt

		log.Println("shutting down")
		close(stopper)
		close(messages)
		s.Close()
	}()

	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
