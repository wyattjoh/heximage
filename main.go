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
func GetImage(conn redis.Conn) error {
	data, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return errors.Wrap(err, "can't get the image bytes")
	}

	if len(data) != bits {
		newData := make([]uint8, bits)
		copy(newData, data)
		data = newData
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

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
