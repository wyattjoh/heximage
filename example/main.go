package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	heximage "github.com/wyattjoh/heximage/lib"
)

func main() {

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

	// Start the heximage server.
	go heximage.StartServer("localhost:8080", pool, conn, []string{"http://localhost:8000"})

	// Prepare to serve the index.html file from the example directory.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "example/index.html")
	})

	// Start the server.
	log.Println("Now listening for requests on localhost:8000")
	http.ListenAndServe("localhost:8000", nil)
}
