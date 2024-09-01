package commands

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"

	"github.com/achernya/nonstick/frontend"
	"github.com/achernya/nonstick/pamsocket"
	"github.com/gorilla/mux"
	"github.com/gorilla/csrf"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v2"

	vueglue "github.com/torenware/vite-go"
	tmpls "github.com/achernya/nonstick/template" 
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

func index(w http.ResponseWriter, r *http.Request) {
	if err := t.ExecuteTemplate(w, "index.tmpl", vue); err != nil {
		log.Fatal().Err(err).Msg("Could not execute template")
	}
}

func devserverRedirect(w http.ResponseWriter, r *http.Request) {
	url := vue.BaseURL + r.RequestURI
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func serve(c *cli.Context) error {
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
	
	r := mux.NewRouter()

	// Set up a file server for our assets.
	fsHandler, err := glue.FileServer()
	if err != nil {
		return err
	}
	log.Info().Msgf("Serving files from %q", config.URLPrefix)
	r.PathPrefix(config.URLPrefix).Handler(fsHandler)

	csrfOptions := []csrf.Option{}
	if config.Environment == "development" {
		csrfOptions = append(csrfOptions, csrf.Secure(false))
	}
	csrfMiddleware := csrf.Protect([]byte(c.String("csrf_secret")),
		csrfOptions...)
	r.Use(csrfMiddleware)
	
	api := r.PathPrefix("/api").Subrouter()
	api.Handle("/pamws", &pamsocket.PamSocket{
		Service: "google-authenticator",
		ConfDir: "pam.d/",
	}).Methods("GET")

	r.HandleFunc("/", index)

	port := c.String("port")

	log.Info().Msgf("Listening on %s", port)
	src := &http.Server{
		Handler: r,
		Addr: ":"+port,
	}
	return src.ListenAndServe()
}
