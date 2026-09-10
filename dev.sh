#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
mkdir -p videos bin
go build -o bin/api ./api
go build -o bin/scheduler ./scheduler
go build -o bin/streamsever ./streamsever

./bin/api &
P1=$!
./bin/scheduler &
P2=$!
./bin/streamsever &
P3=$!
trap 'kill $P1 $P2 $P3 2>/dev/null' INT TERM
wait