// ==============================================================================
// KROTEK SCRIPT HUB — SSL / TLS STRATEGY & CERTIFICATE PROVISIONER
// Service: OpenResty (web-server)
// File:    web-server/openresty/src/system/ssl.go
//
// Provides interactive SSL/TLS strategy selection, Cloudflare Real-IP rules,
// custom certificate wiring (Cloudflare Origin CA, OVH, commercial certs),
// and automated Let's Encrypt Certbot integration.
// ==============================================================================

package system

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"krotek-openresty-installer/core"
	"krotek-openresty-installer/logger"
	"krotek-openresty-installer/tui"
)

// IsLocalOrPrivateDomain reports whether domainName is a local, loopback, IP,
// single-label, or reserved non-public domain that public ACME CAs (e.g. Let's Encrypt) reject.
func IsLocalOrPrivateDomain(domainName string) bool {
	d := CleanDomain(domainName)
	if d == "" || d == "localhost" || !strings.Contains(d, ".") {
		return true
	}
	if net.ParseIP(d) != nil {
		return true
	}
	localSuffixes := []string{
		".local",
		".localhost",
		".lan",
		".home",
		".home.arpa",
		".internal",
		".localdomain",
		".arpa",
		".test",
		".invalid",
		".example",
		".intranet",
		".corp",
		".private",
	}
	for _, suffix := range localSuffixes {
		if strings.HasSuffix(d, suffix) {
			return true
		}
	}
	return false
}

// SSLStrategy holds the selected SSL configuration and associated file paths / credentials.
type SSLStrategy struct {
	Mode     string // "certbot", "cloudflare", "custom", "none"
	Email    string // Administrator email for Certbot registration
	CertPath string // Filepath to .crt / .pem bundle for custom SSL mode
	KeyPath  string // Filepath to .key private key for custom SSL mode
}

func sanitizeSSLEmail(email, domainName string) string {
	clean := strings.TrimSpace(email)
	clean = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '@' || r == '.' || r == '_' || r == '-' || r == '+' {
			return r
		}
		return -1
	}, clean)
	if clean == "" || strings.HasPrefix(clean, "-") || !strings.Contains(clean, "@") {
		if domainName != "" {
			return "admin@" + domainName
		}
		return "admin@localhost"
	}
	return clean
}

func sanitizeSSLPath(p, fallback string) string {
	clean := strings.TrimSpace(p)
	if clean == "" || !strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") || strings.ContainsAny(clean, ";\r\n\x00\"' \t$`\\|&<>{}") {
		return fallback
	}
	resolved := filepath.Clean(clean)
	if resolved == "/" || !strings.HasPrefix(resolved, "/") {
		return fallback
	}
	return resolved
}

// ParseSSLStrategyFromEnv extracts unattended SSL automation parameters from environment variables.
func ParseSSLStrategyFromEnv(domainName string) SSLStrategy {
	envSSL := strings.ToLower(strings.TrimSpace(os.Getenv("KROTEK_SSL")))
	strategy := SSLStrategy{Mode: "none"}

	switch envSSL {
	case "1", "true", "certbot", "letsencrypt":
		strategy.Mode = "certbot"
	case "cloudflare", "cf":
		strategy.Mode = "cloudflare"
	case "custom", "manual", "ovh", "origin":
		strategy.Mode = "custom"
	case "none", "skip", "0", "false", "":
		strategy.Mode = "none"
	default:
		logger.LogWarn(fmt.Sprintf("Unrecognized KROTEK_SSL value '%s'; failing closed with SSL disabled.", envSSL))
		strategy.Mode = "none"
	}

	if envEmail := strings.TrimSpace(os.Getenv("KROTEK_SSL_EMAIL")); envEmail != "" {
		strategy.Email = sanitizeSSLEmail(envEmail, domainName)
	} else if domainName != "" {
		strategy.Email = "admin@" + domainName
	}

	defaultCert := fmt.Sprintf("/etc/ssl/certs/%s.crt", domainName)
	defaultKey := fmt.Sprintf("/etc/ssl/private/%s.key", domainName)

	if envCert := strings.TrimSpace(os.Getenv("KROTEK_SSL_CERT")); envCert != "" {
		strategy.CertPath = sanitizeSSLPath(envCert, defaultCert)
	} else if domainName != "" {
		strategy.CertPath = defaultCert
	}

	if envKey := strings.TrimSpace(os.Getenv("KROTEK_SSL_KEY")); envKey != "" {
		strategy.KeyPath = sanitizeSSLPath(envKey, defaultKey)
	} else if domainName != "" {
		strategy.KeyPath = defaultKey
	}

	if strategy.Mode == "certbot" && IsLocalOrPrivateDomain(domainName) {
		logger.LogWarn(fmt.Sprintf("Domain '%s' is local/private — Let's Encrypt cannot issue certificates for non-public domains. Falling back to plain HTTP.", domainName))
		strategy.Mode = "none"
	}

	return strategy
}

