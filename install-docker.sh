#!/usr/bin/env bash

if [ -z $server_name ]; then
  read -p "please enter server_name(default:survive_monitor):" server_name
fi
if [ -z $server_name ]; then
  server_name="survive_monitor"
fi

if [ -z "$listen_port" ]; then
  read -p "please enter listen port(default:'127.0.0.1:4343'):" listen_port
fi
if [ -z "$listen_port" ]; then
  listen_port="127.0.0.1:4343"
fi

if [ -z $resource ]; then
  read -p "please enter resource(default:volume):" resource
fi
if [ -z $resource ]; then
  resource=$server_name'_resource'
fi

while :; do
  if [ ! -z $wx_app_id ]; then
    break
  fi
  read -p "please enter wx_app_id(required):" wx_app_id
done

while :; do
  if [ ! -z $wx_secret ]; then
    break
  fi
  read -p "please enter wx_secret(required):" wx_secret
done

while :; do
  if [ ! -z $wx_user ]; then
    break
  fi
  read -p "please enter wx_user(required):" wx_user
done

echo
echo "server_name: $server_name"
echo "listen_port: $listen_port"
echo "wx_app_id: $wx_app_id"
echo "wx_secret: $wx_secret"
echo "wx_user: $wx_user"
echo "input any key go on, or control+c over"
read

echo 'create volume'
docker volume create log
echo 'stop container'
docker stop $server_name
echo 'remove container'
docker rm $server_name
echo 'remove image'
docker rmi $server_name
echo 'docker build'
docker build -t $server_name .
echo 'docker run'
docker run -d \
  --restart=always \
  --name $server_name \
  -v log:/log \
  -v $resource:/resource \
  -p $listen_port:4343 \
  -e server_name=$server_name \
  -e wx_app_id=$wx_app_id \
  -e wx_secret=$wx_secret \
  -e wx_user=$wx_user \
  $server_name

echo 'all finish'
