package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"golang.org/x/net/proxy"
)

func buildClient(cfg *Config, sessionPath string, handler telegram.UpdateHandler) (*telegram.Client, error) {
	opts := telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionPath},
		UpdateHandler:  handler,
	}
	if cfg.Proxy != "" {
		d, err := proxyDialer(cfg.Proxy)
		if err != nil {
			return nil, err
		}
		opts.Resolver = dcs.Plain(dcs.PlainOptions{Dial: d.DialContext})
	}
	return telegram.NewClient(cfg.APIID, cfg.APIHash, opts), nil
}

func proxyDialer(raw string) (proxy.ContextDialer, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("代理地址无效: %w", err)
	}
	if u.Scheme != "socks5" && u.Scheme != "socks5h" {
		return nil, errors.New("仅支持 socks5 代理, 格式如 socks5://127.0.0.1:1080")
	}
	var auth *proxy.Auth
	if u.User != nil {
		pw, _ := u.User.Password()
		auth = &proxy.Auth{User: u.User.Username(), Password: pw}
	}
	d, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}
	cd, ok := d.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("代理初始化失败")
	}
	return cd, nil
}

type service struct {
	cfg     *Config
	webhook *Webhook
	watched map[int64]bool
	selfID  int64
}

func runService(ctx context.Context, cfg *Config, cfgPath string) error {
	if err := cfg.validateForRun(); err != nil {
		return err
	}
	if len(cfg.ChatIDs) == 0 {
		log.Println("警告: chat_ids 为空, 不会转发任何消息 (用 tgsms config add-chat <ID> 添加)")
	}

	svc := &service{
		cfg:     cfg,
		webhook: NewWebhook(cfg.WebhookURL, cfg.WebhookSecret),
		watched: make(map[int64]bool, len(cfg.ChatIDs)),
	}
	for _, id := range cfg.ChatIDs {
		svc.watched[id] = true
	}

	dispatcher := tg.NewUpdateDispatcher()
	gaps := updates.New(updates.Config{Handler: dispatcher})
	dispatcher.OnNewMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		svc.handle(e, u.Message)
		return nil
	})
	dispatcher.OnNewChannelMessage(func(_ context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		svc.handle(e, u.Message)
		return nil
	})

	client, err := buildClient(cfg, sessionPathFor(cfgPath), gaps)
	if err != nil {
		return err
	}

	svc.webhook.Start(ctx)
	log.Printf("tgsms %s 启动, 监听 %d 个会话, webhook: %s", version, len(cfg.ChatIDs), cfg.WebhookURL)

	// 断线自动重连, 指数退避
	backoff := 5 * time.Second
	for {
		err := client.Run(ctx, func(ctx context.Context) error {
			status, err := client.Auth().Status(ctx)
			if err != nil {
				return err
			}
			if !status.Authorized {
				return errNotAuthorized
			}
			self, err := client.Self(ctx)
			if err != nil {
				return err
			}
			svc.selfID = self.ID
			log.Printf("已登录: %s (id=%d)", displayName(self), self.ID)
			backoff = 5 * time.Second
			return gaps.Run(ctx, client.API(), self.ID, updates.AuthOptions{
				OnStart: func(_ context.Context) {
					log.Println("消息监听已就绪")
				},
			})
		})
		if ctx.Err() != nil {
			log.Println("收到退出信号, 服务停止")
			return nil
		}
		if errors.Is(err, errNotAuthorized) {
			return err
		}
		log.Printf("连接中断: %v, %v 后重连", err, backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < time.Minute {
			backoff *= 2
		}
	}
}

func (s *service) handle(e tg.Entities, mc tg.MessageClass) {
	msg, ok := mc.(*tg.Message)
	if !ok {
		return // 忽略服务消息 (入群/置顶等)
	}
	if msg.Out && !s.cfg.IncludeOutgoing {
		return
	}
	chatID, chatType, chatTitle := peerInfo(e, msg.PeerID)
	if !s.watched[chatID] {
		return
	}
	senderID, senderName, senderUsername := s.senderInfo(e, msg)
	s.webhook.Enqueue(WebhookPayload{
		Event:          "message",
		MessageID:      msg.ID,
		ChatID:         chatID,
		ChatType:       chatType,
		ChatTitle:      chatTitle,
		SenderID:       senderID,
		SenderName:     senderName,
		SenderUsername: senderUsername,
		Text:           msg.Message,
		Media:          mediaType(msg),
		Date:           int64(msg.Date),
		ReceivedAt:     time.Now().Unix(),
	})
	log.Printf("[消息] %s(%d) %s: %s", chatTitle, chatID, senderName, truncate(msg.Message, 80))
}

// peerInfo 把 MTProto 的 Peer 转成 Bot API 风格的会话 ID:
// 用户为正数, 普通群为负数, 频道/超级群为 -100 前缀
func peerInfo(e tg.Entities, p tg.PeerClass) (int64, string, string) {
	switch pp := p.(type) {
	case *tg.PeerUser:
		title := ""
		if u, ok := e.Users[pp.UserID]; ok {
			title = displayName(u)
		}
		return pp.UserID, "user", title
	case *tg.PeerChat:
		title := ""
		if c, ok := e.Chats[pp.ChatID]; ok {
			title = c.Title
		}
		return -pp.ChatID, "group", title
	case *tg.PeerChannel:
		title := ""
		chatType := "channel"
		if c, ok := e.Channels[pp.ChannelID]; ok {
			title = c.Title
			if c.Megagroup {
				chatType = "group"
			}
		}
		return -1000000000000 - pp.ChannelID, chatType, title
	}
	return 0, "unknown", ""
}

func (s *service) senderInfo(e tg.Entities, msg *tg.Message) (int64, string, string) {
	var uid int64
	if from, ok := msg.GetFromID(); ok {
		switch f := from.(type) {
		case *tg.PeerUser:
			uid = f.UserID
		case *tg.PeerChannel:
			// 频道身份发言 (频道帖子 / 匿名管理员)
			id := int64(-1000000000000) - f.ChannelID
			if c, ok := e.Channels[f.ChannelID]; ok {
				return id, c.Title, c.Username
			}
			return id, "", ""
		case *tg.PeerChat:
			return -f.ChatID, "", ""
		}
	} else if p, ok := msg.PeerID.(*tg.PeerUser); ok {
		// 私聊消息可能不带 FromID: 入站为对方, 出站为自己
		if msg.Out {
			uid = s.selfID
		} else {
			uid = p.UserID
		}
	}
	if uid == 0 {
		return 0, "", ""
	}
	if u, ok := e.Users[uid]; ok {
		return uid, displayName(u), u.Username
	}
	return uid, "", ""
}

func mediaType(msg *tg.Message) string {
	media, ok := msg.GetMedia()
	if !ok {
		return ""
	}
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	case *tg.MessageMediaContact:
		return "contact"
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive, *tg.MessageMediaVenue:
		return "location"
	case *tg.MessageMediaPoll:
		return "poll"
	case *tg.MessageMediaWebPage:
		return ""
	default:
		return "other"
	}
}

func displayName(u *tg.User) string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" && u.Username != "" {
		name = "@" + u.Username
	}
	return name
}

func truncate(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}
