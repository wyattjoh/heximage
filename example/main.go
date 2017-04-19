package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"

	heximage "github.com/wyattjoh/heximage/lib"
)

const (
	bindIP   = "localhost"
	apiBind  = bindIP + ":8080"
	temBind  = bindIP + ":8000"
	redisURL = "redis://localhost:6379"
)

func main() {

	// Load the templates.
	tmpl := template.Must(template.ParseFiles(filepath.Join("example", "index.html")))

	// Create the redis pool, assuming a default here.
	pool, err := heximage.ConnectRedis(redisURL, 0)
	if err != nil {
		fmt.Println("Can't connect to redis", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Create a connection to use.
	conn := pool.Get()
	defer conn.Close()

	// Start the heximage server.
	go func() {
		log.Printf("Now serving the api on %s", apiBind)
		if err := heximage.StartServer(apiBind, pool, conn, []string{"http://" + temBind}); err != nil {
			fmt.Println("Can't serve the api server", err)
			os.Exit(1)
		}
	}()

	// Prepare to serve the index.html file from the example directory.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Execute(w, map[string]interface{}{
			"Origin": apiBind,
		})
	})

	// Start the server.
	log.Printf("Now serving the template on %s", temBind)
	if err := http.ListenAndServe(temBind, nil); err != nil {
		fmt.Println("Can't serve the template server", err)
		os.Exit(1)
	}
}
