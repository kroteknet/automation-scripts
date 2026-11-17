package main

import (
	"os"
	"strings"
	"testing"

	"krotek-apache-installer/core"
	"krotek-apache-installer/modules"
	"krotek-apache-installer/system"
	"krotek-apache-installer/tui"
)

// 1. SSL & TLS Strategies: Certbot, Cloudflare, Custom SSL & Plain HTTP
func TestSSLStrategies(t *testing.T) {
	// 1.1 Let's Encrypt / Certbot Parsing
	os.Setenv("KROTEK_SSL", "certbot")
	os.Setenv("KROTEK_SSL_EMAIL", "ops@example.com")
	stratCertbot := system.ParseSSLStrategyFromEnv("example.com")
	if stratCertbot.Mode != "certbot" || stratCertbot.Email != "ops@example.com" {
		t.Errorf("Expected certbot strategy with ops@example.com, got %+v", stratCertbot)
	}

	// 1.2 Cloudflare Proxy Parsing
	os.Setenv("KROTEK_SSL", "cloudflare")
	stratCF := system.ParseSSLStrategyFromEnv("example.com")
	if stratCF.Mode != "cloudflare" {
		t.Errorf("Expected cloudflare strategy, got %+v", stratCF)
	}

	// 1.3 Custom Certificate Parsing
	os.Setenv("KROTEK_SSL", "custom")
	os.Setenv("KROTEK_SSL_CERT", "/etc/ssl/certs/ovh-bundle.crt")
	os.Setenv("KROTEK_SSL_KEY", "/etc/ssl/private/ovh-bundle.key")
	stratCustom := system.ParseSSLStrategyFromEnv("example.com")
	if stratCustom.Mode != "custom" || stratCustom.CertPath != "/etc/ssl/certs/ovh-bundle.crt" || stratCustom.KeyPath != "/etc/ssl/private/ovh-bundle.key" {
		t.Errorf("Expected custom SSL strategy, got %+v", stratCustom)
	}

	// 1.4 Skip SSL Parsing
	os.Setenv("KROTEK_SSL", "none")
	stratNone := system.ParseSSLStrategyFromEnv("example.com")
	if stratNone.Mode != "none" {
		t.Errorf("Expected none strategy, got %+v", stratNone)
	}

	// Clean up
	os.Unsetenv("KROTEK_SSL")
	os.Unsetenv("KROTEK_SSL_EMAIL")
	os.Unsetenv("KROTEK_SSL_CERT")
	os.Unsetenv("KROTEK_SSL_KEY")
}

// 2. Application Presets & VirtualHost Templates
func TestVHostPresets(t *testing.T) {
	presets := modules.GetAvailablePresets()
	if len(presets) < 5 {
		t.Fatalf("Expected at least 5 presets, got %d", len(presets))
	}

	// Node.js Reverse Proxy
	nodeVHost := modules.GenerateTemplateVHost("api.test.com", modules.PresetNodeJS, "http://127.0.0.1:3000", system.SSLStrategy{Mode: "none"})
	if !strings.Contains(nodeVHost, "ProxyPreserveHost On") || !strings.Contains(nodeVHost, "ProxyPass / http://127.0.0.1:3000/") {
		t.Errorf("Node.js VHost template invalid:\n%s", nodeVHost)
	}

	// SPA Fallback Routing
	spaVHost := modules.GenerateTemplateVHost("app.test.com", modules.PresetSPA, "/var/www/app.test.com", system.SSLStrategy{Mode: "none"})
	if !strings.Contains(spaVHost, "RewriteRule . /index.html [L]") || !strings.Contains(spaVHost, "DocumentRoot /var/www/app.test.com") {
		t.Errorf("SPA VHost template invalid:\n%s", spaVHost)
	}

	// Custom SSL VirtualHost
	sslVHost := modules.GenerateTemplateVHost("secure.test.com", modules.PresetStatic, "/var/www/secure.test.com", system.SSLStrategy{
		Mode:     "custom",
		CertPath: "/etc/ssl/certs/custom.crt",
		KeyPath:  "/etc/ssl/private/custom.key",
	})
	if !strings.Contains(sslVHost, "<VirtualHost *:443>") || !strings.Contains(sslVHost, "SSLEngine on") || !strings.Contains(sslVHost, "SSLCertificateFile /etc/ssl/certs/custom.crt") {
		t.Errorf("SSL VHost template missing SSL directives:\n%s", sslVHost)
	}
}

// 3. TUI Box Rendering
func TestTUI_BoxRendering(t *testing.T) {
	kv := tui.FormatKV("Apache Port", "80", core.GREEN)
	cleanKV := core.StripANSI(kv)
	if !strings.Contains(cleanKV, "Apache Port") || !strings.Contains(cleanKV, "80") {
		t.Errorf("FormatKV output malformed: %s", kv)
	}
}
