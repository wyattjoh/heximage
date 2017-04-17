#!/bin/bash

go install ./...

# heximage clear
# heximage init
heximage set 1 1 255
heximage set 2 1 0
heximage set 3 1 0
heximage set 4 1 255
heximage set 5 1 0
heximage set 6 1 255
heximage set 7 1 0
heximage set 8 1 255
heximage get > image.png