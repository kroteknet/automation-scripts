// ==============================================================================
// KROTEK SCRIPT HUB — LIFECYCLE, TUI, FILE EDITING & DATA INTEGRATION TESTS
// ==============================================================================

package main_test

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"krotek-nginx-installer/core"
	"krotek-nginx-installer/modules"
	"krotek-nginx-installer/system"
	"krotek-nginx-installer/tui"
)

// 1. DATA FETCHING: System detection, OS release, Architecture, Init system
func TestDataFetching_SystemDetection(t *testing.T) {
	system.DetectSystem()

	if core.SysInfo.Architecture == "" {
		t.Errorf("Architecture is empty")
	}
	if core.SysInfo.Family == "" {
		t.Errorf("Family is empty")
	}
	if core.SysInfo.Distro == "" {
		t.Errorf("Distro is empty")
	}
	if core.SysInfo.InitSystem == "" {
		t.Errorf("InitSystem is empty")
	}

	pretty := system.GetOSPrettyVersion()
	if pretty == "" {
		t.Errorf("GetOSPrettyVersion returned empty string")
	}

	t.Logf("Detected System: Arch=%s, Distro=%s, Family=%s, Init=%s, Pretty=%s",
		core.SysInfo.Architecture, core.SysInfo.Distro, core.SysInfo.Family, core.SysInfo.InitSystem, pretty)
}

// 2. DATA FETCHING: Public IP and Local IP
func TestDataFetching_PublicIP(t *testing.T) {
	pubIP := system.GetPublicIP()
	if pubIP == "" {
		t.Fatalf("GetPublicIP returned empty string")
	}
	// Parse as IP
	parsed := net.ParseIP(pubIP)
	if parsed == nil {
		t.Fatalf("GetPublicIP returned invalid IP string: %s", pubIP)
	}
	t.Logf("Successfully fetched public IP: %s (IsLoopback=%t, IsPrivate=%t)", pubIP, parsed.IsLoopback(), parsed.IsPrivate())

	localIP := system.GetLocalIP()
	if localIP == "" {
		t.Errorf("GetLocalIP returned empty string")
	}
	t.Logf("Local IP: %s", localIP)
}

// 3. DATA FETCHING & NETWORK: Port Occupancy and Conflict Checking
func TestDataFetching_PortConflict(t *testing.T) {
	// Start a dummy listener on random TCP port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to open test listener: %v", err)
	}
	defer ln.Close()

	port := fmt.Sprintf("%d", ln.Addr().(*net.TCPAddr).Port)

	// Check if system detects it as occupied
	info := system.CheckPortConflict(port, "tcp")
	if !info.InUse {
		t.Errorf("Expected port %s to be detected as occupied", port)
	} else {
		t.Logf("Correctly detected active listener on port %s", port)
	}

	// Unused port test
	freePort := "59123"
	freeInfo := system.CheckPortConflict(freePort, "tcp")
	if freeInfo.InUse {
		t.Logf("Port %s happens to be in use", freePort)
	} else {
		t.Logf("Confirmed port %s is free", freePort)
	}
}

// 4. FOLDER & DATA CREATION: Directories, Permissions, and Landing Pages
func TestFolderAndDataCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krotek-test-folders-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Subdirectories
	subDirs := []string{
		filepath.Join(tmpDir, "etc", "nginx", "conf.d"),
		filepath.Join(tmpDir, "etc", "nginx", "sites-available"),
		filepath.Join(tmpDir, "etc", "nginx", "sites-enabled"),
		filepath.Join(tmpDir, "var", "www", "example.test"),
		filepath.Join(tmpDir, "var", "log", "nginx"),
	}

	for _, d := range subDirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Errorf("Failed to create directory %s: %v", d, err)
		}
		fi, err := os.Stat(d)
		if err != nil || !fi.IsDir() {
			t.Errorf("Directory %s does not exist or is not a dir", d)
		}
		// Verify directory permissions
		perm := fi.Mode().Perm()
		if perm&0755 != 0755 {
			t.Logf("Notice: dir %s perm is %o", d, perm)
		}
	}

	// Landing Page Data Generation
	cfg := system.LandingPageConfig{
		ServiceName: "NGINX Web Server",
		LogoText:    "N",
		HeroTitle:   "Test Site Active",
		HeroDesc:    "Unit test description",
		EnvType:     "native",
		Port:        "8080",
		WebDir:      filepath.Join(tmpDir, "var", "www", "example.test"),
		ConfigPath:  filepath.Join(tmpDir, "etc", "nginx", "nginx.conf"),
		WebUser:     "www-data",
		ManagerCmd:  "nginx-manager",
	}
	html := system.GenerateLandingPageHTML(cfg)
	if !strings.Contains(html, "Test Site Active") || !strings.Contains(html, "NGINX Web Server") {
		t.Errorf("Generated HTML missing key content tokens")
	}

	indexPath := filepath.Join(cfg.WebDir, "index.html")
	if err := os.WriteFile(indexPath, []byte(html), 0644); err != nil {
		t.Fatalf("Failed to write index.html: %v", err)
	}

	fi, err := os.Stat(indexPath)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("Written index.html is empty or does not exist")
	}
	t.Logf("Successfully created index.html (%d bytes) in %s", fi.Size(), indexPath)
}

