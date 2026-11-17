// ==============================================================================
// KROTEK SCRIPT HUB — SSL / TLS STRATEGY & CERTIFICATE PROVISIONER
// Service: APACHE (web-server)
// File:    web-server/apache/src/system/ssl.go
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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-apache-installer/core"
	"krotek-apache-installer/logger"
	"krotek-apache-installer/tui"
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
	if clean == "" || !strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") || strings.ContainsAny(clean, ";\r\n\"' \t$`\\|&<>") {
		return fallback
	}
	return filepath.Clean(clean)
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
	case "none", "skip", "0", "false":
		strategy.Mode = "none"
	default:
		if envSSL != "" {
			strategy.Mode = "certbot"
		}
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
	tui.ClearScreen()
	certPathDefault := fmt.Sprintf("/etc/ssl/certs/%s.crt", domainName)
	certPromptLines := []string{
		core.BOLD + core.CYAN + "Custom Certificate File (.crt / .pem)" + core.NC,
		"",
		"  Enter the full absolute path to your SSL certificate bundle.",
		"  (e.g. Cloudflare Origin CA certificate, OVH cert, or fullchain.pem)",
	}
	tui.PrintInBox("--no-wait", "Custom SSL Certificate", certPromptLines)

	cPath, st := tui.ReadTextInput("Certificate File Path", certPathDefault)
	if st != 0 || strings.TrimSpace(cPath) == "" {
		return SSLStrategy{Mode: "none"}, false
	}
	strategy.CertPath = sanitizeSSLPath(cPath, certPathDefault)

	keyPathDefault := fmt.Sprintf("/etc/ssl/private/%s.key", domainName)
	keyPromptLines := []string{
		core.BOLD + core.CYAN + "Custom Private Key File (.key)" + core.NC,
		"",
		"  Enter the full absolute path to the matching private key.",
	}
	tui.PrintInBox("--no-wait", "Custom SSL Private Key", keyPromptLines)

	kPath, st := tui.ReadTextInput("Private Key File Path", keyPathDefault)
	if st != 0 || strings.TrimSpace(kPath) == "" {
		return SSLStrategy{Mode: "none"}, false
	}
	strategy.KeyPath = sanitizeSSLPath(kPath, keyPathDefault)

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
			"      Configures Apache cleanly on HTTP port 80 without ACME errors.",
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
		"      SSL is terminated at Cloudflare CDN edge (Flexible/Full). Configures Apache",
		"      on HTTP port 80 with automatic mod_remoteip visitor IP restoration.",
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
		infoLines := []string{
			core.BOLD + core.YELLOW + "Cloudflare Proxy Selected" + core.NC,
			"",
			"  " + core.Sym.Bullet + " Apache will listen on HTTP port 80.",
			"  " + core.Sym.Bullet + " mod_remoteip will be enabled to restore visitors' real IP addresses.",
			"  " + core.Sym.Bullet + " Set your Cloudflare SSL/TLS encryption mode to 'Flexible' or install",
			"    an Origin CA certificate using option [3] for 'Full (Strict)' mode.",
		}
		tui.PrintInBox("Cloudflare Information", infoLines)
		time.Sleep(1200 * time.Millisecond)

	case 2: // Custom Certificate / Cloudflare Origin CA / OVH
		return promptCustomSSL(domainName)

	case 3: // Skip SSL
		strategy.Mode = "none"
	}

	return strategy, true
}

