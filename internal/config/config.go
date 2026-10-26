package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Embedding     EmbeddingConfig `yaml:"embedding"`
	LLM           LLMConfig       `yaml:"llm"`
	Chunk         ChunkConfig     `yaml:"chunk"`
	Store         StoreConfig     `yaml:"store"`
	Log           LogConfig       `yaml:"log"`
	Web           WebConfig       `yaml:"web"`
	APIKey        string          `yaml:"-"`
	AdminUser     string          `yaml:"-"`
	AdminPassword string          `yaml:"-"`
}

type EmbeddingConfig struct {
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	BatchSize int    `yaml:"batch_size"`
}

type LLMConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type ChunkConfig struct {
	Size    int `yaml:"size"`
	Overlap int `yaml:"overlap"`
}

type StoreConfig struct {
	Path string `yaml:"path"`
}

type LogConfig struct {
	File string `yaml:"file"`
}

type WebConfig struct {
	Addr        string `yaml:"addr"`
	UploadDir   string `yaml:"upload_dir"`
	MaxUploadMB int    `yaml:"max_upload_mb"`
	FrontendDir string `yaml:"frontend_dir"`
}

func defaults() Config {
	return Config{
		Embedding: EmbeddingConfig{BaseURL: "http://localhost:11434", Model: "bge-m3", BatchSize: 16},
		Chunk:     ChunkConfig{Size: 500, Overlap: 80},
		Store:     StoreConfig{Path: "./data/KB-RAG.db"},
		Log:       LogConfig{File: "./data/logs/KB-RAG.log"},
		Web: WebConfig{
			Addr:        "127.0.0.1:8080",
			UploadDir:   "./data/uploads",
			MaxUploadMB: 20,
			FrontendDir: "./fronted/dist",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}
	cfg.APIKey = os.Getenv("KB_RAG_LLM_API_KEY")
	cfg.AdminUser = strings.TrimSpace(os.Getenv("KB_RAG_ADMIN_USER"))
	if cfg.AdminUser == "" {
		cfg.AdminUser = "admin"
	}
	cfg.AdminPassword = os.Getenv("KB_RAG_ADMIN_PASSWORD")
	return &cfg, nil
}

func (c *Config) Validate(command string) error {
	if err := validateURL("embedding.base_url", c.Embedding.BaseURL); err != nil {
		return err
	}
	if strings.TrimSpace(c.Embedding.Model) == "" {
		return fmt.Errorf("embedding.model 不能为空")
	}
	if c.Embedding.BatchSize <= 0 {
		return fmt.Errorf("embedding.batch_size 必须大于 0")
	}
	if c.Chunk.Size <= 0 || c.Chunk.Overlap <= 0 || c.Chunk.Size <= c.Chunk.Overlap {
		return fmt.Errorf("chunk 配置必须满足 size > overlap > 0（当前 size=%d, overlap=%d）", c.Chunk.Size, c.Chunk.Overlap)
	}
	if strings.TrimSpace(c.Store.Path) == "" {
		return fmt.Errorf("store.path 不能为空")
	}
	if err := ensureDir(filepath.Dir(c.Store.Path)); err != nil {
		return err
	}
	if strings.TrimSpace(c.Log.File) == "" {
		return fmt.Errorf("log.file 不能为空")
	}
	if err := ensureDir(filepath.Dir(c.Log.File)); err != nil {
		return err
	}
	if command == "ask" || command == "serve" {
		if err := validateURL("llm.base_url", c.LLM.BaseURL); err != nil {
			return err
		}
		if strings.TrimSpace(c.LLM.Model) == "" {
			return fmt.Errorf("llm.model 不能为空")
		}
		if c.APIKey == "" {
			return fmt.Errorf("环境变量 KB_RAG_LLM_API_KEY 不能为空")
		}
	}
	if command == "serve" {
		return c.validateServe()
	}
	return nil
}

func (c *Config) validateServe() error {
	if err := validateAddr(c.Web.Addr); err != nil {
		return err
	}
	if strings.TrimSpace(c.Web.UploadDir) == "" {
		return fmt.Errorf("web.upload_dir 不能为空")
	}
	if err := ensureDir(c.Web.UploadDir); err != nil {
		return err
	}
	if c.Web.MaxUploadMB <= 0 {
		return fmt.Errorf("web.max_upload_mb 必须大于 0")
	}
	if strings.TrimSpace(c.Web.FrontendDir) == "" {
		return fmt.Errorf("web.frontend_dir 不能为空")
	}
	index := filepath.Join(c.Web.FrontendDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		return fmt.Errorf("前端未构建：缺少 %s，请先在 fronted 执行 npm run build", index)
	}
	return nil
}

func validateAddr(raw string) error {
	_, port, err := net.SplitHostPort(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("web.addr 必须是 host:port（当前为 %q）", raw)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("web.addr 端口非法（当前为 %q）", raw)
	}
	return nil
}

func validateURL(field, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s 必须是合法的 http(s) URL（当前为 %q）", field, raw)
	}
	return nil
}

func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("无法创建目录 %s: %w", dir, err)
	}
	return nil
}
