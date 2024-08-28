package pamsocket

import (
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func TestMain(m *testing.M) {
	log.Logger = log.With().Caller().Logger()
	m.Run()
}

type socket struct {
	listener net.Listener
	port int
}

func makeSocket() socket {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}
	return socket{
		listener: listener,
		port: listener.Addr().(*net.TCPAddr).Port,
	}
}

type server struct {
	s socket
	ws *PamSocket
}

func makeServer() *server {
	s := &server{
		s: makeSocket(),
		ws: &PamSocket{
			Service: "google-authenticator",
			ConfDir: "/home/achernya/src/idp-example/pam.d/",
			// Service: "passwd",
			// ConfDir: "/etc/pam.d",
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/ws", s.ws)
	go http.Serve(s.s.listener, mux)
	return s
	
}

func connect(s *server) (*websocket.Conn, *http.Response, error) {
	d := &websocket.Dialer{}
	return d.Dial("ws://localhost:"+fmt.Sprint(s.s.port)+"/ws", nil)
}

func TestConnectWebsocket(t *testing.T) {
	s := makeServer()
	_, _, err := connect(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("Connected to websocket server successfully")
}

func TestBadWebsocket(t *testing.T) {
	s := makeServer()
	d := &websocket.Dialer{}
	_, _, err := d.Dial("ws://localhost:"+fmt.Sprint(s.s.port)+"/ws2", nil)
	if err == nil {
		t.Fatal("Successful connection to non-websocket")
	}
}

func TestProvideUsername(t *testing.T) {
	s := makeServer()
	conn, _, err := connect(s)
	if err != nil {
		t.Fatal(err)
	}
	fromServer := toClient{}
	conn.ReadJSON(&fromServer)
	switch fromServer.Type {
	case "PromptEchoOn":
		log.Info().Msg("Sending username to server")
		conn.WriteJSON(fromClient{
			Input: "achernya",
		})
	default:
		t.Fatalf("Unexpected message type: %#v", fromServer)
	}
	conn.ReadJSON(&fromServer)
	log.Info().Msgf("%#v", fromServer)
}