// ConfigureCloudflareRealIP creates mod_remoteip configuration for Apache so real visitor IPs are restored.
func ConfigureCloudflareRealIP() {
	cfConfig := `# Cloudflare Real-IP Restoration for Apache
<IfModule remoteip_module>
    RemoteIPHeader CF-Connecting-IP
    RemoteIPTrustedProxy 173.245.48.0/20
    RemoteIPTrustedProxy 103.21.244.0/22
    RemoteIPTrustedProxy 103.22.200.0/22
    RemoteIPTrustedProxy 103.31.4.0/22
    RemoteIPTrustedProxy 141.101.64.0/18
    RemoteIPTrustedProxy 108.162.192.0/18
    RemoteIPTrustedProxy 190.93.240.0/20
    RemoteIPTrustedProxy 188.114.96.0/20
    RemoteIPTrustedProxy 197.234.240.0/22
    RemoteIPTrustedProxy 198.41.128.0/17
    RemoteIPTrustedProxy 162.158.0.0/15
    RemoteIPTrustedProxy 104.16.0.0/13
    RemoteIPTrustedProxy 104.24.0.0/14
    RemoteIPTrustedProxy 172.64.0.0/13
    RemoteIPTrustedProxy 131.0.72.0/22
    RemoteIPTrustedProxy 2400:cb00::/32
    RemoteIPTrustedProxy 2606:4700::/32
    RemoteIPTrustedProxy 2803:f800::/32
    RemoteIPTrustedProxy 2405:b500::/32
    RemoteIPTrustedProxy 2405:8100::/32
    RemoteIPTrustedProxy 2a06:98c0::/29
    RemoteIPTrustedProxy 2c0f:f248::/32
</IfModule>
`
	if core.SysInfo.Family == "debian" {
		_ = exec.Command("a2enmod", "remoteip").Run()
		_ = os.MkdirAll("/etc/apache2/conf-available", 0755)
		_ = WriteFileNoFollow("/etc/apache2/conf-available/cloudflare-realip.conf", []byte(cfConfig), 0644, "")
		_ = exec.Command("a2enconf", "cloudflare-realip").Run()
	} else if core.SysInfo.Family == "rhel" {
		_ = os.MkdirAll("/etc/httpd/conf.d", 0755)
		_ = WriteFileNoFollow("/etc/httpd/conf.d/cloudflare-realip.conf", []byte(cfConfig), 0644, "")
	}
	logger.LogSuccess("Cloudflare Real-IP rules installed successfully for Apache.")
}

// DetectExistingSSLCert checks whether a certificate already exists in common cert directories.
func DetectExistingSSLCert(domainName string) (certPath string, keyPath string, found bool) {
	candidates := []struct {
		cert string
		key  string
	}{
		{
			cert: fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", domainName),
			key:  fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", domainName),
		},
		{
			cert: fmt.Sprintf("/etc/ssl/certs/%s.crt", domainName),
			key:  fmt.Sprintf("/etc/ssl/private/%s.key", domainName),
		},
		{
			cert: fmt.Sprintf("/etc/ssl/%s/cert.pem", domainName),
			key:  fmt.Sprintf("/etc/ssl/%s/key.pem", domainName),
		},
	}

	for _, c := range candidates {
		if _, errC := os.Stat(c.cert); errC == nil {
			if _, errK := os.Stat(c.key); errK == nil {
				return c.cert, c.key, true
			}
		}
	}
	return "", "", false
}

// GenerateSelfSignedCert generates a fallback SSL certificate for localhost/dev domains.
func GenerateSelfSignedCert(domainName string) (string, string, error) {
	certDir := "/etc/ssl/certs"
	keyDir := "/etc/ssl/private"
	_ = os.MkdirAll(certDir, 0755)
	_ = os.MkdirAll(keyDir, 0700)

	certPath := filepath.Join(certDir, domainName+".crt")
	keyPath := filepath.Join(keyDir, domainName+".key")

	if _, err := os.Stat(certPath); err == nil {
		return certPath, keyPath, nil
	}

	cmd := exec.Command("openssl", "req", "-x509", "-nodes", "-days", "365",
		"-newkey", "rsa:2048",
		"-keyout", keyPath,
		"-out", certPath,
		"-subj", fmt.Sprintf("/CN=%s/O=Krotek Apache Server", domainName),
	)
	if err := cmd.Run(); err != nil {
		return "", "", err
	}
	_ = os.Chmod(keyPath, 0600)
	_ = os.Chmod(certPath, 0644)
	return certPath, keyPath, nil
}
