package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/gotd/td/tg"
)

// runChats 列出最近会话及其 Bot API 风格 ID, 方便用户填入 chat_ids
func runChats(ctx context.Context, cfg *Config, cfgPath string) error {
	if cfg.APIID == 0 || cfg.APIHash == "" {
		return fmt.Errorf("%w: 请先设置 api_id / api_hash", errBadConfig)
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
		if !status.Authorized {
			return errNotAuthorized
		}
		res, err := client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      100,
		})
		if err != nil {
			return fmt.Errorf("获取会话列表失败: %w", err)
		}
		var users []tg.UserClass
		var chats []tg.ChatClass
		switch dl := res.(type) {
		case *tg.MessagesDialogs:
			users, chats = dl.Users, dl.Chats
		case *tg.MessagesDialogsSlice:
			users, chats = dl.Users, dl.Chats
		default:
			return errors.New("服务端返回了未知的会话列表格式")
		}

		fmt.Println("最近会话列表 (ID 可直接用于: tgsms config add-chat <ID>)")
		fmt.Println("--------------------------------------------------------------")
		fmt.Printf("%-18s %-8s %s\n", "ID", "类型", "名称")
		for _, c := range chats {
			switch cc := c.(type) {
			case *tg.Chat:
				fmt.Printf("%-18d %-8s %s\n", -cc.ID, "群组", cc.Title)
			case *tg.Channel:
				t := "频道"
				if cc.Megagroup {
					t = "超级群"
				}
				fmt.Printf("%-18d %-8s %s\n", -1000000000000-cc.ID, t, cc.Title)
			}
		}
		for _, u := range users {
			uu, ok := u.(*tg.User)
			if !ok {
				continue
			}
			name := displayName(uu)
			if uu.Username != "" {
				name += " (@" + uu.Username + ")"
			}
			switch {
			case uu.Self:
				name += " [自己]"
			case uu.Bot:
				name += " [机器人]"
			}
			fmt.Printf("%-18d %-8s %s\n", uu.ID, "用户", name)
		}
		return nil
	})
}
