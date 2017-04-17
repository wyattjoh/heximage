#!/bin/bash

set -e

go install ./...

heximage clear
heximage init
heximage test

for x in $(seq 1 10)
do
  for y in $(seq 1 10)
  do
    heximage set $x $y FF0000FF
  done
done

heximage get > image.png