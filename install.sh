#!/bin/bash
# tgsms 一键安装/更新脚本
# 用法: bash <(curl -fsSL https://raw.githubusercontent.com/kuosfls/tm/master/install.sh)
# 安装指定版本: bash <(curl -fsSL ...) v1.0.0

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

err()  { echo -e "${red}$*${plain}"; }
ok()   { echo -e "${green}$*${plain}"; }
warn() { echo -e "${yellow}$*${plain}"; }

REPO="kuosfls/tm"
INSTALL_DIR="/usr/local/tgsms"
CONFIG_DIR="/etc/tgsms"

if [[ $EUID -ne 0 ]]; then
    err "错误: 请使用 root 用户运行安装脚本"
    exit 1
fi

if ! command -v systemctl &>/dev/null; then
    err "错误: 未检测到 systemd, 目前仅支持 Debian/Ubuntu/CentOS 等使用 systemd 的 Linux 系统"
    exit 1
fi

case "$(uname -m)" in
    x86_64 | amd64)  ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *)
        err "错误: 不支持的 CPU 架构 $(uname -m) (仅支持 amd64 / arm64)"
        exit 1
        ;;
esac

install_pkg() {
    if command -v apt-get &>/dev/null; then
        apt-get update -y >/dev/null 2>&1
        apt-get install -y "$@" >/dev/null 2>&1
    elif command -v dnf &>/dev/null; then
        dnf install -y "$@" >/dev/null 2>&1
    elif command -v yum &>/dev/null; then
        yum install -y "$@" >/dev/null 2>&1
    fi
}

command -v curl &>/dev/null || install_pkg curl
command -v tar  &>/dev/null || install_pkg tar
if ! command -v curl &>/dev/null || ! command -v tar &>/dev/null; then
    err "错误: 缺少 curl 或 tar, 请手动安装后重试"
    exit 1
fi

TAG="${1:-latest}"
if [[ "$TAG" == "latest" ]]; then
    URL="https://github.com/${REPO}/releases/latest/download/tgsms-linux-${ARCH}.tar.gz"
else
    URL="https://github.com/${REPO}/releases/download/${TAG}/tgsms-linux-${ARCH}.tar.gz"
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "正在下载: $URL"
if ! curl -fL --connect-timeout 15 --retry 2 -o "$TMP/tgsms.tar.gz" "$URL"; then
    err "下载失败: 请检查网络是否能访问 GitHub, 或确认仓库 ${REPO} 已发布 Release"
    exit 1
fi

WAS_ACTIVE=0
if systemctl is-active tgsms &>/dev/null; then
    WAS_ACTIVE=1
    systemctl stop tgsms
fi

mkdir -p "$INSTALL_DIR"
tar -xzf "$TMP/tgsms.tar.gz" -C "$INSTALL_DIR"
chmod +x "$INSTALL_DIR/tgsms" "$INSTALL_DIR/tgsms.sh"

# 管理脚本占用 tgsms 命令, 主程序放在 /usr/local/tgsms/tgsms
install -m 755 "$INSTALL_DIR/tgsms.sh" /usr/bin/tgsms
install -m 644 "$INSTALL_DIR/tgsms.service" /etc/systemd/system/tgsms.service

mkdir -p "$CONFIG_DIR"
"$INSTALL_DIR/tgsms" config init -c "$CONFIG_DIR/config.yaml" >/dev/null

systemctl daemon-reload
systemctl enable tgsms &>/dev/null

VER="$("$INSTALL_DIR/tgsms" version 2>/dev/null | awk '{print $2}')"

if [[ $WAS_ACTIVE -eq 1 ]]; then
    systemctl start tgsms
    ok "tgsms 已更新到 ${VER:-未知版本} 并恢复运行"
else
    ok "tgsms ${VER:-} 安装完成!"
    echo ""
    echo -e "${green}使用步骤:${plain}"
    echo -e "  1. 打开 https://my.telegram.org -> API development tools, 申请 api_id 和 api_hash"
    echo -e "  2. 输入 ${green}tgsms${plain} 打开管理菜单 -> ${green}[8] 配置管理${plain}, 填入 api_id / api_hash / webhook 地址"
    echo -e "  3. 菜单 ${green}[6]${plain} 登录 Telegram 账号"
    echo -e "  4. 菜单 ${green}[7]${plain} 查看会话 ID, 再到 ${green}[8]${plain} 添加要监听的会话"
    echo -e "  5. 菜单 ${green}[2]${plain} 启动服务"
    echo ""
    echo -e "随时输入 ${green}tgsms${plain} 打开管理菜单"
fi
