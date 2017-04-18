package main

import (
	"image/png"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	heximage "github.com/wyattjoh/heximage/lib"
)

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
		pool, err = heximage.ConnectRedis(c.GlobalString("redis-url"), c.GlobalInt("redis-max-clients"))
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
					Value: "127.0.0.1:8080",
				},
				cli.StringSliceFlag{
					Name:  "allowed-cors-origin",
					Usage: "allow one or many origins to access the api",
				},
			},
			Action: func(c *cli.Context) error {
				if err := heximage.StartServer(c.String("listen-addr"), pool, conn, c.StringSlice("allowed-cors-origin")); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't run the server"), 1)
				}

				return nil
			},
		},
		{
			Name:  "init",
			Usage: "initializes the image canvas",
			Action: func(c *cli.Context) error {
				if err := heximage.InitImage(conn); err != nil {
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
				if err := heximage.SetColour(conn, args.Get(0), args.Get(1), args.Get(2)); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't set the colour"), 1)
				}

				return nil
			},
		},
		{
			Name:  "get",
			Usage: "gets the image and return's an encoded png to the stdout",
			Action: func(c *cli.Context) error {
				img, err := heximage.GetImage(conn)
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
				if err := heximage.ClearImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't clear the image"), 1)
				}

				if err := heximage.InitImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't init the image"), 1)
				}

				return nil
			},
		},
		{
			Name:  "test",
			Usage: "prints a test pattern onto the image",
			Action: func(c *cli.Context) error {
				if err := heximage.TestImage(conn); err != nil {
					return cli.NewExitError(errors.Wrap(err, "can't set the test pattern"), 1)
				}

				return nil
			},
		},
	}

	app.Run(os.Args)
}
