#!/usr/bin/env bash

# 解析命令行参数（支持传参指定 server_name / listen_port / resource / timezone / 微信参数）
while [[ $# -gt 0 ]]; do
  case "$1" in
    -s|--server_name|--server-name)
      server_name="$2"
      shift 2
      ;;
    -p|--listen_port|--listen-port|--port)
      listen_port="$2"
      shift 2
      ;;
    -r|--resource)
      resource="$2"
      shift 2
      ;;
    -t|-z|--tz|--timezone)
      timezone="$2"
      shift 2
      ;;
    -u|--user)
      user="$2"
      shift 2
      ;;
    --uid)
      host_uid="$2"
      shift 2
      ;;
    --gid)
      host_gid="$2"
      shift 2
      ;;
    --wx_app_id|--wx-app-id)
      wx_app_id="$2"
      shift 2
      ;;
    --wx_secret|--wx-secret)
      wx_secret="$2"
      shift 2
      ;;
    --wx_open_id|--wx-open-id)
      wx_open_id="$2"
      shift 2
      ;;
    --wx_template_id|--wx-template-id)
      wx_template_id="$2"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $0 [OPTIONS]"
      echo
      echo "Options:"
      echo "  -s, --server_name <name>      Server/container name (default: survive_monitor)"
      echo "  -p, --listen_port <host:port> Listen port (default: 127.0.0.1:4343)"
      echo "  -r, --resource <path|volume>  Resource mount path or volume name (default: <server_name>_resource)"
      echo "  -t, --timezone <tz>           Timezone for container (default: Asia/Shanghai or host timezone)"
      echo "  -u, --user <uid[:gid]>        User and group to run container (default: current host user/group)"
      echo "      --uid <uid>               Host user UID (default: detected from host/sudo)"
      echo "      --gid <gid>               Host user GID (default: detected from host/sudo)"
      echo "      --wx_app_id <id>          WeChat official account app id for alarm (required)"
      echo "      --wx_secret <secret>      WeChat official account secret for alarm (required)"
      echo "      --wx_open_id <ids>        Alarm receiver open ids, comma separated (optional, default: earliest subscriber)"
      echo "      --wx_template_id <id>     Alarm message template id (optional, default: looked up from account)"
      echo "  -h, --help                    Show this help message"
      exit 0
      ;;
    *)
      shift
      ;;
  esac
done

if [ -z "$server_name" ]; then
  read -p "please enter server_name(default:survive_monitor):" server_name
fi
if [ -z "$server_name" ]; then
  server_name="survive_monitor"
fi

if [ -z "$listen_port" ]; then
  read -p "please enter listen port(default:'127.0.0.1:4343'):" listen_port
fi
if [ -z "$listen_port" ]; then
  listen_port="127.0.0.1:4343"
fi

if [ -z "$resource" ]; then
  read -p "please enter resource(default:volume):" resource
fi
if [ -z "$resource" ]; then
  resource=$server_name'_resource'
fi

# 探测宿主机时区作为缺省推荐值
default_timezone="Asia/Shanghai"
if [ -f /etc/timezone ]; then
  host_tz=$(cat /etc/timezone | tr -d ' \n\r')
  [ -n "$host_tz" ] && default_timezone="$host_tz"
elif [ -h /etc/localtime ]; then
  host_tz=$(readlink /etc/localtime | sed 's#.*/zoneinfo/##')
  [ -n "$host_tz" ] && default_timezone="$host_tz"
fi

if [ -z "$timezone" ]; then
  read -p "please enter timezone(default:'$default_timezone'):" timezone
fi
if [ -z "$timezone" ]; then
  timezone="$default_timezone"
fi

# 探测宿主机用户与用户组 UID/GID 作为缺省推荐值
# 若使用 sudo 运行，优先获取唤起 sudo 的真实宿主机用户 UID/GID，避免误用 root
default_uid=""
default_gid=""
if [ -n "$SUDO_UID" ]; then
  default_uid="$SUDO_UID"
  default_gid="${SUDO_GID:-$SUDO_UID}"
