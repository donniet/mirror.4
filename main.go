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

var (
	addr       = "localhost:8081"
	weatherURL = "http://api.wunderground.com/api/52a3d65a04655627/forecast/q/MN/Minneapolis.json"
	statePath  = "state.json"
)

func init() {
	flag.StringVar(&addr, "addr", addr, "address to run webserver")
	flag.StringVar(&weatherURL, "weatherURL", weatherURL, "weather underground url")
	flag.StringVar(&statePath, "statePath", statePath, "path to save state")
}

func main() {
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	flag.Parse()

	interrupt := make(chan os.Signal)
	stopper := make(chan struct{})

	signal.Notify(interrupt, os.Interrupt)

	state := NewState(weatherURL, 1*time.Hour, stopper)

	if err := state.Load(statePath); err != nil {
		switch err.(type) {
		case *os.PathError:
			// do nothing, path doesn't exist
		default:
			log.Fatal(err)
		}
	}

	indexBytes := &bytes.Buffer{}

	if tmpl, err := template.ParseFiles("client/index.html"); err != nil {
		log.Fatalf("could not parse template: %v", err)
	} else if err := tmpl.ExecuteTemplate(indexBytes, "index.html", map[string]interface{}{
		"WebsocketURL": template.URL(fmt.Sprintf("ws://%s/websocket", addr)),
	}); err != nil {
		log.Fatalf("error executing template: %v", err)
	}

	sockets := NewSockets(state, stopper)

	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", state))
	mux.Handle("/websocket", sockets)
	mux.Handle("/client/", http.StripPrefix("/client/", http.FileServer(http.Dir("client"))))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(indexBytes.Bytes())
	})

	s := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	state.OnChanged = func() {
		if err := state.Save(statePath); err != nil {
			log.Fatal(err)
		}
		sockets.Write(state)
	}
	// try saving the state, better to fail now than later
	state.OnChanged()

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
