package commands

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"

	"github.com/urfave/cli/v2"

	vueglue "github.com/torenware/vite-go"
)

var vue *vueglue.VueGlue

func outboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	
	return localAddr.IP
}

func index(w http.ResponseWriter, r *http.Request) {
	re := regexp.MustCompile(`^/([^.]+)\.(svg|ico|jpg)$`)
	matches := re.FindStringSubmatch(r.RequestURI)
	if matches != nil {
		if vue.Environment == "development" {
			log.Printf("vite logo requested")
			url := vue.BaseURL + r.RequestURI
			http.Redirect(w, r, url, http.StatusPermanentRedirect)
			return
		}
	}
	
	t, err := template.New("").ParseFS(vue.DistFS, "*.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	if err := t.ExecuteTemplate(w, "index.tmpl", vue); err != nil {
		log.Fatal(err)
	}

}

func serve(c *cli.Context) error {
	var config *vueglue.ViteConfig
	switch env := c.String("env"); env {
	case "dev":
		config = &vueglue.ViteConfig{
			Environment: "development",
			AssetsPath:  "frontend",
			EntryPoint:  "src/main.js",
			FS:          os.DirFS("frontend"),
		}
	case "prod":
		config = &vueglue.ViteConfig{
			Environment: "production",
			AssetsPath:  "dist",
			EntryPoint:  "src/main.js",
			FS:          os.DirFS("dist"),
		}
	default:
		return fmt.Errorf("Unknown environment %q", env)
	}

	port := c.String("port")
	config.DevServerDomain = outboundIP().String()
	//config.DevServerPort = port
	
	glue, err := vueglue.NewVueGlue(config)
	if err != nil {
		return err
	}
	vue = glue

	mux := http.NewServeMux()
	
	// Set up a file server for our assets.
	fsHandler, err := glue.FileServer()
	if err != nil {
		return err
	}
	log.Printf("Serving files from %q", config.URLPrefix)
	mux.Handle(config.URLPrefix, fsHandler)

	mux.Handle("/", http.HandlerFunc(index))

	
	log.Printf("Listening on %s", port)
	http.ListenAndServe(":" + port, mux)
	
	return nil
}
