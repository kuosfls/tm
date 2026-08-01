package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config 是 tgsms 的全部配置, 存储于 YAML 文件 (默认 /etc/tgsms/config.yaml)。
type Config struct {
	APIID           int     `yaml:"api_id"`
	APIHash         string  `yaml:"api_hash"`
	Phone           string  `yaml:"phone"`
	WebhookURL      string  `yaml:"webhook_url"`
	WebhookSecret   string  `yaml:"webhook_secret"`
	ChatIDs         []int64 `yaml:"chat_ids"`
	Proxy           string  `yaml:"proxy"`
	IncludeOutgoing bool    `yaml:"include_outgoing"`
}

func defaultConfigPath() string {
	if env := os.Getenv("TGSMS_CONFIG"); env != "" {
		return env
	}
	if runtime.GOOS == "linux" {
		return "/etc/tgsms/config.yaml"
	}
	return "config.yaml"
}

// 会话文件与配置文件放在同一目录
func sessionPathFor(cfgPath string) string {
	return filepath.Join(filepath.Dir(cfgPath), "session.json")
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	return &cfg, nil
}

func loadOrEmpty(path string) (*Config, error) {
	cfg, err := loadConfig(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	return cfg, err
}

func saveConfig(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# tgsms 配置文件 (可用 tgsms config set <key> <value> 修改)\n"
	// 0600: 文件内含 api_hash 等敏感信息
	return os.WriteFile(path, append([]byte(header), data...), 0o600)
}

const configTemplate = `# tgsms 配置文件
# api_id / api_hash 在 https://my.telegram.org -> API development tools 申请
api_id: 0
api_hash: ""
# 手机号 (含国际区号, 如 +8613800138000), 留空则登录时交互输入
phone: ""
# 收到消息后 POST 的目标地址, 如 https://example.com/hook
webhook_url: ""
# 可选: 推送请求头 X-TGSMS-Secret 的值, 用于目标端校验来源
webhook_secret: ""
# 监听的会话 ID 列表: 用户为正数, 普通群为负数, 频道/超级群以 -100 开头
# 可用 tgsms chats 查看最近会话的 ID, 例:
# chat_ids:
#   - 777000
#   - -1001234567890
chat_ids: []
# 可选: socks5 代理, 如 socks5://127.0.0.1:1080 (支持 socks5://user:pass@host:port)
proxy: ""
# 是否也转发自己发出的消息
include_outgoing: false
`

func configInit(path string) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Println("配置文件已存在:", path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(configTemplate), 0o600); err != nil {
		return err
	}
	fmt.Println("已生成默认配置:", path)
	return nil
}

func (c *Config) validateForRun() error {
	if c.APIID == 0 || c.APIHash == "" {
		return fmt.Errorf("%w: api_id / api_hash 未设置, 请先执行 tgsms config set api_id <ID> 和 tgsms config set api_hash <HASH>", errBadConfig)
	}
	if c.WebhookURL == "" {
		return fmt.Errorf("%w: webhook_url 未设置, 请先执行 tgsms config set webhook_url <URL>", errBadConfig)
	}
	return nil
}
