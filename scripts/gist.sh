#!/usr/bin/env bash
# Gist 单文件二进制的 Debian 启动/服务管理脚本。
#
# 所有配置均从脚本同目录的 gist.conf 读取；配置文件缺失、不可读或缺少必需项时
# 直接报错并停止执行。
#
# 用法:
#   ./gist.sh            前台启动
#   sudo ./gist.sh install    安装为 systemd 服务并启动
#   sudo ./gist.sh uninstall  停止并移除服务（保留数据）
#   sudo ./gist.sh start|stop|restart|status
#   ./gist.sh logs            查看服务日志
#
# 配置文件 gist.conf（每行一个 KEY=VALUE，不支持 export 与行内注释）:
#   GIST_BIN=/usr/local/gist/gist     二进制路径（相对路径按脚本目录解析）
#   GIST_ADDR=:9190                   监听地址
#   GIST_DATA_DIR=/usr/local/gist     数据目录
#   GIST_LOG_LEVEL=info               日志级别
#   GIST_INSTALL_DIR=/usr/local/gist  安装目录

set -euo pipefail

APP_NAME="gist"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/gist.conf"
SERVICE_FILE="/etc/systemd/system/${APP_NAME}.service"

GIST_BIN=""
GIST_ADDR=""
GIST_DATA_DIR=""
GIST_LOG_LEVEL=""
GIST_INSTALL_DIR=""
CONFIG_ENV_KEYS=()

usage() {
    cat <<'EOF'
用法:
  ./gist.sh                     前台启动
  sudo ./gist.sh install        安装为 systemd 服务并启动
  sudo ./gist.sh uninstall      停止并移除服务（保留数据）
  sudo ./gist.sh start|stop|restart|status
  ./gist.sh logs                查看服务日志

所有配置从脚本同目录的 gist.conf 读取，缺少文件或必需配置项会直接报错退出。
配置文件格式（每行一个 KEY=VALUE，不支持 export 与行内注释）:
  GIST_BIN=/usr/local/gist/gist     二进制路径（相对路径按脚本目录解析）
  GIST_ADDR=:9190                   监听地址
  GIST_DATA_DIR=/usr/local/gist     数据目录
  GIST_LOG_LEVEL=info               日志级别
  GIST_INSTALL_DIR=/usr/local/gist  安装目录
EOF
}

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        echo "错误: 该命令需要 root 权限，请使用 sudo 运行。" >&2
        exit 1
    fi
}

require_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        echo "错误: 未找到 systemctl，请确认系统使用 systemd。" >&2
        exit 1
    fi
}

