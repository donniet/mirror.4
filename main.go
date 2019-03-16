package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
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

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.Parse()

	interrupt := make(chan os.Signal)
	stopper := make(chan struct{})

	signal.Notify(interrupt, os.Interrupt)

	state := NewState(weatherKey, 1*time.Hour, float32(lat), float32(long), stopper)

	if err := state.Load(statePath); err != nil && err != os.ErrNotExist {
		log.Fatal(err)
	}

	sockets := NewSockets(state, stopper)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", state))
	mux.Handle("/websocket", sockets)
	mux.Handle("/client/", http.StripPrefix("/client/", http.FileServer(http.Dir("client"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexBytes := mustExecuteTemplate("client/index.html", "index.html", map[string]interface{}{
			"WebsocketURL": template.URL(fmt.Sprintf("ws://%s/websocket", addr)),
		})

		w.Write(indexBytes)
	})

	state.OnChanged = func() {
		if err := state.Save(statePath); err != nil {
			log.Fatal(err)
		}
		sockets.Write(state)
	}
	// try saving the state, better to fail now than later
	state.OnChanged()

	s := http.Server{
		Addr:    addr,
		Handler: mux,
	}

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
