#!/bin/bash
# tgsms 管理脚本 (安装后位于 /usr/bin/tgsms)
# 直接输入 tgsms 打开数字菜单, 也支持子命令: tgsms start|stop|log|login|config ...

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
blue='\033[0;36m'
plain='\033[0m'

BIN="/usr/local/tgsms/tgsms"
CONFIG_DIR="/etc/tgsms"
CONFIG="${CONFIG_DIR}/config.yaml"
SERVICE="tgsms"
REPO="kuosfls/tm"
BRANCH="master"

err()  { echo -e "${red}$*${plain}"; }
ok()   { echo -e "${green}$*${plain}"; }
warn() { echo -e "${yellow}$*${plain}"; }

if [[ $EUID -ne 0 ]]; then
    err "错误: 请使用 root 用户运行 tgsms"
    exit 1
fi

if [[ ! -x "$BIN" ]]; then
    err "未找到主程序 $BIN, 请重新运行安装脚本:"
    echo "bash <(curl -fsSL https://raw.githubusercontent.com/${REPO}/${BRANCH}/install.sh)"
    exit 1
fi

svc_active()  { systemctl is-active "$SERVICE" &>/dev/null; }
svc_enabled() { systemctl is-enabled "$SERVICE" &>/dev/null; }

confirm() {
    local ans
    read -rp "$1 [y/N]: " ans
    [[ "$ans" == "y" || "$ans" == "Y" ]]
}

do_start() {
    if svc_active; then
        warn "tgsms 已在运行"
        return
    fi
    systemctl start "$SERVICE"
    sleep 1
    if svc_active; then
        ok "tgsms 启动成功"
    else
        err "启动失败, 请查看日志: tgsms log (常见原因: 未登录或配置不完整)"
    fi
}

do_stop() {
    systemctl stop "$SERVICE"
    ok "tgsms 已停止"
}

do_restart() {
    systemctl restart "$SERVICE"
    sleep 1
    if svc_active; then
        ok "tgsms 重启成功"
    else
        err "重启失败, 请查看日志: tgsms log"
    fi
}

do_status() {
    systemctl status "$SERVICE" --no-pager -l
}

do_log() {
    warn "正在实时查看日志, 按 Ctrl+C 退出"
    journalctl -u "$SERVICE" -n 100 -f
}

# 登录/查会话都会独占会话文件, 需要先暂停服务
do_login() {
    local was_active=0
    if svc_active; then
        was_active=1
        warn "登录期间将暂停 tgsms 服务..."
        systemctl stop "$SERVICE"
    fi
    "$BIN" login -c "$CONFIG"
    if [[ $was_active -eq 1 ]]; then
        systemctl start "$SERVICE" && ok "tgsms 服务已恢复运行"
    elif confirm "是否立即启动 tgsms 服务?"; then
        do_start
    fi
}

do_chats() {
    local was_active=0
    if svc_active; then
        was_active=1
        warn "查询期间将暂停 tgsms 服务..."
        systemctl stop "$SERVICE"
    fi
    "$BIN" chats -c "$CONFIG"
    if [[ $was_active -eq 1 ]]; then
        systemctl start "$SERVICE" && ok "tgsms 服务已恢复运行"
    fi
}

do_test() {
    "$BIN" test -c "$CONFIG"
}

restart_if_running() {
    if svc_active && confirm "配置已修改, 是否立即重启 tgsms 生效?"; then
        do_restart
    fi
}

config_menu() {
    while true; do
        echo ""
        echo -e "${blue}—— 配置管理 ——${plain}"
        echo "  1. 查看当前配置"
        echo "  2. 设置 api_id / api_hash"
        echo "  3. 设置手机号"
        echo "  4. 设置 webhook 推送地址"
        echo "  5. 设置 webhook 密钥 (可选)"
        echo "  6. 添加监听会话 ID"
        echo "  7. 删除监听会话 ID"
        echo "  8. 设置 socks5 代理 (可选)"
        echo "  9. 设置是否转发自己发出的消息"
        echo "  0. 返回主菜单"
        local c v v1 v2
        read -rp "请选择 [0-9]: " c
        case "$c" in
            1) "$BIN" config show -c "$CONFIG" ;;
            2)
                echo "api_id / api_hash 在 https://my.telegram.org -> API development tools 申请"
                read -rp "请输入 api_id: " v1
                read -rp "请输入 api_hash: " v2
                "$BIN" config set -c "$CONFIG" api_id "$v1" && \
                "$BIN" config set -c "$CONFIG" api_hash "$v2"
                ;;
            3)
                read -rp "请输入手机号 (含国际区号, 如 +8613800138000): " v
                "$BIN" config set -c "$CONFIG" phone "$v"
                ;;
            4)
                read -rp "请输入 webhook 推送地址 (如 https://example.com/hook): " v
                "$BIN" config set -c "$CONFIG" webhook_url "$v"
                restart_if_running
                ;;
            5)
                read -rp "请输入 webhook 密钥 (将放在请求头 X-TGSMS-Secret): " v
                "$BIN" config set -c "$CONFIG" webhook_secret "$v"
                restart_if_running
                ;;
            6)
                echo "提示: 用户为正数, 普通群为负数, 频道/超级群以 -100 开头 (可用主菜单 [7] 查看)"
                read -rp "请输入要监听的会话 ID: " v
                "$BIN" config add-chat -c "$CONFIG" "$v"
                restart_if_running
                ;;
            7)
                read -rp "请输入要删除的会话 ID: " v
                "$BIN" config del-chat -c "$CONFIG" "$v"
                restart_if_running
                ;;
            8)
                read -rp "请输入代理地址 (如 socks5://127.0.0.1:1080, 输入空格清除): " v
                "$BIN" config set -c "$CONFIG" proxy "$(echo "$v" | tr -d ' ')"
                restart_if_running
                ;;
            9)
                read -rp "是否转发自己发出的消息? (true/false): " v
                "$BIN" config set -c "$CONFIG" include_outgoing "$v"
                restart_if_running
                ;;
            0) break ;;
            *) err "无效选择" ;;
        esac
    done
}

