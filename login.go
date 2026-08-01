package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

var stdin = bufio.NewReader(os.Stdin)

func prompt(label string) (string, error) {
	fmt.Print(label)
	line, err := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

// termAuth 实现 gotd 的 auth.UserAuthenticator, 通过终端交互完成登录
type termAuth struct {
	phone string
}

func (a termAuth) Phone(_ context.Context) (string, error) {
	if a.phone != "" {
		fmt.Println("使用配置中的手机号:", a.phone)
		return a.phone, nil
	}
	return prompt("请输入手机号 (含国际区号, 如 +8613800138000): ")
}

func (a termAuth) Password(_ context.Context) (string, error) {
	fmt.Print("请输入两步验证密码 (2FA): ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := stdin.ReadString('\n')
	line = strings.TrimSpace(line)
	if err != nil && line == "" {
		return "", err
	}
	return line, nil
}

func (a termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return prompt("请输入 Telegram 发来的验证码: ")
}

func (a termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("该手机号尚未注册 Telegram 账号")
}

func (a termAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func runLogin(ctx context.Context, cfg *Config, cfgPath string) error {
	if cfg.APIID == 0 || cfg.APIHash == "" {
		return fmt.Errorf("%w: 请先设置 api_id / api_hash (my.telegram.org 申请)", errBadConfig)
	}
	client, err := buildClient(cfg, sessionPathFor(cfgPath), nil)
	if err != nil {
		return err
	}
	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized {
			self, err := client.Self(ctx)
			if err == nil {
				fmt.Printf("当前已登录: %s (id=%d)\n", displayName(self), self.ID)
				fmt.Println("如需更换账号, 请先执行 tgsms logout 再重新登录")
			}
			return nil
		}
		flow := auth.NewFlow(termAuth{phone: cfg.Phone}, auth.SendCodeOptions{})
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return fmt.Errorf("登录失败: %w", err)
		}
		self, err := client.Self(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("登录成功: %s (id=%d)\n", displayName(self), self.ID)
		return nil
	})
}