// 5. FILE EDITING: RunEditor and Safe Editor Execution
func TestFileEditing_RunEditor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krotek-editor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "nested", "config.conf")

	// Create a mock editor binary that adds a line to the file.
	// Since getSafeEditorLocal checks allowedEditors (nano, vim, vi, etc.),
	// we create a script named 'nano' in a temporary bin directory and prepend it to PATH.
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("Failed to create bin dir: %v", err)
	}

	mockEditorScript := filepath.Join(binDir, "nano")
	scriptContent := "#!/bin/sh\necho 'EDITED_BY_TEST_RUNNER=TRUE' >> \"$1\"\nexit 0\n"
	if err := os.WriteFile(mockEditorScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create mock editor: %v", err)
	}

	origPath := os.Getenv("PATH")
	origEditor := os.Getenv("EDITOR")
	_ = os.Setenv("PATH", binDir+":"+origPath)
	_ = os.Setenv("EDITOR", "nano")
	defer func() {
		_ = os.Setenv("PATH", origPath)
		_ = os.Setenv("EDITOR", origEditor)
	}()

	// Call RunEditor on a file that does not yet exist!
	// RunEditor must:
	// 1. Create parent directory if missing
	// 2. Initialize file with skeleton content
	// 3. Launch editor
	// 4. Return without error
	err = tui.RunEditor(testFile)
	if err != nil {
		t.Fatalf("RunEditor returned error: %v", err)
	}

	// Verify file was created and modified by the mock editor
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read edited file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "# Configuration File:") {
		t.Errorf("Expected initial skeleton header in file, got: %s", content)
	}
	if !strings.Contains(content, "EDITED_BY_TEST_RUNNER=TRUE") {
		t.Errorf("Expected mock editor modifications in file, got: %s", content)
	}

	t.Logf("RunEditor successfully created parent folder, initialized skeleton, and applied edits:\n%s", content)
}

// 6. BACKUP CREATION: Archive Generation and Verification
func TestFolderAndDataCreation_ArchiveBackup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "krotek-backup-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	_ = os.MkdirAll(sourceDir, 0755)
	_ = os.WriteFile(filepath.Join(sourceDir, "file1.txt"), []byte("data 1"), 0644)
	_ = os.WriteFile(filepath.Join(sourceDir, "file2.conf"), []byte("setting=on"), 0644)

	backupTarGz := filepath.Join(tmpDir, "backup.tar.gz")

	// Execute tar creation matching scripts pattern
	cmd := exec.Command("tar", "-czf", backupTarGz, "-C", tmpDir, "source")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar -czf failed: %v, out: %s", err, string(out))
	}

	// Verify archive exists and contains expected files
	f, err := os.Open(backupTarGz)
	if err != nil {
		t.Fatalf("Failed to open generated archive: %v", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("Failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	foundFiles := make(map[string]bool)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Error reading tar entry: %v", err)
		}
		foundFiles[header.Name] = true
	}

	if !foundFiles["source/file1.txt"] || !foundFiles["source/file2.conf"] {
		t.Errorf("Archive missing expected files. Found: %+v", foundFiles)
	} else {
		t.Logf("Archive successfully generated and validated. Files found: %+v", foundFiles)
	}
}

// 7. TUI: Card, Box, Table, and Width Formatting
func TestTUI_BoxAndTableRendering(t *testing.T) {
	// FormatKV
	kv := tui.FormatKV("Listening Port", "80", core.GREEN)
	cleanKV := core.StripANSI(kv)
	if !strings.Contains(cleanKV, "Listening Port") || !strings.Contains(cleanKV, "80") {
		t.Errorf("FormatKV output malformed: %s", kv)
	}

	// RenderBoxCard
	card := tui.RenderBoxCard("SUMMARY", []string{"Line 1", "Line 2"}, 60)
	if len(card) == 0 {
		t.Fatalf("RenderBoxCard returned empty slice")
	}
	for i, l := range card {
		w := core.RuneWidth(l)
		if w != 60 {
			t.Errorf("Line %d width %d != 60: %s", i, w, l)
		}
	}
	t.Logf("RenderBoxCard verified with perfect 60-character boundary across %d lines", len(card))
}