func promptCustomSSL(domainName string) (SSLStrategy, bool) {
	strategy := SSLStrategy{Mode: "custom"}
	defaultCert := fmt.Sprintf("/etc/ssl/certs/%s.crt", domainName)
	defaultKey := fmt.Sprintf("/etc/ssl/private/%s.key", domainName)

	certIn, stC := tui.ReadTextInput("Certificate File Path (.crt / .pem)", defaultCert)
	if stC != 0 || certIn == "" {
		return SSLStrategy{Mode: "none"}, false
	}
	strategy.CertPath = sanitizeSSLPath(certIn, defaultCert)

	keyIn, stK := tui.ReadTextInput("Private Key File Path (.key)", defaultKey)
	if stK != 0 || keyIn == "" {
		return SSLStrategy{Mode: "none"}, false
	}
	strategy.KeyPath = sanitizeSSLPath(keyIn, defaultKey)

	// Ensure parent directories exist
	_ = os.MkdirAll(filepath.Dir(strategy.CertPath), 0755)
	_ = os.MkdirAll(filepath.Dir(strategy.KeyPath), 0700)

	// Check if certificate file exists; if not, offer to create and edit now
	if _, err := os.Lstat(strategy.CertPath); os.IsNotExist(err) {
		tui.ClearScreen()
		res := tui.AskYesNo(fmt.Sprintf("Certificate file '%s' does not exist. Create skeleton & open editor now?", strategy.CertPath))
		if res == 0 {
			skeleton := "-----BEGIN CERTIFICATE-----\n# Paste your certificate or fullchain bundle here\n-----END CERTIFICATE-----\n"
			if WriteFileNoFollow(strategy.CertPath, []byte(skeleton), 0644, "") == nil {
				_ = tui.RunEditor(strategy.CertPath)
			}
		}
	}

	// Check if private key file exists; if not, offer to create and edit now
	if _, err := os.Lstat(strategy.KeyPath); os.IsNotExist(err) {
		tui.ClearScreen()
		res := tui.AskYesNo(fmt.Sprintf("Private key file '%s' does not exist. Create skeleton & open editor now?", strategy.KeyPath))
		if res == 0 {
			skeleton := "-----BEGIN PRIVATE KEY-----\n# Paste your private key here\n-----END PRIVATE KEY-----\n"
			if WriteFileNoFollow(strategy.KeyPath, []byte(skeleton), 0600, "") == nil {
				_ = tui.RunEditor(strategy.KeyPath)
			}
		}
	}

	return strategy, true
}

// PromptSSLStrategy renders an interactive TUI card allowing the administrator to choose
// how HTTPS and SSL/TLS certificates will be managed for the given domain.
func PromptSSLStrategy(domainName string) (SSLStrategy, bool) {
	tui.ClearScreen()

	if IsLocalOrPrivateDomain(domainName) {
		cardLines := []string{
			core.BOLD + core.CYAN + "SSL / TLS Strategy (Local / Private Domain)" + core.NC,
			"",
			"  " + core.BOLD + core.YELLOW + "[!] Local / non-public domain detected: " + domainName + core.NC,
			"      Public ACME CAs (Let's Encrypt) and Cloudflare Edge Proxy cannot",
			"      issue or terminate certificates for .local / private hostnames.",
			"",
			"  " + core.BOLD + core.GREEN + "[1] Skip SSL (Plain HTTP Port 80 Only) [Recommended]" + core.NC,
			"      Configures OpenResty cleanly on HTTP port 80 without ACME errors.",
			"",
			"  " + core.BOLD + core.CYAN + "[2] Custom / Self-Signed Certificate (.crt & .key)" + core.NC,
			"      Supply your own local CA or self-signed certificate and private key.",
		}
		choices := []string{
			"1. Skip SSL (Plain HTTP Port 80 Only)",
			"2. Custom / Self-Signed Certificate (.crt & .key)",
		}
		idx, _, status := tui.SelectMenuWithContent("Step: SSL & Encryption Strategy", cardLines, choices)
		if status != 0 {
			return SSLStrategy{Mode: "none"}, false
		}
		if idx == 1 {
			return promptCustomSSL(domainName)
		}
		return SSLStrategy{Mode: "none"}, true
	}

	cardLines := []string{
		core.BOLD + core.CYAN + "SSL / TLS Encryption Strategy" + core.NC,
		"",
		"  Choose how HTTPS traffic and certificates will be configured for " + core.BOLD + domainName + core.NC + ":",
		"",
		"  " + core.BOLD + core.GREEN + "[1] Automated Let's Encrypt (Certbot)" + core.NC,
		"      Free ACME certificate with automatic renewal cron. Requires DNS A record",
		"      pointing directly to this host IP.",
		"",
		"  " + core.BOLD + core.YELLOW + "[2] Cloudflare Edge Proxy (Skip Local SSL + Real-IP)" + core.NC,
		"      SSL is terminated at Cloudflare CDN edge (Flexible/Full). Configures OpenResty",
		"      on HTTP port 80 with automatic Cloudflare CF-Connecting-IP restoration.",
		"",
		"  " + core.BOLD + core.CYAN + "[3] Custom Certificate / Cloudflare Origin CA / OVH" + core.NC,
		"      Supply your own certificate and private key files (e.g. 15-year Cloudflare",
		"      Origin CA, OVH SSL, Sectigo, or DigiCert bundle).",
		"",
		"  " + core.BOLD + core.RED + "[4] Skip SSL (Plain HTTP Port 80 Only)" + core.NC,
		"      No SSL/TLS certificate will be provisioned on this machine.",
	}

	choices := []string{
		"1. Automated Let's Encrypt (Certbot)",
		"2. Cloudflare Edge Proxy (Skip Local SSL + Real-IP)",
		"3. Custom Certificate / Cloudflare Origin CA / OVH (.crt & .key)",
		"4. Skip SSL (Plain HTTP Port 80 Only)",
	}

	idx, _, status := tui.SelectMenuWithContent("Step: SSL & Encryption Strategy", cardLines, choices)
	if status != 0 {
		return SSLStrategy{Mode: "none"}, false
	}

	strategy := SSLStrategy{}

	switch idx {
	case 0: // Let's Encrypt
		strategy.Mode = "certbot"
		emailIn, st := tui.ReadTextInput("SSL Admin Email for Let's Encrypt", "admin@"+domainName)
		if st != 0 {
			return SSLStrategy{Mode: "none"}, false
		}
		strategy.Email = sanitizeSSLEmail(emailIn, domainName)

	case 1: // Cloudflare Edge Proxy
		strategy.Mode = "cloudflare"
		tui.ClearScreen()
		tui.PrintInBox("Cloudflare Proxy Selected", []string{
			core.GREEN + "[SUCCESS] Cloudflare Edge Proxy Integration Enabled" + core.NC,
			"",
			"  " + core.Sym.Bullet + " Local SSL certificate generation will be skipped.",
			"  " + core.Sym.Bullet + " OpenResty virtual host will listen on HTTP port 80.",
			"  " + core.Sym.Bullet + " Cloudflare Real-IP rules (CF-Connecting-IP) will be injected.",
			"  " + core.Sym.Bullet + " Ensures client visitor IP addresses are preserved in access logs.",
		})
		time.Sleep(1200 * time.Millisecond)

	case 2: // Custom Certificate / Cloudflare Origin CA / OVH
		return promptCustomSSL(domainName)

	case 3: // Skip SSL
		strategy.Mode = "none"
	}

	return strategy, true
}