else
  default_uid="$(id -u)"
  default_gid="$(id -g)"
fi
default_user="${default_uid}:${default_gid}"

if [ -z "$user" ] && [ -n "$host_uid" ]; then
  user="${host_uid}:${host_gid:-$default_gid}"
fi

if [ -z "$user" ]; then
  read -p "please enter user UID:GID(default:'$default_user'):" user
fi
if [ -z "$user" ]; then
  user="$default_user"
fi

if [[ "$user" == *:* ]]; then
  host_uid="${user%%:*}"
  host_gid="${user##*:}"
else
  host_uid="$user"
  host_gid="${host_gid:-$default_gid}"
fi
host_uid="${host_uid:-$default_uid}"
host_gid="${host_gid:-$default_gid}"
user="${host_uid}:${host_gid}"

# 告警走微信公众号，app_id与secret缺一个告警就发不出去，所以必填；
# open_id与template_id不填时由go_common自动查询（最早关注的用户、账号下的模板），这里只按传参透传
while [ -z "$wx_app_id" ]; do
  read -p "please enter wx_app_id(required):" wx_app_id
done
while [ -z "$wx_secret" ]; do
  read -p "please enter wx_secret(required):" wx_secret
done

# 若 resource 为相对路径，转换为绝对路径以便 Docker 正确识别为 bind mount
if [[ "$resource" == ./* || "$resource" == ../* ]]; then
  resource=$(cd "$resource" 2>/dev/null && pwd || echo "$PWD/$resource")
fi

echo
echo "server_name: $server_name"
echo "listen_port: $listen_port"
echo "resource:    $resource"
echo "timezone:    $timezone"
echo "user:        $user ($host_uid:$host_gid)"
echo "wx_app_id:   $wx_app_id"
echo "wx_secret:   $wx_secret"
echo "wx_open_id:  ${wx_open_id:-(auto)}"
echo "wx_template_id: ${wx_template_id:-(auto)}"
echo "input any key go on, or control+c over"
if [ -t 0 ]; then
  read
fi

echo 'create volume'
docker volume create log

# 若 resource 不是路径（无斜杠），作为 Docker volume 创建
if [[ "$resource" != /* && "$resource" != .* ]]; then
  docker volume create "$resource"
else
  # 若 resource 为宿主机目录，提前创建目录并对齐宿主机用户属主与读写权限
  mkdir -p "$resource"
  chown -R "$host_uid:$host_gid" "$resource" 2>/dev/null || true
  chmod -R u+rwX "$resource" 2>/dev/null || true
fi

echo 'stop container'
docker stop $server_name
echo 'remove container'
docker rm $server_name
echo 'remove image'
docker rmi $server_name
echo 'docker build'
docker build \
  --build-arg TZ="$timezone" \
  --build-arg UID="$host_uid" \
  --build-arg GID="$host_gid" \
  -t $server_name .

# 确保挂载卷具备写入权限（适配 Docker 命名卷或历史遗留 root 属主）
docker run --rm --user 0:0 -v log:/log $server_name chmod 777 /log 2>/dev/null || true
if [[ "$resource" != /* && "$resource" != .* ]]; then
  docker run --rm --user 0:0 -v "$resource":/resource $server_name chmod 777 /resource 2>/dev/null || true
fi

# 可选的微信参数只有填了才传，容器环境里不留空变量，一眼能看出走的是传参还是自动查询
wx_env=()
[ -n "$wx_open_id" ] && wx_env+=(-e wx_open_id="$wx_open_id")
[ -n "$wx_template_id" ] && wx_env+=(-e wx_template_id="$wx_template_id")

echo 'docker run'
docker run -d \
  --restart=always \
  --name $server_name \
  --user "$host_uid:$host_gid" \
  -v log:/log \
  -v "$resource":/resource \
  -p $listen_port:4343 \
  -e server_name=$server_name \
  -e TZ="$timezone" \
  -e wx_app_id="$wx_app_id" \
  -e wx_secret="$wx_secret" \
  "${wx_env[@]}" \
  $server_name

echo 'all finish'
