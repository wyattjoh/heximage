package main

import (
	"net/url"
	"time"

	"github.com/garyburd/redigo/redis"
)

// ConnectRedis connects to the redis instance, pings it, and returns the pool
// object on sucesfull connect.
func ConnectRedis(dsn string, maxActive int) (*redis.Pool, error) {

	// Parse the dsn down to a URL.
	addr, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}

	// Create the pool.
	pool := redis.Pool{
		MaxActive:   maxActive,
		MaxIdle:     3,
		IdleTimeout: 30 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", addr.Host)
			if err != nil {
				return nil, err
			}
			if addr.User != nil {
				password, _ := addr.User.Password()
				if password != "" {
					if _, err = c.Do("AUTH", password); err != nil {
						c.Close()
						return nil, err
					}
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
			_, err := c.Do("PING")
			return err
		},
	}

	// Try and get the pool connection.
	con := pool.Get()
	defer con.Close()

	// Ensure we can ping the server.
	if _, err := con.Do("PING"); err != nil {
		return nil, err
	}

	return &pool, nil
}
