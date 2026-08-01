package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// version 由构建时 -ldflags "-X main.version=vX.Y.Z" 注入
var version = "dev"

var (
	errNotAuthorized = errors.New("尚未登录 Telegram 账号, 请先运行: tgsms login")
	errBadConfig     = errors.New("配置不完整")
)

func main() {
	log.SetFlags(log.LstdFlags)
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}
	cmd, rest := args[0], args[1:]

	var err error
	switch cmd {
	case "run":
		err = withConfig(rest, runService)
	case "login":
		err = withConfig(rest, runLogin)
	case "logout":
		err = withConfig(rest, runLogout)
	case "chats":
		err = withConfig(rest, runChats)
	case "test":
		err = withConfig(rest, runTest)
	case "config":
		err = configCmd(rest)
	case "version", "-v", "--version":
		fmt.Println("tgsms", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintln(os.Stderr, "未知命令:", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		// 配置类错误返回 2, systemd 配置了 RestartPreventExitStatus=2,
		// 避免"未登录/未配置"时无意义地反复重启
		if errors.Is(err, errNotAuthorized) || errors.Is(err, errBadConfig) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

type cmdFunc func(ctx context.Context, cfg *Config, cfgPath string) error

func withConfig(args []string, fn cmdFunc) error {
	cfgPath, _ := parseConfigFlag(args)
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: 配置文件不存在 (%s), 请先运行 tgsms config init", errBadConfig, cfgPath)
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return fn(ctx, cfg, cfgPath)
}

// parseConfigFlag 从参数中解析 -c <path>, 返回配置路径和剩余位置参数。
// 注意: -c 需要位于位置参数之前, 如 tgsms config set -c /path key value
func parseConfigFlag(args []string) (string, []string) {
	fs := flag.NewFlagSet("tgsms", flag.ExitOnError)
	c := fs.String("c", defaultConfigPath(), "配置文件路径")
	_ = fs.Parse(args)
	return *c, fs.Args()
}

func runLogout(_ context.Context, _ *Config, cfgPath string) error {
	p := sessionPathFor(cfgPath)
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("当前没有已保存的登录会话")
			return nil
		}
		return err
	}
	fmt.Println("已退出登录, 会话文件已删除:", p)
	return nil
}

func configCmd(args []string) error {
	if len(args) == 0 {
		configUsage()
		return nil
	}
	sub := args[0]
	cfgPath, rest := parseConfigFlag(args[1:])
	switch sub {
	case "init":
		return configInit(cfgPath)
	case "show":
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return fmt.Errorf("读取配置失败 (%s), 请先运行 tgsms config init", cfgPath)
		}
		fmt.Println("# 配置文件:", cfgPath)
		fmt.Println(strings.TrimRight(string(data), "\n"))
		return nil
	case "set":
		if len(rest) == 1 && strings.Contains(rest[0], "=") {
			kv := strings.SplitN(rest[0], "=", 2)
			rest = []string{kv[0], kv[1]}
		}
		if len(rest) != 2 {
			return errors.New("用法: tgsms config set <key> <value>")
		}
		return configSet(cfgPath, rest[0], rest[1])
	case "add-chat", "del-chat":
		if len(rest) != 1 {
			return fmt.Errorf("用法: tgsms config %s <会话ID>", sub)
		}
		id, err := strconv.ParseInt(rest[0], 10, 64)
		if err != nil {
			return fmt.Errorf("无效的会话ID: %s (必须是整数, 频道/超级群以 -100 开头)", rest[0])
		}
		return configChat(cfgPath, id, sub == "add-chat")
	default:
		configUsage()
		return fmt.Errorf("未知 config 子命令: %s", sub)
	}
}

func configSet(path, key, value string) error {
	cfg, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	switch key {
	case "api_id":
		v, err := strconv.Atoi(value)
		if err != nil {
			return errors.New("api_id 必须是数字")
		}
		cfg.APIID = v
	case "api_hash":
		cfg.APIHash = value
	case "phone":
		cfg.Phone = value
	case "webhook_url":
		cfg.WebhookURL = value
	case "webhook_secret":
		cfg.WebhookSecret = value
	case "proxy":
		cfg.Proxy = value
	case "include_outgoing":
		v, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New("include_outgoing 取值为 true 或 false")
		}
		cfg.IncludeOutgoing = v
	default:
		return fmt.Errorf("不支持的配置项: %s (可用: api_id, api_hash, phone, webhook_url, webhook_secret, proxy, include_outgoing)", key)
	}
	if err := saveConfig(path, cfg); err != nil {
		return err
	}
	fmt.Printf("已设置 %s = %s\n", key, value)
	return nil
}

func configChat(path string, id int64, add bool) error {
	cfg, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	if add {
		for _, v := range cfg.ChatIDs {
			if v == id {
				fmt.Println("该会话ID已在监听列表中:", id)
				return nil
			}
		}
		cfg.ChatIDs = append(cfg.ChatIDs, id)
		if err := saveConfig(path, cfg); err != nil {
			return err
		}
		fmt.Printf("已添加监听会话: %d (当前共 %d 个)\n", id, len(cfg.ChatIDs))
		return nil
	}
	kept := cfg.ChatIDs[:0]
	found := false
	for _, v := range cfg.ChatIDs {
		if v == id {
			found = true
			continue
		}
		kept = append(kept, v)
	}
	if !found {
		fmt.Println("监听列表中没有该会话ID:", id)
		return nil
	}
	cfg.ChatIDs = kept
	if err := saveConfig(path, cfg); err != nil {
		return err
	}
	fmt.Printf("已删除监听会话: %d (当前共 %d 个)\n", id, len(cfg.ChatIDs))
	return nil
}

func usage() {
	fmt.Printf(`tgsms %s - 监听 Telegram 指定会话消息并转发到 Webhook (基于 gotd/td)

用法: tgsms <命令> [-c 配置文件]

命令:
  run        前台运行转发服务 (systemd 调用)
  login      登录 Telegram 账号 (交互式, 需先配置 api_id/api_hash)
  logout     退出登录并删除本地会话
  chats      列出最近会话及可直接使用的会话 ID
  config     配置管理, 见 tgsms config
  test       向 webhook_url 发送一条测试消息
  version    显示版本

默认配置文件: %s (可用 -c 指定)
`, version, defaultConfigPath())
}

func configUsage() {
	fmt.Print(`用法: tgsms config <子命令> [-c 配置文件] [参数]

子命令:
  init                     生成默认配置文件
  show                     查看当前配置
  set <key> <value>        设置配置项
  add-chat <会话ID>        添加监听会话
  del-chat <会话ID>        删除监听会话

可设置的 key:
  api_id, api_hash, phone, webhook_url, webhook_secret, proxy, include_outgoing
`)
}
