package heximage

import (
	"encoding/json"
	"image"
	"strconv"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	key        = "heximage"
	updatesKey = key + ":updates"
	width      = 50
	height     = 50
	bits       = width * height * 4
)

// SendSet sends the set comment but does not flush it.
func SendSet(conn redis.Conn, x, y, colour uint32) error {

	offset := (width*(y-1) + (x - 1)) * 32

	log.WithField("query", "redis").Debugf("BITFIELD %s SET u32 %d %d", key, offset, colour)
	return conn.Send("BITFIELD", key, "SET", "u32", offset, colour)
}

// GetImage returns an image.
func GetImage(conn redis.Conn) (image.Image, error) {
	data, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return nil, errors.Wrap(err, "can't get the image bytes")
	}

	log.WithField("query", "redis").Debugf("GET %s", key)

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

	log.WithField("query", "redis").Debugf("DEL %s", key)

	return nil
}

// InitImage initializes the image.
func InitImage(conn redis.Conn) error {
	if err := conn.Send("DEL", key); err != nil {
		return errors.Wrap(err, "can't execute the del command")
	}

	log.WithField("query", "redis").Debugf("DEL %s", key)

	if err := SendSet(conn, width, height, 0); err != nil {
		return err
	}

	if err := conn.Flush(); err != nil {
		return err
	}

	log.WithField("query", "redis").Debugf("FLUSH")

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

	log.WithField("query", "redis").Debugf("FLUSH")

	return nil
}

// Pixel represents a given point and it's colour.
type Pixel struct {
	X      uint32
	Y      uint32
	Colour uint32
}

// PublishPixel publishes the pixel as it was created to redis by sending the
// redis command for PUBLISH, it does not flush the command.
func PublishPixel(conn redis.Conn, px Pixel) error {
	message, err := json.Marshal(px)
	if err != nil {
		return err
	}

	if err := conn.Send("PUBLISH", updatesKey, string(message)); err != nil {
		return err
	}

	return nil
}

// ParsePixel parses pixel data from strings.
func ParsePixel(xs, ys, colours string) (*Pixel, error) {
	x, err := strconv.ParseUint(xs, 10, 32)
	if err != nil {
		return nil, errors.Wrap(err, "can't parse x")
	}

	y, err := strconv.ParseUint(ys, 10, 32)
	if err != nil {
		return nil, errors.Wrap(err, "can't parse y")
	}

	colour, err := strconv.ParseUint(colours, 16, 32)
	if err != nil {
		return nil, errors.Wrap(err, "can't parse color")
	}

	return &Pixel{
		X:      uint32(x),
		Y:      uint32(y),
		Colour: uint32(colour),
	}, nil
}

// SetColour sets the colour on a pixel of the image.
func SetColour(conn redis.Conn, px Pixel) error {

	if px.X > width || px.Y > height || px.X == 0 || px.Y == 0 {
		return errors.New("invalid pixel location supplied")
	}

	if err := SendSet(conn, px.X, px.Y, px.Colour); err != nil {
		return err
	}

	if err := PublishPixel(conn, px); err != nil {
		return err
	}

	if err := conn.Flush(); err != nil {
		return err
	}

	log.WithField("query", "redis").Debugf("FLUSH")

	return nil
}
