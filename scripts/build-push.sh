#!/bin/bash

# Login to Docker Hub
docker login

# Build and create locally first
make clean build create

# Test it works
docker plugin enable ashutoshpw/axiom-docker-logger:latest

# Push to Docker Hub
make push
