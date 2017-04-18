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

func main() {

	// Load the templates.
	tmpl := template.Must(template.ParseFiles(filepath.Join("example", "index.html")))

	// Create the redis pool, assuming a default here.
	pool, err := heximage.ConnectRedis("redis://localhost:6379", 0)
	if err != nil {
		fmt.Println("Can't connect to redis", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Create a connection to use.
	conn := pool.Get()
	defer conn.Close()

	const heximageBind = "localhost:8080"

	// Start the heximage server.
	go heximage.StartServer(heximageBind, pool, conn, []string{"http://localhost:8000"})

	// Prepare to serve the index.html file from the example directory.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Execute(w, map[string]interface{}{
			"Origin": heximageBind,
		})
	})

	// Start the server.
	log.Println("Now listening for requests on localhost:8000")
	http.ListenAndServe("localhost:8000", nil)
}