trim() {
    local value="$1"
    value="${value#"${value%%[![:space:]]*}"}"
    value="${value%"${value##*[![:space:]]}"}"
    printf '%s' "$value"
}

load_config() {
    if [ ! -e "$CONFIG_FILE" ]; then
        echo "错误: 未找到配置文件 $CONFIG_FILE。" >&2
        echo "请在脚本同目录创建 gist.conf（格式见 ./gist.sh help）。" >&2
        exit 1
    fi
    if [ ! -f "$CONFIG_FILE" ] || [ ! -r "$CONFIG_FILE" ]; then
        echo "错误: 配置文件 $CONFIG_FILE 不可读，请检查文件类型与权限。" >&2
        exit 1
    fi

    local line key value lineno=0
    while IFS= read -r line || [ -n "$line" ]; do
        lineno=$((lineno + 1))
        line="$(trim "${line%$'\r'}")"
        case "$line" in
            ''|'#'*) continue ;;
        esac
        if [[ ! "$line" =~ ^(GIST_[A-Za-z0-9_]*)=(.*)$ ]]; then
            echo "错误: $CONFIG_FILE 第 $lineno 行格式不正确: $line" >&2
            echo "要求为 GIST_ 开头的 KEY=VALUE，不支持 export 与行内注释。" >&2
            exit 1
        fi
        key="${BASH_REMATCH[1]}"
        value="$(trim "${BASH_REMATCH[2]}")"
        if [ "${#value}" -ge 2 ]; then
            case "$value" in
                \"*\"|\'*\') value="${value:1:${#value}-2}" ;;
            esac
        fi
        if [ -z "$value" ]; then
            echo "错误: $CONFIG_FILE 第 $lineno 行的 $key 值为空。" >&2
            exit 1
        fi
        printf -v "$key" '%s' "$value"
        CONFIG_ENV_KEYS+=("$key")
    done < "$CONFIG_FILE"
}

require_config_keys() {
    local missing=() key
    for key in "$@"; do
        if [ -z "${!key}" ]; then
            missing+=("$key")
        fi
    done
    if [ "${#missing[@]}" -gt 0 ]; then
        echo "错误: 配置文件 $CONFIG_FILE 缺少必需配置项: ${missing[*]}" >&2
        exit 1
    fi
}

check_binary() {
    case "$GIST_BIN" in
        /*) BIN_PATH="$GIST_BIN" ;;
        *) BIN_PATH="$SCRIPT_DIR/$GIST_BIN" ;;
    esac
    if [ ! -x "$BIN_PATH" ]; then
        echo "错误: 配置项 GIST_BIN 指向的文件不可执行: $BIN_PATH" >&2
        exit 1
    fi
}

run_foreground() {
    check_binary
    mkdir -p "$GIST_DATA_DIR"
    cd "$GIST_DATA_DIR"

    local env_args=() key
    for key in "${CONFIG_ENV_KEYS[@]}"; do
        env_args+=("$key=${!key}")
    done

    echo "启动 Gist: 监听 $GIST_ADDR，数据目录 $GIST_DATA_DIR，配置 $CONFIG_FILE"
    exec env "${env_args[@]}" "$BIN_PATH"
}

install_service() {
    require_root
    require_systemd
    check_binary

    local run_user="${SUDO_USER:-$(id -un)}"
    local run_group
    run_group="$(id -gn "$run_user")"

    install -d -m 0755 "$GIST_INSTALL_DIR"
    if [ "$(readlink -f "$BIN_PATH")" != "$(readlink -f "$GIST_INSTALL_DIR/$APP_NAME")" ]; then
        install -m 0755 "$BIN_PATH" "$GIST_INSTALL_DIR/$APP_NAME"
    fi

    install -d -m 0755 -o "$run_user" -g "$run_group" "$GIST_DATA_DIR"
    chown "$run_user:$run_group" "$GIST_DATA_DIR"

    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Gist RSS Reader
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$run_user
Group=$run_group
WorkingDirectory=$GIST_DATA_DIR
EnvironmentFile=$CONFIG_FILE
ExecStart=$GIST_INSTALL_DIR/$APP_NAME
Restart=on-failure
RestartSec=5
LimitNOFILE=65535
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=$GIST_DATA_DIR

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable --now "$APP_NAME"
    systemctl status --no-pager "$APP_NAME" || true

    echo
    echo "安装完成。"
    echo "  配置:   $CONFIG_FILE（修改后执行 sudo systemctl restart $APP_NAME）"
    echo "  数据:   $GIST_DATA_DIR"
    echo "  日志:   journalctl -u $APP_NAME -f"
}

uninstall_service() {
    require_root
    systemctl disable --now "$APP_NAME" 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    echo "已移除 $APP_NAME 服务。"
    echo "安装目录 ${GIST_INSTALL_DIR:-未配置} 与数据目录 ${GIST_DATA_DIR:-未配置} 未删除。"
}

cmd="${1:-run}"

case "$cmd" in
    help|-h|--help)
        usage
        exit 0
        ;;
esac

load_config

case "$cmd" in
    run)
        require_config_keys GIST_BIN GIST_ADDR GIST_DATA_DIR GIST_LOG_LEVEL
        run_foreground
        ;;
    install)
        require_config_keys GIST_BIN GIST_ADDR GIST_DATA_DIR GIST_LOG_LEVEL GIST_INSTALL_DIR
        install_service
        ;;
    uninstall)
        uninstall_service
        ;;
    start|stop|restart)
        require_root
        require_systemd
        systemctl "$cmd" "$APP_NAME"
        ;;
    status)
        require_systemd
        systemctl status --no-pager "$APP_NAME"
        ;;
    logs)
        require_systemd
        journalctl -u "$APP_NAME" -f --no-pager
        ;;
    *)
        usage >&2
        exit 1
        ;;
esac