do_enable()  { systemctl enable "$SERVICE" &>/dev/null && ok "已设置开机自启"; }
do_disable() { systemctl disable "$SERVICE" &>/dev/null && ok "已取消开机自启"; }

do_update() {
    if confirm "将从 GitHub 下载最新版本并覆盖安装 (配置和登录会话会保留), 是否继续?"; then
        bash <(curl -fsSL "https://raw.githubusercontent.com/${REPO}/${BRANCH}/install.sh") || err "更新失败"
    fi
}

do_uninstall() {
    confirm "确定要卸载 tgsms 吗?" || return
    systemctl stop "$SERVICE" 2>/dev/null
    systemctl disable "$SERVICE" 2>/dev/null
    rm -f /etc/systemd/system/tgsms.service
    systemctl daemon-reload
    rm -rf /usr/local/tgsms
    rm -f /usr/bin/tgsms
    if confirm "是否同时删除配置和登录会话 (${CONFIG_DIR})?"; then
        rm -rf "$CONFIG_DIR"
    fi
    ok "tgsms 已卸载"
    exit 0
}

status_line() {
    local st en ver
    if svc_active; then st="${green}运行中${plain}"; else st="${red}已停止${plain}"; fi
    if svc_enabled; then en="${green}是${plain}"; else en="${red}否${plain}"; fi
    ver=$("$BIN" version 2>/dev/null | awk '{print $2}')
    echo -e "服务状态: $st    开机自启: $en    版本: ${ver:-未知}"
}

show_menu() {
    echo -e "
${green}tgsms 管理脚本${plain} ${blue}(Telegram 消息转发到 Webhook)${plain}
——————————————————————————————
  ${green}0.${plain} 退出脚本
——————————————————————————————
  ${green}1.${plain} 查看服务状态
  ${green}2.${plain} 启动 tgsms
  ${green}3.${plain} 停止 tgsms
  ${green}4.${plain} 重启 tgsms
  ${green}5.${plain} 查看实时日志
——————————————————————————————
  ${green}6.${plain} 登录 Telegram 账号
  ${green}7.${plain} 查看最近会话 ID
  ${green}8.${plain} 配置管理 (api / webhook / 监听ID / 代理)
  ${green}9.${plain} 测试 webhook 推送
——————————————————————————————
 ${green}10.${plain} 设置开机自启
 ${green}11.${plain} 取消开机自启
 ${green}12.${plain} 更新 tgsms
 ${green}13.${plain} 卸载 tgsms
——————————————————————————————"
    status_line
    echo ""
    local num
    read -rp "请输入选择 [0-13]: " num
    case "$num" in
        0) exit 0 ;;
        1) do_status ;;
        2) do_start ;;
        3) do_stop ;;
        4) do_restart ;;
        5) do_log ;;
        6) do_login ;;
        7) do_chats ;;
        8) config_menu ;;
        9) do_test ;;
        10) do_enable ;;
        11) do_disable ;;
        12) do_update ;;
        13) do_uninstall ;;
        *) err "请输入 0-13 之间的数字" ;;
    esac
}

main_menu_loop() {
    while true; do
        show_menu
        echo ""
        read -rp "按回车返回主菜单..." _
    done
}

case "$1" in
    "" | menu) main_menu_loop ;;
    start)     do_start ;;
    stop)      do_stop ;;
    restart)   do_restart ;;
    status)    do_status ;;
    log)       do_log ;;
    login)     do_login ;;
    logout)    "$BIN" logout -c "$CONFIG" ;;
    chats)     do_chats ;;
    test)      do_test ;;
    enable)    do_enable ;;
    disable)   do_disable ;;
    update)    do_update ;;
    uninstall) do_uninstall ;;
    version)   "$BIN" version ;;
    config)
        shift
        if [[ $# -eq 0 ]]; then
            config_menu
        else
            sub="$1"
            shift
            "$BIN" config "$sub" -c "$CONFIG" "$@"
        fi
        ;;
    *)
        echo "用法: tgsms [menu|start|stop|restart|status|log|login|logout|chats|config|test|enable|disable|update|uninstall|version]"
        echo "不带参数直接输入 tgsms 打开管理菜单"
        exit 1
        ;;
esac
