# heximage

This serves as a mimic of the api provided during the reddit.com/r/place
experiment.

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

