package heximage

import (
	"net/url"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
)

// ConnectRedis connects to the redis instance, pings it, and returns the pool
// object on sucesfull connect.
func ConnectRedis(dsn string, maxActive int) (*redis.Pool, error) {

	// Parse the dsn down to a URL.
	addr, err := url.Parse(dsn)
	if err != nil {
		return nil, errors.Wrap(err, "can't parse the redis server dsn")
	}

	// Create the pool.
	pool := redis.Pool{
		MaxActive:   maxActive,
		MaxIdle:     3,
		IdleTimeout: 30 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", addr.Host)
			if err != nil {
				return nil, errors.Wrap(err, "can't dial the redis server")
			}
			if addr.User != nil {
				password, _ := addr.User.Password()
				if password != "" {
					if _, err = c.Do("AUTH", password); err != nil {
						c.Close()
						return nil, errors.Wrap(err, "can't auth to the redis server")
					}

					logrus.WithField("query", "redis").Debugf("AUTH <redacted>")
				}
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < 1*time.Minute {
				return nil
			}

			// Test that we can ping if the last time we tested this connection was
			// more than 1 minute ago.
			if _, err := c.Do("PING"); err != nil {
				return errors.Wrap(err, "can't ping redis server after timeout exhausted")
			}

			logrus.WithField("query", "redis").Debugf("PING")

			return nil
		},
	}

	// Try and get the pool connection.
	con := pool.Get()
	defer con.Close()

	// Ensure we can ping the server.
	if _, err := con.Do("PING"); err != nil {
		return nil, errors.Wrap(err, "can't ping the redis server for the first time")
	}

	logrus.WithField("query", "redis").Debugf("PING")

	return &pool, nil
}
