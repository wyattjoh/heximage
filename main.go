package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
)

const (
	// timeout is the cache timeout used to add to requests to prevent the
	// browser from re-requesting the image.
	timeout = 30 * time.Second

	key    = "heximage"
	width  = 1000
	height = 1000
	bits   = width * height * 4
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

	if x > width || y > height {
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
func StartServer(pool *redis.Pool) error {

	http.HandleFunc("/api/place/draw", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != "POST" {
			log.Printf("%s /api/place/draw %d - %s", r.Method, http.StatusMethodNotAllowed, time.Since(start))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		conn := pool.Get()
		defer conn.Close()

		var pl struct {
			X, Y, Colour string
		}
		if err := json.NewDecoder(r.Body).Decode(&pl); err != nil {
			log.Printf("POST /api/place/draw %d - %s", http.StatusBadRequest, time.Since(start))
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		if err := SetColour(conn, pl.X, pl.Y, pl.Colour); err != nil {
			log.Printf("POST /api/place/draw %d - %s", http.StatusInternalServerError, time.Since(start))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		log.Printf("POST /api/place/draw %d - %s", http.StatusOK, time.Since(start))
		w.WriteHeader(http.StatusCreated)
	})

	http.HandleFunc("/api/place/board-bitmap", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != "GET" {
			log.Printf("%s /api/place/board-bitmap %d - %s", r.Method, http.StatusMethodNotAllowed, time.Since(start))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		conn := pool.Get()
		defer conn.Close()

		img, err := GetImage(conn)
		if err != nil {
			log.Printf("%s /api/place/board-bitmap %d - %s", r.Method, http.StatusInternalServerError, time.Since(start))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int64(timeout.Seconds())))
		w.Header().Set("Last-Modified", time.Now().String())

		enc := png.Encoder{
			CompressionLevel: png.BestSpeed,
		}

		if err := enc.Encode(w, img); err != nil {
			log.Printf("GET /api/place/board-bitmap %d - %s", http.StatusInternalServerError, time.Since(start))
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("GET /api/place/board-bitmap %d - %s", http.StatusOK, time.Since(start))
	})

	log.Printf("Now listening on 127.0.0.1:8080")
	return http.ListenAndServe("127.0.0.1:8080", nil)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	pool, err := ConnectRedis("redis://localhost:6379", 10)
	if err != nil {
		fmt.Printf("Can't connect to redis: %s\n", err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "server":
		if err := StartServer(pool); err != nil {
			fmt.Printf("Can't run the server: %s\n", err.Error())
			os.Exit(1)
		}
	}

	conn := pool.Get()
	defer conn.Close()

	switch os.Args[1] {
	case "init":
		if err := InitImage(conn); err != nil {
			fmt.Printf("Can't init the image: %s\n", err.Error())
			os.Exit(1)
		}
	case "set":
		if len(os.Args) != 5 {
			fmt.Println("heximage set <x> <y> <colour>")
			os.Exit(1)
		}

		if err := SetColour(conn, os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Printf("Can't set the colour: %s\n", err.Error())
			os.Exit(1)
		}
	case "get":
		img, err := GetImage(conn)
		if err != nil {
			fmt.Printf("Can't get the image: %s\n", err.Error())
			os.Exit(1)
		}

		enc := png.Encoder{
			CompressionLevel: png.BestCompression,
		}

		if err := enc.Encode(os.Stdout, img); err != nil {
			fmt.Printf("Can't encode the image: %s\n", err.Error())
			os.Exit(1)
		}
	case "test":
		if err := TestImage(conn); err != nil {
			fmt.Printf("Can't set the test pattern: %s\n", err.Error())
			os.Exit(1)
		}
	case "clear":
		if err := ClearImage(conn); err != nil {
			fmt.Printf("Can't clear the image: %s\n", err.Error())
			os.Exit(1)
		}

	default:
		usage()
	}
}
