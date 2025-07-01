#!/bin/bash

# Konfiguration
IMAGE_NAME="mkihr1/sts-restart-watcher"
TAG="amd64"

echo "🔧 Baue Docker-Image: $IMAGE_NAME:$TAG"
docker build -t $IMAGE_NAME:$TAG .

#echo "🔑 Docker Login (du wirst ggf. nach Benutzer/Token gefragt)"
#docker login

echo "🚀 Pushe Image nach Docker Hub"
docker push $IMAGE_NAME:$TAG

echo "✅ Fertig: $IMAGE_NAME:$TAG wurde erfolgreich veröffentlicht"
