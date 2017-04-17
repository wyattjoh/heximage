package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
)

const (
	key     = "heximage"
	width   = 1000
	height  = 1000
	quality = 100
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

	colour, err := strconv.ParseUint(colours, 10, 8)
	if err != nil {
		return errors.Wrap(err, "can't parse color")
	}

	offset := (width*(y-1) + (x - 1)) * 8

	if _, err := conn.Do("BITFIELD", key, "SET", "u8", offset, colour); err != nil {
		return err
	}

	return nil
}

// GetImage returns an image.
func GetImage(conn redis.Conn) error {
	data, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return errors.Wrap(err, "can't get the image bytes")
	}

	if len(data) != width*height {
		newData := make([]uint8, width*height)
		copy(newData, data)
		data = newData
	}

	img := image.NewRGBA(image.Rect(0, 0, width/2, height/2))

	img.Pix = data

	enc := png.Encoder{
		CompressionLevel: png.BestCompression,
	}

	if err := enc.Encode(os.Stdout, img); err != nil {
		return err
	}

	return nil
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

	// offset := (width*(height-1) + (width - 1)) * 8

	// if _, err := conn.Do("BITFIELD", key, "SET", "u8", offset, "0"); err != nil {
	// 	return err
	// }

	return nil
}

func usage() {
	fmt.Println("heximage [init|set|get|clear]")
	os.Exit(1)
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
		if err := GetImage(conn); err != nil {
			fmt.Printf("Can't get the image: %s\n", err.Error())
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
