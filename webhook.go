package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookPayload 是 POST 到目标地址的 JSON 结构
type WebhookPayload struct {
	Event          string `json:"event"` // message | test
	MessageID      int    `json:"message_id,omitempty"`
	ChatID         int64  `json:"chat_id,omitempty"`
	ChatType       string `json:"chat_type,omitempty"` // user | group | channel
	ChatTitle      string `json:"chat_title,omitempty"`
	SenderID       int64  `json:"sender_id,omitempty"`
	SenderName     string `json:"sender_name,omitempty"`
	SenderUsername string `json:"sender_username,omitempty"`
	Text           string `json:"text"`
	Media          string `json:"media,omitempty"` // photo | document | ...
	Date           int64  `json:"date"`            // 消息时间 (unix)
	ReceivedAt     int64  `json:"received_at"`     // 本机收到时间 (unix)
}

type Webhook struct {
	url    string
	secret string
	client *http.Client
	queue  chan WebhookPayload
}

func NewWebhook(url, secret string) *Webhook {
	return &Webhook{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 15 * time.Second},
		queue:  make(chan WebhookPayload, 1024),
	}
}

// Start 启动后台推送协程, 消息入队后异步投递, 不阻塞 Telegram 更新处理
func (w *Webhook) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-w.queue:
				w.deliver(ctx, p)
			}
		}
	}()
}

func (w *Webhook) Enqueue(p WebhookPayload) {
	select {
	case w.queue <- p:
	default:
		log.Printf("[webhook] 队列已满, 丢弃消息 chat=%d msg=%d", p.ChatID, p.MessageID)
	}
}

func (w *Webhook) deliver(ctx context.Context, p WebhookPayload) {
	delays := []time.Duration{0, 2 * time.Second, 5 * time.Second, 15 * time.Second}
	for i, d := range delays {
		if d > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(d):
			}
		}
		err := w.post(ctx, p)
		if err == nil {
			log.Printf("[webhook] 已推送 chat=%d msg=%d", p.ChatID, p.MessageID)
			return
		}
		log.Printf("[webhook] 推送失败 (第%d次): %v", i+1, err)
	}
	log.Printf("[webhook] 重试次数用尽, 放弃推送 chat=%d msg=%d", p.ChatID, p.MessageID)
}

func (w *Webhook) post(ctx context.Context, p WebhookPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tgsms/"+version)
	if w.secret != "" {
		req.Header.Set("X-TGSMS-Secret", w.secret)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("目标返回 HTTP %d", resp.StatusCode)
	}
	return nil
}

func runTest(ctx context.Context, cfg *Config, _ string) error {
	if cfg.WebhookURL == "" {
		return fmt.Errorf("%w: webhook_url 未设置", errBadConfig)
	}
	w := NewWebhook(cfg.WebhookURL, cfg.WebhookSecret)
	now := time.Now().Unix()
	err := w.post(ctx, WebhookPayload{
		Event:      "test",
		Text:       "tgsms webhook 连通性测试",
		Date:       now,
		ReceivedAt: now,
	})
	if err != nil {
		return errors.New("webhook 测试失败: " + err.Error())
	}
	fmt.Println("webhook 测试成功: 目标返回 2xx")
	return nil
}
