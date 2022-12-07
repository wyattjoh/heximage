package heximage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"time"

	// Import pprof so we can measure runtime performance.
	_ "net/http/pprof"

	"github.com/disintegration/imaging"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/rs/cors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/negroni"
)

// timeout is the cache timeout used to add to requests to prevent the
// browser from re-requesting the image.
const timeout = 30 * time.Second

// StartServer runs an http server capable of serving requests for the image
// service.
func StartServer(addr string, pool *redis.Pool, conn redis.Conn, corsOrigins []string) error {
	mux := http.NewServeMux()

	mux.Handle("/api/place/live", HandleLiveConnection(pool, conn))
	mux.Handle("/api/place/draw", HandleCreatePixel(pool))
	mux.Handle("/api/place/board", HandleGetBoard(pool))
	mux.Handle("/api/place/board-bitmap", HandleGetBoardBitmap(pool))

	n := negroni.Classic() // Includes some default middlewares

	if len(corsOrigins) != 0 {
		n.Use(cors.New(cors.Options{
			AllowedOrigins: corsOrigins,
		}))
	}

	n.UseHandler(mux)

	log.Debugf("Now listening on %s", addr)
	return http.ListenAndServe(addr, n)
}

// HandleLiveConnection brokers the websocket connection with redis.
func HandleLiveConnection(pool *redis.Pool, psconn redis.Conn) http.HandlerFunc {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {

			// This looks wrong, but the origin is checked in the middleware (which
			// happens before this handler) and therefore we should just skip this.
			return true
		},
	} // use default options

	messages := make(chan []byte)
	newClients := make(chan *websocket.Conn)
	closingClients := make(chan *websocket.Conn)

	// Create the pubsub connection and subscribe.
	psc := redis.PubSubConn{Conn: psconn}

	psc.Subscribe(updatesKey)

	// This routine will keep track of new clients connecting so we know if we
	// need to send them data
	go func() {
		clients := make(map[*websocket.Conn]bool)

		for {
			select {
			case con := <-newClients:

				log.Debugf("CML: A new client has connected")
				clients[con] = true

			case msg := <-messages:

				log.Debugf("CML: A new message needs to be sent")
				for con := range clients {
					if err := con.WriteMessage(websocket.TextMessage, msg); err != nil {
						log.Debugf("CML: Writing to a client failed: %s", err.Error())
						delete(clients, con)
						continue
					}
				}

			case con := <-closingClients:

				log.Debugf("CML: A client has disconnected.")
				delete(clients, con)

			}
		}
	}()

	go func() {

		for {
			switch n := psc.Receive().(type) {
			case redis.Message:

				// A new message from Redis, send it to all connected clients.
				log.Debugf("RD: New Message: %s", string(n.Data))
				messages <- n.Data

			case redis.Subscription:

				log.Debugf("RD: Subscription: %d", n.Count)

			case error:

				// An error was encountered while managing the pubsub connection.
				log.Debugf("RD: Error: %s", n.Error())
				return
			}
		}
	}()

	return func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() {
			closingClients <- c
			c.Close()
		}()

		newClients <- c

		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				break
			}

			log.Debugf("WS: New Message: %s", string(message))

			var buf = bytes.NewBuffer(message)

			var px Pixel
			if err := json.NewDecoder(buf).Decode(&px); err != nil {
				log.Debugf("WS: Error: %s", err.Error())
				continue
			}

			conn := pool.Get()

			if err := SetColour(conn, px); err != nil {
				log.Debugf("WS: Error: %s", err.Error())
				conn.Close()
				continue
			}

			conn.Close()
		}
	}
}

// HandleCreatePixel handles creating pixels on the image.
func HandleCreatePixel(pool *redis.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		conn := pool.Get()
		defer conn.Close()

		var px Pixel
		if err := json.NewDecoder(r.Body).Decode(&px); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if err := SetColour(conn, px); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// HandleGetBoard handles serving the board as a png image.
func HandleGetBoard(pool *redis.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		conn := pool.Get()
		defer conn.Close()

		img, err := GetImage(conn)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		widthParam := r.URL.Query().Get("w")
		if widthParam != "" {
			if widthValue, err := strconv.Atoi(widthParam); err == nil {
				img = imaging.Resize(img, widthValue, 0, imaging.NearestNeighbor)
			}
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int64(timeout.Seconds())))
		w.Header().Set("Last-Modified", time.Now().String())

		enc := png.Encoder{
			CompressionLevel: png.BestSpeed,
		}

		if err := enc.Encode(w, img); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// HandleGetBoardBitmap handles serving the board as a png image.
func HandleGetBoardBitmap(pool *redis.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		conn := pool.Get()
		defer conn.Close()

		img, err := GetImage(conn)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int64(timeout.Seconds())))
		w.Header().Set("Last-Modified", time.Now().String())

		rimg, ok := img.(*image.RGBA)
		if !ok {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		buf := bytes.NewBuffer(rimg.Pix)

		if _, err := io.Copy(w, buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
