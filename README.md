# heximage

[![Go Doc](https://godoc.org/github.com/wyattjoh/heximage/lib?status.svg)](http://godoc.org/github.com/wyattjoh/heximage/lib)
[![Go Report](https://goreportcard.com/badge/github.com/wyattjoh/heximage)](https://goreportcard.com/report/github.com/wyattjoh/heximage)

This serves as a mimic of the api provided during the reddit.com/r/place
experiment. It exposes a few web api's, a websocket interface, as well as a cli
interface to editing, displaying, and fetching images stored in Redis and
modified using it's [BITFIELD](https://redis.io/commands/bitfield) command.

## Endpoints

- `GET /api/place/live`: websocket api
- `POST /api/place/draw`:  post json with content `{"X": "1", "Y": "1", "Colour": "FFFFFFFF"} to set a pixel's colour
- `GET /api/place/board-bitmap`: get the bit repr of the image in raw bytes
- `GET /api/place/board?w=`: get the png repr of the image, optional `w` allows you to set any width for the image to be resized to

## Running

You can run the application simply by running go get:

```bash
go get github.com/wyattjoh/heximage
```

And running:

```bash
$ heximage server --help
NAME:
   heximage server - serves the heximage server

USAGE:
   heximage server [command options] [arguments...]

OPTIONS:
   --listen-addr value          address for the server to listen on (default: "127.0.0.1:8080")
   --allowed-cors-origin value  allow one or many origins to access the api
```

For an example of the application in action, visit the `example` directory.

## License

MIT