// GetCloudflareRealIPBlock generates OpenResty/Nginx configuration directives to restore
// real client visitor IPs from Cloudflare reverse proxy edge servers.
func GetCloudflareRealIPBlock() string {
	return `    # Cloudflare Real-IP Restoration
    set_real_ip_from 173.245.48.0/20;
    set_real_ip_from 103.21.244.0/22;
    set_real_ip_from 103.22.200.0/22;
    set_real_ip_from 103.31.4.0/22;
    set_real_ip_from 141.101.64.0/18;
    set_real_ip_from 108.162.192.0/18;
    set_real_ip_from 190.93.240.0/20;
    set_real_ip_from 188.114.96.0/20;
    set_real_ip_from 197.234.240.0/22;
    set_real_ip_from 198.41.128.0/17;
    set_real_ip_from 162.158.0.0/15;
    set_real_ip_from 104.16.0.0/13;
    set_real_ip_from 104.24.0.0/14;
    set_real_ip_from 172.64.0.0/13;
    set_real_ip_from 131.0.72.0/22;
    set_real_ip_from 2400:cb00::/32;
    set_real_ip_from 2606:4700::/32;
    set_real_ip_from 2803:f800::/32;
    set_real_ip_from 2405:b500::/32;
    set_real_ip_from 2405:8100::/32;
    set_real_ip_from 2a06:98c0::/29;
    set_real_ip_from 2c0f:f248::/32;
    real_ip_header CF-Connecting-IP;
`
}

// GetCustomSSLBlock generates modern, secure TLS parameters for custom SSL certificates.
func GetCustomSSLBlock(certPath, keyPath string) string {
	safeCert := sanitizeSSLPath(certPath, "/etc/ssl/certs/localhost.crt")
	safeKey := sanitizeSSLPath(keyPath, "/etc/ssl/private/localhost.key")
	return fmt.Sprintf(`    # SSL / TLS Directives
    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers off;
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
`, safeCert, safeKey)
}

// LogSSLSummary outputs diagnostic feedback about the activated SSL strategy.
func LogSSLSummary(strategy SSLStrategy, domainName string) {
	switch strategy.Mode {
	case "certbot":
		logger.LogInfo(fmt.Sprintf("SSL Mode: Automated Let's Encrypt (Certbot) registered to %s", strategy.Email))
	case "cloudflare":
		logger.LogInfo(fmt.Sprintf("SSL Mode: Cloudflare Edge Proxy with Real-IP restoration for %s", domainName))
	case "custom":
		logger.LogInfo(fmt.Sprintf("SSL Mode: Custom Certificate (%s, %s)", strategy.CertPath, strategy.KeyPath))
	default:
		logger.LogInfo("SSL Mode: Disabled (Plain HTTP)")
	}
}
