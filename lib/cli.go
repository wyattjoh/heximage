package heximage

import (
	"image"
	"strconv"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
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
