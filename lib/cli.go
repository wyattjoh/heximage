package heximage

import (
	"strconv"

	"github.com/Sirupsen/logrus"
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

	logrus.WithField("query", "redis").Debugf("FLUSH")

	return nil
}
