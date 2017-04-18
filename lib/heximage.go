package heximage

import (
	"image"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
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

	logrus.WithField("query", "redis").Debugf("BITFIELD %s SET u32 %d %d", key, offset, colour)
	return conn.Send("BITFIELD", key, "SET", "u32", offset, colour)
}

// GetImage returns an image.
func GetImage(conn redis.Conn) (image.Image, error) {
	data, err := redis.Bytes(conn.Do("GET", key))
	if err != nil {
		return nil, errors.Wrap(err, "can't get the image bytes")
	}

	logrus.WithField("query", "redis").Debugf("GET %s", key)

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

	logrus.WithField("query", "redis").Debugf("DEL %s", key)

	return nil
}

// InitImage initializes the image.
func InitImage(conn redis.Conn) error {
	if err := conn.Send("DEL", key); err != nil {
		return errors.Wrap(err, "can't execute the del command")
	}

	logrus.WithField("query", "redis").Debugf("DEL %s", key)

	if err := SendSet(conn, width, height, 0); err != nil {
		return err
	}

	if err := conn.Flush(); err != nil {
		return err
	}

	logrus.WithField("query", "redis").Debugf("FLUSH")

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

	logrus.WithField("query", "redis").Debugf("FLUSH")

	return nil
}
