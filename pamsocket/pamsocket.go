package pamsocket

import (
	"context"
	"net/http"
	"os/user"

	"github.com/gorilla/websocket"
	"github.com/msteinert/pam/v2"
	"github.com/rs/zerolog/log"
)

// fromClient is a message sent from the client over the
// websocket to this server.
type fromClient struct {
	// The input data from the client.
	Input string
}

type toClient struct {
	// Type indicates the type of message this is. It will be one
	// of either `PromptEchoOff` requesting user input (e.g., a
	// password), `PromptEchoOn` requesting user input (e.g., a
	// username), `Error`, containing an error string, `Info`,
	// containing an informational message, or `Redirect`,
	// containing a URL to navigate to next.
	Type string
	// message is the actual payload. What to do with it depends
	// on the value of Type.
	Message string
}

type Scope struct {
	Name        string
	Description string
	Hidden      bool
}

type ConsentInfo struct {
	// If set, Redirect contains the URL to redirect to
	// immediately. No consent screen needs to be shown.
	Redirect string
	//
	Target string
	//
	Scopes []*Scope
}

type LoginFlow interface {
	// PreLogin is run before the sign-in flow. It may conclude
	// the sign-in flow is unnecessary, and return a URL to
	// redirect to. If an empty URL is returned, the login flow
	// should proceed.
	PreLogin(r *http.Request) (string, error)
	// Authenticated is run after the sign-in flow, to indicate
	// that the given user has been authenticated. This function
	// should return a URL to redirect to. This accepts a
	// `subject`, not a username, so it could be an anonymized
	// identifier (i.e., the UID) instead of the
	// username/email/etc. This must be a stable identifier.
	Authenticated(r *http.Request, subject string) (string, error)
	// RequestConsent is called after a user is authenticated to
	// determine if the target application should be permitted to
	// learn some information (such as username, or full name, or
	// email).
	RequestConsent(r *http.Request) (*ConsentInfo, error)
	// AcceptConsent is called after the user specifies they
	// accept the requested application learn some information.
	AcceptConsent(r *http.Request) (string, error)
	// SupportsOidc returns true if this login flow is OpenID
	// Connect capable, and false otherwise. This is primarily
	// used for testing, wher the NoopFlow indicates it does not
	// support OIDC so that the client-side app will not register
	// its URL handlers.
	SupportsOidc() bool
}

type NoopFlow struct{}

func (*NoopFlow) PreLogin(*http.Request) (string, error) {
	return "", nil
}

func (*NoopFlow) Authenticated(*http.Request, string) (string, error) {
	return "/authenticated", nil
}

func (*NoopFlow) RequestConsent(r *http.Request) (*ConsentInfo, error) {
	return &ConsentInfo{
		Redirect: "/consented",
	}, nil
}

func (*NoopFlow) AcceptConsent(r *http.Request) (string, error) {
	return "/accept", nil
}

func (*NoopFlow) SupportsOidc() bool { return false }

// PamSocket implements a WebSocket-based PAM session. PAM is
// transactional, so running it over a WebSocket guarantees that all
// messages between the client (browser) and server are sent to the
// same task, without any additional session-based routing.
type PamSocket struct {
	// Service is the specific PAM profile to use. This
	// corresponds to a configuration file of the same name in the
	// configured ConfDir. Typically, this is something like
	// `passwd`, but note that that requires running this program
	// with privileges to read /etc/shadow, which is not
	// generally recommended.
	Service string
	// ConfDir is the directory where the PAM service
	// configurations live. By default, this is `/etc/pam.d/`.
	ConfDir string
	// Flow is a series of functions that are called as part of
	// the login process. If you do not need to customize the
	// login process, use NoopFlow.
	Flow LoginFlow
}

// session represents a single PAM session, bound to a websocket.Conn
// connection.
type session struct {
	// The active websocket connection with the client.
	conn *websocket.Conn
	// A channel that contains messages from the client. Populated
	// by the `readFromClient` goroutine.
	clientMsgs chan fromClient
}

// readFromClient actively reads all JSON messages from the client,
// and makes them available in a select'able channel, until cancelled.
func (s *session) readFromClient(ctx context.Context, conn *websocket.Conn) {
	for {
		msg := fromClient{}
		err := conn.ReadJSON(&msg)
		select {
		case <-ctx.Done():
			return
		case s.clientMsgs <- msg:
		}
		if err != nil {
			log.Error().Err(err).Msg("ReadJSON failed")
			conn.Close()
			return
		}
	}
}

// RespondPAM satisifies the pam.ConversationHandler interface. It is
// called by the PAM session whenever PAM needs to interact with the
// user, either to display a message, or request input.
//
// This function directly communicates with the client over the
// websocket and no other concurrent messaging must occur until the
// PAM conversation has quiesced.
func (s *session) RespondPAM(style pam.Style, m string) (string, error) {
	msg := toClient{
		Message: m,
	}
	switch style {
	case pam.PromptEchoOff:
		msg.Type = "PromptEchoOff"
	case pam.PromptEchoOn:
		msg.Type = "PromptEchoOn"
	case pam.ErrorMsg:
		msg.Type = "Error"
	case pam.TextInfo:
		msg.Type = "Info"
	}

	// Regardless of the type, the client needs to get this message
	log.Info().Msgf("Sending %#v", msg)
	s.conn.WriteJSON(msg)

	// However, a client response is only needed in some cases
	switch style {
	case pam.PromptEchoOff:
		fallthrough
	case pam.PromptEchoOn:
		response := <-s.clientMsgs
		return response.Input, nil
	default:
	}
	return "", nil
}

func (s *session) writeErr(message string) {
	s.conn.WriteJSON(toClient{
		Type:    "Error",
		Message: message,
	})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func (p *PamSocket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Info().Err(err).Msg("Could not upgrade to websocket")
		return
	}
	// Ensure the connection is closed when this function ends.
	defer conn.Close()

	s := &session{
		conn:       conn,
		clientMsgs: make(chan fromClient, 1),
	}

	redirect, err := p.Flow.PreLogin(r)
	if err != nil {
		s.writeErr(err.Error())
		return
	}
	if redirect != "" {
		conn.WriteJSON(toClient{
			Type:    "Redirect",
			Message: redirect,
		})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go s.readFromClient(ctx, conn)

	// Start the PAM conversation, with no username provided. PAM
	// will request one, if needed.
	t, err := pam.StartConfDir(p.Service, "", s, p.ConfDir)
	if err != nil {
		log.Fatal().Msgf("Cannot start PAM session: %v", err)
		return
	}
	defer t.End()

	err = t.Authenticate(0)
	if err != nil {
		log.Info().Err(err).Msg("Could not authenticate user")
		s.writeErr("Authentication failed.")
		return
	}

	username, err := t.GetItem(pam.User)
	if err != nil {
		log.Error().Err(err).Msg("Could not retrieve username")
		s.writeErr("Internal error")
		return
	}
	userinfo, err := user.Lookup(username)
	if err != nil {
		log.Error().Err(err).Msg("Could not retrieve UNIX user account information")
		s.writeErr("Internal error")
		return
	}
	log.Info().Msgf("Authenticated %q (uid=%q)", username, userinfo.Uid)

	redirect, err = p.Flow.Authenticated(r, string(userinfo.Uid))
	if err != nil {
		s.writeErr(err.Error())
		return
	}
	conn.WriteJSON(toClient{
		Type:    "Redirect",
		Message: redirect,
	})
	log.Info().Msgf("Sent redirect for %q", username)
}
