package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaultsAndEnv(t *testing.T) {
	t.Setenv("KB_RAG_LLM_API_KEY", "secret")
	path := writeConfig(t, "llm:\n  base_url: http://localhost:9999\n  model: test-llm\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Embedding.BaseURL != "http://localhost:11434" {
		t.Errorf("embedding.base_url = %q", cfg.Embedding.BaseURL)
	}
	if cfg.Embedding.Model != "bge-m3" {
		t.Errorf("embedding.model = %q", cfg.Embedding.Model)
	}
	if cfg.Embedding.BatchSize != 16 {
		t.Errorf("embedding.batch_size = %d", cfg.Embedding.BatchSize)
	}
	if cfg.Chunk.Size != 500 || cfg.Chunk.Overlap != 80 {
		t.Errorf("chunk = %+v", cfg.Chunk)
	}
	if cfg.Store.Path != "./data/KB-RAG.db" {
		t.Errorf("store.path = %q", cfg.Store.Path)
	}
	if cfg.Log.File != "./data/logs/KB-RAG.log" {
		t.Errorf("log.file = %q", cfg.Log.File)
	}
	if cfg.LLM.BaseURL != "http://localhost:9999" || cfg.LLM.Model != "test-llm" {
		t.Errorf("llm = %+v", cfg.LLM)
	}
	if cfg.APIKey != "secret" {
		t.Errorf("APIKey 未从环境变量读取")
	}
}

func validConfig(dir string) *Config {
	return &Config{
		Embedding: EmbeddingConfig{BaseURL: "http://localhost:11434", Model: "bge-m3", BatchSize: 16},
		LLM:       LLMConfig{BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat"},
		Chunk:     ChunkConfig{Size: 500, Overlap: 80},
		Store:     StoreConfig{Path: filepath.Join(dir, "data", "KB-RAG.db")},
		Log:       LogConfig{File: filepath.Join(dir, "logs", "KB-RAG.log")},
		APIKey:    "key",
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		cmd    string
		mutate func(*Config)
	}{
		{"bad embedding url", "ingest", func(c *Config) { c.Embedding.BaseURL = "localhost:11434" }},
		{"empty embedding model", "ingest", func(c *Config) { c.Embedding.Model = "" }},
		{"bad batch size", "ingest", func(c *Config) { c.Embedding.BatchSize = 0 }},
		{"overlap equals size", "ingest", func(c *Config) { c.Chunk.Overlap = 500 }},
		{"empty store path", "ingest", func(c *Config) { c.Store.Path = "" }},
		{"empty log file", "ingest", func(c *Config) { c.Log.File = "" }},
		{"ask bad llm url", "ask", func(c *Config) { c.LLM.BaseURL = "ftp://example.com" }},
		{"ask empty llm model", "ask", func(c *Config) { c.LLM.Model = "" }},
		{"ask empty key", "ask", func(c *Config) { c.APIKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig(t.TempDir())
			tc.mutate(cfg)
			if err := cfg.Validate(tc.cmd); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

func TestValidateOK(t *testing.T) {
	cfg := validConfig(t.TempDir())
	if err := cfg.Validate("ask"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !strings.Contains(cfg.Embedding.BaseURL, "http") {
		t.Fatal("测试前置数据异常")
	}
}

func TestLoadWebDefaultsAndToken(t *testing.T) {
	t.Setenv("KB_RAG_WEB_TOKEN", "0123456789abcdef")
	path := writeConfig(t, "llm:\n  base_url: http://localhost:9999\n  model: test-llm\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Web.Addr != "127.0.0.1:8080" {
		t.Errorf("web.addr = %q", cfg.Web.Addr)
	}
	if cfg.Web.UploadDir != "./data/uploads" {
		t.Errorf("web.upload_dir = %q", cfg.Web.UploadDir)
	}
	if cfg.Web.MaxUploadMB != 20 {
		t.Errorf("web.max_upload_mb = %d", cfg.Web.MaxUploadMB)
	}
	if cfg.Web.FrontendDir != "./fronted/dist" {
		t.Errorf("web.frontend_dir = %q", cfg.Web.FrontendDir)
	}
	if cfg.WebToken != "0123456789abcdef" {
		t.Errorf("WebToken = %q", cfg.WebToken)
	}
}

func TestValidateServe(t *testing.T) {
	dir := t.TempDir()
	frontend := filepath.Join(dir, "frontend")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frontend, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := func() *Config {
		c := validConfig(dir)
		c.Web = WebConfig{
			Addr:        "127.0.0.1:8080",
			UploadDir:   filepath.Join(dir, "uploads"),
			MaxUploadMB: 20,
			FrontendDir: frontend,
		}
		c.WebToken = "0123456789abcdef"
		return c
	}
	if err := base().Validate("serve"); err != nil {
		t.Fatalf("合法 serve 配置被拒绝: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"bad addr", func(c *Config) { c.Web.Addr = "localhost" }},
		{"bad port", func(c *Config) { c.Web.Addr = "127.0.0.1:99999" }},
		{"zero upload limit", func(c *Config) { c.Web.MaxUploadMB = 0 }},
		{"missing frontend index", func(c *Config) { c.Web.FrontendDir = filepath.Join(dir, "missing") }},
		{"short token", func(c *Config) { c.WebToken = "short" }},
		{"missing api key", func(c *Config) { c.APIKey = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(cfg)
			if err := cfg.Validate("serve"); err == nil {
				t.Fatal("期望校验失败")
			}
		})
	}
}
