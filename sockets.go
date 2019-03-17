package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type StateMessage struct {
	Method string           `json:"method"`
	Path   string           `json:"path"`
	Body   *json.RawMessage `json:"body"`
}

type SocketConn struct {
	conn     *websocket.Conn
	messages chan *json.RawMessage
}

func (w SocketConn) Header() http.Header {
	return http.Header(make(map[string][]string))
}
func (w SocketConn) WriteHeader(statusCode int) {}
func (w SocketConn) Write(b []byte) (int, error) {
	w.messages <- (*json.RawMessage)(&b)
	return len(b), nil
}

func (c SocketConn) writer() {
	for {
		if msg, ok := <-c.messages; !ok {
			break
		} else if msg == nil {
			// this shouldn't ever happen
			log.Fatal("nil message passed to websocket")
		} else if err := c.conn.WriteMessage(websocket.TextMessage, *msg); err != nil {
			log.Printf("error writing to socket: %v", err)
			break
		}
	}

	log.Printf("writer ending")
}

type ErrorMessage struct {
	Error string `json:"error"`
}

func MessageFromError(err error) ErrorMessage {
	return ErrorMessage{Error: err.Error()}
}

type Sockets struct {
	locker      sync.Locker
	upgrader    websocket.Upgrader
	state       *State
	stopper     <-chan struct{}
	connections map[*websocket.Conn]SocketConn
}

func NewSockets(state *State, stopper <-chan struct{}) *Sockets {
	ret := &Sockets{
		locker:  &sync.Mutex{},
		state:   state,
		stopper: stopper,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		connections: make(map[*websocket.Conn]SocketConn),
	}
	return ret
}

func (socks *Sockets) Write(obj interface{}) error {
	var b []byte
	var err error

	if b, err = json.Marshal(obj); err != nil {
		return err
	}

	if len(b) == 0 {
		return nil
	}

	socks.locker.Lock()
	defer socks.locker.Unlock()

	for _, c := range socks.connections {
		c.messages <- (*json.RawMessage)(&b)
	}
	return nil
}

func (socks *Sockets) reader(c SocketConn, stopper chan struct{}) {
	defer func() {
		close(stopper)
	}()

	msg := StateMessage{}

	for {
		if _, b, err := c.conn.ReadMessage(); err != nil {
			log.Printf("error from websocket: %v", err)
			break
		} else if err := json.Unmarshal(b, &msg); err != nil {
			log.Printf("error unmarshalling message: %v", err)
			buf, _ := json.Marshal(MessageFromError(err))
			c.messages <- (*json.RawMessage)(&buf)
			continue
		}

		var reader io.Reader
		if msg.Body != nil {
			reader = bytes.NewReader(*msg.Body)
		}
		if r, err := http.NewRequest(msg.Method, msg.Path, reader); err != nil {
			log.Fatalf("error constructing request: %v", err)
		} else {
			socks.state.ServeHTTP(c, r)
		}
	}

	log.Printf("closing reader")
}
func (socks *Sockets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var conn *websocket.Conn
	var err error

	if conn, err = socks.upgrader.Upgrade(w, r, nil); err != nil {
		log.Printf("error upgrading connection: %v", err)
		return
	}

	stopper := make(chan struct{})

	c := SocketConn{
		conn:     conn,
		messages: make(chan *json.RawMessage),
	}

	socks.locker.Lock()
	socks.connections[conn] = c
	socks.locker.Unlock()

	go c.writer()
	go socks.reader(c, stopper)
	go func() {
		<-stopper
		close(c.messages)

		socks.locker.Lock()
		defer socks.locker.Unlock()

		delete(socks.connections, conn)
	}()
}

func (socks *Sockets) Close() {
	socks.locker.Lock()
	defer socks.locker.Unlock()

	for _, sc := range socks.connections {
		sc.conn.Close()
	}
}
