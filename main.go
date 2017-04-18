package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/disintegration/imaging"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"github.com/rs/cors"
	"github.com/urfave/cli"
	"github.com/urfave/negroni"
)

const (
	// timeout is the cache timeout used to add to requests to prevent the
	// browser from re-requesting the image.
	timeout = 30 * time.Second

	key        = "heximage"
	updatesKey = key + ":updates"
	width      = 50
	height     = 50
	bits       = width * height * 4
)

// SetColour sets the colour on a pixel of the image.
func SetColour(conn redis.Conn, xs, ys, colours string) error {

	x, err := strconv.ParseUint(xs, 10, 32)
	if err != nil {
		return errors.Wrap(err, "can't parse x")
	}

	y, err := strconv.ParseUint(ys, 10, 32)
	if err != nil {
		return errors.Wrap(err, "can't parse y")
	}

	if x > width || y > height || x == 0 || y == 0 {
		return errors.New("invalid pixel location supplied")
	}

	colour, err := strconv.ParseUint(colours, 16, 32)
	if err != nil {
		return errors.Wrap(err, "can't parse color")
	}

	if err := SendSet(conn, uint32(x), uint32(y), uint32(colour)); err != nil {
		return err
	}

	if err := conn.Flush(); err != nil {
		return err
	}

	return nil
}

// SendSet sends the set comment but does not flush it.
func SendSet(conn redis.Conn, x, y, colour uint32) error {

	offset := (width*(y-1) + (x - 1)) * 32

	return conn.Send("BITFIELD", key, "SET", "u32", offset, colour)
}

// GetImage returns an image.
func GetImage(conn redis.Conn) (image.Image, error) {
	data, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return nil, errors.Wrap(err, "can't get the image bytes")
	}

	if len(data) != bits {
		newData := make([]uint8, bits)
		copy(newData, data)
		data = newData
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	img.Pix = data

	return img, nil
}

// ClearImage clears the image.
func ClearImage(conn redis.Conn) error {
	_, err := conn.Do("DEL", key)
	if err != nil {
		return errors.Wrap(err, "can't execute the del command")
	}

	return nil
}

// InitImage initializes the image.
func InitImage(conn redis.Conn) error {
	_, err := conn.Do("DEL", key)
	if err != nil {
		return errors.Wrap(err, "can't execute the del command")
	}

	if err := SendSet(conn, width, height, 0); err != nil {
		return err
	}

	if err := conn.Flush(); err != nil {
		return err
	}

	return nil
}

// TestImage prints a test pattern onto the image.
func TestImage(conn redis.Conn) error {
	i := 0
	colours := []uint32{0xFF0F00FF, 0xF99F00FF, 0xF0FF00FF}
	coloursLen := len(colours)

	for y := uint32(1); y <= height; y++ {
		for x := uint32(1); x <= width; x++ {
			colour := colours[i]
			i = (i + 1) % coloursLen

			if err := SendSet(conn, x, y, colour); err != nil {
				return err
			}
		}
	}

	if err := conn.Flush(); err != nil {
		return err
	}

	return nil
}

func usage() {
	fmt.Println("heximage [init|set|get|clear|test]")
	os.Exit(1)
}

// StartServer runs an http server capable of serving requests for the image
// service.
func StartServer(addr string, pool *redis.Pool, conn redis.Conn) error {
	mux := http.NewServeMux()

	mux.Handle("/api/place/live", HandleLiveConnection(pool, conn))
	mux.Handle("/api/place/draw", HandleCreatePixel(pool))
	mux.Handle("/api/place/board", HandleGetBoard(pool))
	mux.Handle("/api/place/board-bitmap", HandleGetBoardBitmap(pool))

	n := negroni.Classic() // Includes some default middlewares
	n.Use(cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:8000"},
	}))
	n.UseHandler(mux)

	log.Printf("Now listening on %s", addr)
	return http.ListenAndServe(addr, n)
}