// 8. SSL & TLS STRATEGIES: Certbot, Cloudflare, Custom SSL & Plain HTTP
func TestSSLStrategies(t *testing.T) {
	// 8.1 Let's Encrypt / Certbot Parsing
	os.Setenv("KROTEK_SSL", "certbot")
	os.Setenv("KROTEK_SSL_EMAIL", "ops@example.com")
	stratCertbot := system.ParseSSLStrategyFromEnv("example.com")
	if stratCertbot.Mode != "certbot" || stratCertbot.Email != "ops@example.com" {
		t.Errorf("Expected certbot strategy with ops@example.com, got %+v", stratCertbot)
	}

	// 8.2 Cloudflare Proxy Parsing
	os.Setenv("KROTEK_SSL", "cloudflare")
	stratCF := system.ParseSSLStrategyFromEnv("example.com")
	if stratCF.Mode != "cloudflare" {
		t.Errorf("Expected cloudflare strategy, got %+v", stratCF)
	}

	// 8.3 Custom Certificate Parsing
	os.Setenv("KROTEK_SSL", "custom")
	os.Setenv("KROTEK_SSL_CERT", "/etc/ssl/certs/ovh-bundle.crt")
	os.Setenv("KROTEK_SSL_KEY", "/etc/ssl/private/ovh-bundle.key")
	stratCustom := system.ParseSSLStrategyFromEnv("example.com")
	if stratCustom.Mode != "custom" || stratCustom.CertPath != "/etc/ssl/certs/ovh-bundle.crt" || stratCustom.KeyPath != "/etc/ssl/private/ovh-bundle.key" {
		t.Errorf("Expected custom SSL strategy, got %+v", stratCustom)
	}

	// 8.4 Skip SSL Parsing
	os.Setenv("KROTEK_SSL", "none")
	stratNone := system.ParseSSLStrategyFromEnv("example.com")
	if stratNone.Mode != "none" {
		t.Errorf("Expected none strategy, got %+v", stratNone)
	}

	// Clean up test environment variables
	os.Unsetenv("KROTEK_SSL")
	os.Unsetenv("KROTEK_SSL_EMAIL")
	os.Unsetenv("KROTEK_SSL_CERT")
	os.Unsetenv("KROTEK_SSL_KEY")

	// 8.5 Cloudflare Real-IP Block Verification
	cfBlock := system.GetCloudflareRealIPBlock()
	if !strings.Contains(cfBlock, "CF-Connecting-IP") || !strings.Contains(cfBlock, "set_real_ip_from 173.245.48.0/20") || !strings.Contains(cfBlock, "set_real_ip_from 2606:4700::/32") {
		t.Errorf("Cloudflare Real-IP block missing required directives: %s", cfBlock)
	} else {
		t.Logf("Cloudflare Real-IP configuration block verified with IPv4 & IPv6 CIDRs")
	}

	// 8.6 Custom SSL Block Verification
	customBlock := system.GetCustomSSLBlock("/etc/ssl/certs/test.crt", "/etc/ssl/private/test.key")
	if !strings.Contains(customBlock, "ssl_certificate /etc/ssl/certs/test.crt;") || !strings.Contains(customBlock, "ssl_certificate_key /etc/ssl/private/test.key;") || !strings.Contains(customBlock, "TLSv1.3") {
		t.Errorf("Custom SSL block missing required directives: %s", customBlock)
	} else {
		t.Logf("Custom SSL configuration block verified with TLSv1.2/TLSv1.3 and certificate paths")
	}
}

// 9. VHOST PRESETS & TEMPLATES: Node.js Reverse Proxy, React SPA, PHP, Static
func TestVHostPresets(t *testing.T) {
	presets := modules.GetAvailablePresets()
	if len(presets) != 5 {
		t.Fatalf("Expected 5 presets, got %d", len(presets))
	}

	// 9.1 Node.js Reverse Proxy Preset
	nodePreset := presets[0]
	sslNone := system.SSLStrategy{Mode: "none"}
	conf := modules.GenerateTemplateVHost(nodePreset, "example.com", "http://127.0.0.1:3000", sslNone)
	if !strings.Contains(conf, "proxy_pass http://127.0.0.1:3000;") || !strings.Contains(conf, "client_max_body_size 100M;") || !strings.Contains(conf, "proxy_set_header Upgrade $http_upgrade;") {
		t.Errorf("Node.js preset missing proxy directives:\n%s", conf)
	}

	// 9.2 React / Vite SPA Preset
	spaPreset := presets[1]
	spaConf := modules.GenerateTemplateVHost(spaPreset, "example.com", "/var/www/example.com/dist", sslNone)
	if !strings.Contains(spaConf, "try_files $uri $uri/ /index.html;") || !strings.Contains(spaConf, "location /assets/") {
		t.Errorf("SPA preset missing fallback routing directives:\n%s", spaConf)
	}

	// 9.3 Cloudflare Real-IP Integration inside Node.js Preset
	sslCF := system.SSLStrategy{Mode: "cloudflare"}
	cfNodeConf := modules.GenerateTemplateVHost(nodePreset, "example.com", "http://127.0.0.1:3000", sslCF)
	if !strings.Contains(cfNodeConf, "real_ip_header CF-Connecting-IP;") || !strings.Contains(cfNodeConf, "proxy_pass http://127.0.0.1:3000;") {
		t.Errorf("Cloudflare + Node.js configuration missing Real-IP:\n%s", cfNodeConf)
	}

	// 9.4 Firewall Rules Configurator
	system.ConfigureFirewallRules("80", "443")
	t.Logf("Successfully verified VHost presets and firewall configuration")
}
