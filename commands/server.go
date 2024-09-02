package commands

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"

	"github.com/achernya/nonstick/frontend"
	"github.com/achernya/nonstick/pamsocket"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/openidConnect"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"

	tmpls "github.com/achernya/nonstick/template"
	vueglue "github.com/torenware/vite-go"
)

var vue *vueglue.VueGlue

var t *template.Template

func outboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal().Err(err).Msg("Could not connect to well-known IP, do you have network?")
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func idpLogin(w http.ResponseWriter, r *http.Request) {
	if err := t.ExecuteTemplate(w, "index.tmpl", vue); err != nil {
		log.Fatal().Err(err).Msg("Could not execute template")
	}
}

func renderUserInfo(w http.ResponseWriter, r *http.Request, user goth.User) {
	merged := struct {
		*vueglue.VueGlue
		User goth.User
	}{
		vue,
		user,
	}
	if err := t.ExecuteTemplate(w, "userinfo.tmpl", merged); err != nil {
		log.Fatal().Err(err).Msg("Could not execute template")
	}

}

func appCallback(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		log.Error().Err(err).Msg("Got invalid auth callback")
		return
	}
	renderUserInfo(w, r, user)
}

func appLogout(w http.ResponseWriter, r *http.Request) {
	gothic.Logout(w, r)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func appReauth(w http.ResponseWriter, r *http.Request) {
	if user, err := gothic.CompleteUserAuth(w, r); err == nil {
		log.Info().Msgf("Got user %+v, no need to re-auth", user)
		renderUserInfo(w, r, user)
	} else {
		gothic.BeginAuthHandler(w, r)
	}
}

func devserverRedirect(w http.ResponseWriter, r *http.Request) {
	url := vue.BaseURL + r.RequestURI
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func serve(c *cli.Context) error {
	// Load things from .env, if needed.
	if c.Bool("use_dotenv") {
		err := godotenv.Load()
		if err != nil {
			log.Fatal().Err(err).Msg(".env file failed to load!")
		}
	}
	port := c.String("port")

	// Common initialization to serve Vite/Vue.
	var config *vueglue.ViteConfig
	switch env := c.String("env"); env {
	case "dev":
		config = &vueglue.ViteConfig{
			Environment:     "development",
			AssetsPath:      "src/assets",
			EntryPoint:      "src/main.js",
			FS:              os.DirFS("frontend"),
			DevServerDomain: outboundIP().String(),
		}
	case "prod":
		config = &vueglue.ViteConfig{
			Environment: "production",
			AssetsPath:  "dist",
			EntryPoint:  "src/main.js",
			FS:          frontend.Fs,
		}
	default:
		return fmt.Errorf("Unknown environment %q", env)
	}

	glue, err := vueglue.NewVueGlue(config)
	if err != nil {
		return err
	}
	vue = glue

	t, err = template.New("").ParseFS(tmpls.Fs, "*")
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid templates")
	}

	// Actually start adding URL routing
	r := mux.NewRouter()

	// Enable CSRF protection, for any POST requests.
	csrfOptions := []csrf.Option{}
	if config.Environment == "development" {
		csrfOptions = append(csrfOptions, csrf.Secure(false))
	}
	csrfMiddleware := csrf.Protect([]byte(c.String("csrf_secret")),
		csrfOptions...)
	r.Use(csrfMiddleware)

	// Set up a file server for our assets.
	fsHandler, err := glue.FileServer()
	if err != nil {
		return err
	}
	log.Info().Msgf("Serving files from %q", config.URLPrefix)
	r.PathPrefix(config.URLPrefix).Handler(fsHandler)

	// Set up the PAM websocket
	api := r.PathPrefix("/api").Subrouter()
	flowArg := c.String("login_flow")
	var flow pamsocket.LoginFlow
	switch flowArg {
	case "hydra":
		flow = NewOryHydraFlow()
	case "noop":
		flow = &pamsocket.NoopFlow{}
	}
	api.Handle("/pamws", &pamsocket.PamSocket{
		Service: "google-authenticator",
		ConfDir: "pam.d/",
		Flow:    flow,
	}).Methods("GET")

	// IdP authentication flow URL handlers
	r.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		redirect, _ := flow.PreLogin(r)
		if redirect != "" {
			http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
		}
		idpLogin(w, r)
	})
	r.HandleFunc("/consent", func(w http.ResponseWriter, r *http.Request) {
		redirect, err := flow.AcceptConsent(r)
		if err != nil {
			log.Error().Err(err).Msg("Failed to accept consent")
			return
		}
		http.Redirect(w, r, redirect, http.StatusTemporaryRedirect)
	})

	// User management application URL handlers
	if flowArg == "hydra" {
		openidConnect, err := openidConnect.New(os.Getenv("OPENID_CONNECT_KEY"), os.Getenv("OPENID_CONNECT_SECRET"),
			"http://"+outboundIP().String()+":"+port+"/auth/openid-connect/callback", os.Getenv("OPENID_CONNECT_DISCOVERY_URL"))
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to set up OpenID Connect for Hydra")
		}
		goth.UseProviders(openidConnect)
		r.HandleFunc("/auth/{provider}/callback", appCallback)
		r.HandleFunc("/logout/{provider}", appLogout)
		r.HandleFunc("/auth/{provider}", appReauth)
		r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/auth/openid-connect", http.StatusTemporaryRedirect)
		})
	} else {
		r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
		})
	}

	goth.UseProviders()

	log.Info().Msgf("Listening on %s", port)
	src := &http.Server{
		Handler: r,
		Addr:    ":" + port,
	}
	return src.ListenAndServe()
}