// HandleLiveConnection brokers the websocket connection with redis.
func HandleLiveConnection(pool *redis.Pool, psconn redis.Conn) http.HandlerFunc {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
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

				logrus.Debugf("CML: A new client has connected")
				clients[con] = true

			case msg := <-messages:

				logrus.Debugf("CML: A new message needs to be sent")
				for con := range clients {
					if err := con.WriteMessage(websocket.TextMessage, msg); err != nil {
						logrus.Debugf("CML: Writing to a client failed: %s", err.Error())
						delete(clients, con)
						continue
					}

					logrus.Debugf("CML: Sent message")
				}

			case con := <-closingClients:

				logrus.Debugf("CML: A client has disconnected.")
				delete(clients, con)

			}
		}
	}()

	go func() {

		for {
			switch n := psc.Receive().(type) {
			case redis.Message:

				// A new message from Redis, send it to all connected clients.
				logrus.Debugf("RD: New Message: %s", string(n.Data))
				messages <- n.Data

			case redis.Subscription:

				logrus.Debugf("RD: Subscription: %d", n.Count)

			case error:

				// An error was encoutnered while managing the pubsub connection.
				logrus.Debugf("RD: Error: %s", n.Error())
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

			logrus.Debugf("WS: New Message: %s", string(message))

			var buf = bytes.NewBuffer(message)

			var pl struct {
				X, Y, Colour string
			}
			if err := json.NewDecoder(buf).Decode(&pl); err != nil {
				logrus.Debugf("WS: Error: %s", err.Error())
				continue
			}

			conn := pool.Get()

			if err := SetColour(conn, pl.X, pl.Y, pl.Colour); err != nil {
				logrus.Debugf("WS: Error: %s", err.Error())
				conn.Close()
				continue
			}

			if _, err := conn.Do("PUBLISH", updatesKey, string(message)); err != nil {
				logrus.Debugf("WS: Error: %s", err.Error())
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

		var pl struct {
			X, Y, Colour string
		}
		if err := json.NewDecoder(r.Body).Decode(&pl); err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if err := SetColour(conn, pl.X, pl.Y, pl.Colour); err != nil {
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

func main() {
	var pool *redis.Pool
	var conn redis.Conn

	app := cli.NewApp()
	app.Name = "heximage"
	app.Usage = "mimics reddit.com/r/place experiment"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enables debug mode",
		},
		cli.StringFlag{
			Name:  "redis-url",
			Usage: "url to the redis instance",
			Value: "redis://localhost:6379",
		},
		cli.IntFlag{
			Name:  "redis-max-clients",
			Usage: "maximum amount of clients that can connect to the redis instance",
			Value: 0,
		},
	}
	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}

		var err error
		pool, err = ConnectRedis(c.GlobalString("redis-url"), c.GlobalInt("redis-max-clients"))
		if err != nil {
			return cli.NewExitError(errors.Wrap(err, "can't connect to redis"), 1)
		}

		// Grab our single connection to the pool.
		conn = pool.Get()

		return nil
	}
	app.After = func(c *cli.Context) error {

		// Close this connection.
		conn.Close()

		// Closes the pool.
		pool.Close()

		return nil
	}
	app.Commands = []cli.Command{
		{
			Name:  "server",
			Usage: "serves the heximage server",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "listen-addr",
					Usage: "address for the server to listen on",
					Value: "127.0.0.1:8000",
				},
			},
			Action: func(c *cli.Context) error {
				if err := StartServer(c.String("listen-addr"), pool, conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't run the server"), 1)
				}

				return nil
			},
		},
		{
			Name:  "init",
			Usage: "initializes the image canvas",
			Action: func(c *cli.Context) error {
				if err := InitImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't init the image"), 1)
				}

				return nil
			},
		},
		{
			Name:  "set",
			Usage: "sets a pixel's colour on the image canvas",
			Action: func(c *cli.Context) error {
				if c.NArg() != 3 {
					return cli.NewExitError("missing x, y, or colour", 1)
				}

				args := c.Args()
				if err := SetColour(conn, args.Get(0), args.Get(1), args.Get(2)); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't set the colour"), 1)
				}

				return nil
			},
		},
		{
			Name:  "get",
			Usage: "gets the image and return's an encoded png to the stdout",
			Action: func(c *cli.Context) error {
				img, err := GetImage(conn)
				if err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't get the image"), 1)
				}

				enc := png.Encoder{
					CompressionLevel: png.BestCompression,
				}

				if err := enc.Encode(os.Stdout, img); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't encode the image"), 1)
				}

				return nil
			},
		},
		{
			Name:  "clear",
			Usage: "clears the stored image on the canvas and reinitializes it",
			Action: func(c *cli.Context) error {
				if err := ClearImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't clear the image"), 1)
				}

				if err := InitImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't init the image"), 1)
				}

				return nil
			},
		},
		{
			Name:  "test",
			Usage: "prints a test pattern onto the image",
			Action: func(c *cli.Context) error {
				if err := TestImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't set the test pattern"), 1)
				}

				return nil
			},
		},
	}

	app.Run(os.Args)
}
