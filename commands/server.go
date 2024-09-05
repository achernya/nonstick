package commands

import (
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/achernya/nonstick/frontend"
	"github.com/achernya/nonstick/pamsocket"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/openidConnect"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"

	tmpls "github.com/achernya/nonstick/template"
	vueglue "github.com/torenware/vite-go"
)

func outboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal().Err(err).Msg("Could not connect to well-known IP, do you have network?")
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

type server struct {
	port      string
	config    *vueglue.ViteConfig
	glue      *vueglue.VueGlue
	templates map[string]*template.Template
	router    *mux.Router
	flow      pamsocket.LoginFlow
}

func makeServer(port string, env string) (*server, error) {
	result := &server{
		port:      port,
		router:    mux.NewRouter(),
		templates: make(map[string]*template.Template),
	}
	// Common initialization to serve Vite/Vue.
	switch env {
	case "dev":
		result.config = &vueglue.ViteConfig{
			Environment:     "development",
			AssetsPath:      "src/assets",
			EntryPoint:      "src/main.js",
			FS:              os.DirFS("frontend"),
			DevServerDomain: outboundIP().String(),
		}
	case "prod":
		result.config = &vueglue.ViteConfig{
			Environment: "production",
			AssetsPath:  "dist",
			EntryPoint:  "src/main.js",
			FS:          frontend.Fs,
		}
	default:
		return nil, fmt.Errorf("Unknown environment %q", env)
	}
	var err error
	result.glue, err = vueglue.NewVueGlue(result.config)
	if err != nil {
		return nil, err
	}

	// Parse all of the templates and make sure they're ready to go.
	matches, err := fs.Glob(tmpls.Fs, "pages/*")
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		filename := filepath.Base(match)
		page := strings.TrimSuffix(filename, filepath.Ext(filename))
		t, err := template.New("").ParseFS(tmpls.Fs, "includes/*.tmpl", match)
		if err != nil {
			return nil, err
		}

		result.templates[page] = t
	}

	return result, nil
}

func (s *server) registerUrls(csrfSecret []byte) error {
	// Enable CSRF protection, for any POST requests.
	csrfOptions := []csrf.Option{}
	if s.glue.Environment == "development" {
		csrfOptions = append(csrfOptions, csrf.Secure(false))
	}
	csrfMiddleware := csrf.Protect(csrfSecret, csrfOptions...)
	s.router.Use(csrfMiddleware)

	// Set up a file server for our assets.
	fsHandler, err := s.glue.FileServer()
	if err != nil {
		return err
	}
	log.Info().Msgf("Serving files from %q", s.config.URLPrefix)
	s.router.PathPrefix(s.config.URLPrefix).Handler(fsHandler)

	// IdP authentication flow URL handlers
	s.router.HandleFunc("/login", s.idpLogin)
	s.router.HandleFunc("/consent", s.getConsent).Methods("GET")
	s.router.HandleFunc("/consent", s.postConsent).Methods("POST")

	// pamsocket itself
	s.router.Handle("/api/pamws", &pamsocket.PamSocket{
		Service: "google-authenticator",
		ConfDir: "pam.d/",
		Flow:    s.flow,
	}).Methods("GET")

	// User management app (primarily a testing app for OIDC)
	if s.flow.SupportsOidc() {
		openidConnect, err := openidConnect.New(os.Getenv("OPENID_CONNECT_KEY"), os.Getenv("OPENID_CONNECT_SECRET"),
			"http://"+outboundIP().String()+":"+s.port+"/auth/openid-connect/callback", os.Getenv("OPENID_CONNECT_DISCOVERY_URL"), "profile")
		if err != nil {
			return err
		}
		goth.UseProviders(openidConnect)
		s.router.HandleFunc("/auth/{provider}/callback", s.appCallback)
		s.router.HandleFunc("/logout/{provider}", s.appLogout)
		s.router.HandleFunc("/auth/{provider}", s.appReauth)
		s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/auth/openid-connect", http.StatusTemporaryRedirect)
		})
	} else {
		s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		})
	}

	// Pretty 404 pages
	s.router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		s.renderTemplate("error", map[string]interface{}{
			"Message": "404 Not found",
		}, w)
	})

	return nil
}

