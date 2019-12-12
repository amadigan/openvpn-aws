#! /bin/sh
docker build -t generator .
docker volume create server
docker volume create zeus
docker volume create hercules
docker volume create plato
docker run -it --rm -v server:/mnt/server -v zeus:/mnt/zeus -v hercules:/mnt/hercules -v plato:/mnt/plato generator