func (s *server) renderTemplate(page string, tmplArgs map[string]interface{}, w http.ResponseWriter) {
	t, ok := s.templates[page]
	if !ok {
		log.Fatal().Msgf("Could not find page %q", page)
	}
	if tmplArgs == nil {
		tmplArgs = make(map[string]interface{})
	}
	tmplArgs["vue"] = s.glue
	if err := t.ExecuteTemplate(w, "page", tmplArgs); err != nil {
		log.Fatal().Err(err).Msgf("Could not execute template %q", page)
	}
}

func (s *server) idpLogin(w http.ResponseWriter, r *http.Request) {
	redirect, _ := s.flow.PreLogin(r)
	if redirect != "" {
		http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
	}
	s.renderTemplate("login", nil, w)
}

func (s *server) renderUserInfo(w http.ResponseWriter, r *http.Request, user goth.User) {
	s.renderTemplate("userinfo", map[string]interface{}{
		"User": user,
	}, w)
}

func (s *server) respondWithError(w http.ResponseWriter, r *http.Request, message string) {
	w.WriteHeader(http.StatusBadRequest)
	s.renderTemplate("error", map[string]interface{}{
		"Message": message,
	}, w)
}

func (s *server) appCallback(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		log.Error().Err(err).Msg("Got invalid auth callback")
		s.respondWithError(w, r, err.Error())
		return
	}
	s.renderUserInfo(w, r, user)
}

func (s *server) appLogout(w http.ResponseWriter, r *http.Request) {
	gothic.Logout(w, r)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func (s *server) appReauth(w http.ResponseWriter, r *http.Request) {
	if user, err := gothic.CompleteUserAuth(w, r); err == nil {
		log.Info().Msgf("Got user %+v, no need to re-auth", user)
		s.renderUserInfo(w, r, user)
	} else {
		gothic.BeginAuthHandler(w, r)
	}
}

func (s *server) getConsent(w http.ResponseWriter, r *http.Request) {
	info, err := s.flow.RequestConsent(r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to accept consent")
		s.respondWithError(w, r, err.Error())
		return
	}
	if info.Redirect != "" {
		http.Redirect(w, r, info.Redirect, http.StatusTemporaryRedirect)
		return
	}
	s.renderTemplate("consent", map[string]interface{}{
		"CsrfField": csrf.TemplateField(r),
		"Info":      info,
	}, w)
}

func (s *server) postConsent(w http.ResponseWriter, r *http.Request) {
	redirect, err := s.flow.AcceptConsent(r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to accept consent")
		s.respondWithError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
}

func serve(c *cli.Context) error {
	// Load things from .env, if needed.
	if c.Bool("use_dotenv") {
		err := godotenv.Load()
		if err != nil {
			log.Fatal().Err(err).Msg(".env file failed to load!")
		}
	}

	// Avoid an error being printed by gothic if SESSION_SECRET
	// was set by dotenv, which will run after gothic's init().
	store := sessions.NewCookieStore([]byte(os.Getenv("SESSION_SECRET")))
	gothic.Store = store

	server, err := makeServer(c.String("port"), c.String("env"))
	if err != nil {
		return err
	}

	switch flowArg := c.String("login_flow"); flowArg {
	case "hydra":
		server.flow = NewOryHydraFlow()
	case "noop":
		server.flow = &pamsocket.NoopFlow{}
	}

	server.registerUrls([]byte(c.String("csrf_secret")))

	log.Info().Msgf("Listening on %s", server.port)
	src := &http.Server{
		Handler: server.router,
		Addr:    ":" + server.port,
	}
	return src.ListenAndServe()
}
