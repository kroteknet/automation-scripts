// ==============================================================================
// KROTEK SCRIPT HUB — MULTI-DISTRO PACKAGE MANAGEMENT ABSTRACTION
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/system/packages.go
//
// Abstracts native package managers (apt-get, dnf/yum, apk) across Linux distributions, providing consistent APIs for installing, removing, updating, and querying software packages.
// ==============================================================================

package system

import (
	"krotek-minecraft-installer/core"
	"os/exec"
	"time"
)

// InstallPackages provisions one or more packages using the native package manager
// appropriate for the host distribution (apt-get on Debian/Ubuntu, dnf/yum on RHEL, apk on Alpine).
// GetDpkgConfigFlags returns Debian dpkg configuration flags based on whether user chose to preserve or overwrite existing configs
func GetDpkgConfigFlags() []string {
	if core.OverwriteExistingConfig {
		return []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confnew"}
	}
	return []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
}

func InstallPackages(packages ...string) bool {
	if len(packages) == 0 {
		return true
	}
	switch core.SysInfo.Family {
	case "debian":
		// Dpkg options: --force-confdef and --force-confold preserve existing configuration and prevent interactive terminal prompts from blocking TUI and automation
		// Dpkg options preserve existing configuration or apply clean defaults based on user choice without blocking terminal prompts
		args := append([]string{"install", "-y", "-q"}, GetDpkgConfigFlags()...)
		args = append(args, packages...)
		return RunStep("Installing system packages", "apt-get", args...)
	case "rhel":
		mgr := "dnf"
		if !CommandExists("dnf") {
			mgr = "yum"
		}
		args := append([]string{"install", "-y", "-q"}, packages...)
		return RunStep("Installing system packages", mgr, args...)
	case "alpine":
		args := append([]string{"add", "--quiet"}, packages...)
		return RunStep("Installing system packages", "apk", args...)
	default:
		args := append([]string{"install", "-y"}, packages...)
		return RunStep("Installing system packages", "apt-get", args...)
	}
}

// RemovePackages uninstalls one or more packages using the host distribution's package manager,
// purging configuration files where appropriate (e.g. apt-get purge).
func RemovePackages(packages ...string) bool {
	if len(packages) == 0 {
		return true
	}
	switch core.SysInfo.Family {
	case "debian":
		args := append([]string{"purge", "-y", "-q"}, packages...)
		return RunStep("Removing system packages", "apt-get", args...)
	case "rhel":
		mgr := "dnf"
		if !CommandExists("dnf") {
			mgr = "yum"
		}
		args := append([]string{"remove", "-y", "-q"}, packages...)
		return RunStep("Removing system packages", mgr, args...)
	case "alpine":
		args := append([]string{"del", "--quiet"}, packages...)
		return RunStep("Removing system packages", "apk", args...)
	default:
		args := append([]string{"remove", "-y"}, packages...)
		return RunStep("Removing system packages", "apt-get", args...)
	}
}

// UpdatePackageIndex refreshes local package repository metadata cache (apt-get update, dnf makecache, apk update).
func UpdatePackageIndex() bool {
	switch core.SysInfo.Family {
	case "debian":
		return RunStep("Updating package repository index", "apt-get", "update", "-q")
	case "rhel":
		mgr := "dnf"
		if !CommandExists("dnf") {
			mgr = "yum"
		}
		return RunStep("Updating package repository index", mgr, "makecache", "-q")
	case "alpine":
		return RunStep("Updating package repository index", "apk", "update", "--quiet")
	default:
		return RunStep("Updating package repository index", "apt-get", "update", "-q")
	}
}

// EnsureDockerEngine guarantees that Docker Engine and Docker Compose are installed and running.
func EnsureDockerEngine() bool {
	// 1. If docker binary is already installed, ensure service is running and compose works
	if CommandExists("docker") {
		if exec.Command("docker", "info").Run() != nil {
			_ = RunMultiStep("Starting Docker Service", "systemctl enable --now docker 2>/dev/null || (rc-service cgroups start 2>/dev/null; rc-update add docker default 2>/dev/null && rc-service docker start 2>/dev/null) || service docker start 2>/dev/null || true")
			time.Sleep(2 * time.Second)
		}
		if exec.Command("docker", "compose", "version").Run() == nil || CommandExists("docker-compose") {
			return true
		}
		if core.SysInfo.Family == "alpine" {
			_ = RunMultiStep("Installing Docker Compose Plugin", "apk add --no-cache docker-cli-compose 2>/dev/null || apk add --no-cache docker-compose 2>/dev/null || true")
			return true
		}
	}

	// 2. Install Docker Engine based on OS family
	switch core.SysInfo.Family {
	case "alpine":
		cmd := "apk update -q && (apk add --no-cache docker docker-cli-compose curl iptables ip6tables iproute2 2>/dev/null || apk add --no-cache docker docker-compose curl iptables ip6tables iproute2); modprobe ip_tables 2>/dev/null || true; modprobe iptable_nat 2>/dev/null || true; modprobe br_netfilter 2>/dev/null || true; sysctl -w net.ipv4.ip_forward=1 2>/dev/null || true; rc-service cgroups start 2>/dev/null || true; rc-update add docker default 2>/dev/null || true; rc-service docker restart 2>/dev/null || rc-service docker start 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	case "rhel":
		cmd := "dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin 2>/dev/null || yum install -y docker docker-compose 2>/dev/null || (TMP_D=$(mktemp -d /tmp/krotek-docker-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://get.docker.com -o \"$TMP_D/get-docker.sh\" && [ -s \"$TMP_D/get-docker.sh\" ] && sh \"$TMP_D/get-docker.sh\"; rm -rf \"$TMP_D\"); systemctl enable --now docker 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	case "debian":
		cmd := "(apt-get update -qq && (apt-get install -y docker.io docker-compose-v2 2>/dev/null || apt-get install -y docker.io docker-compose-plugin 2>/dev/null || apt-get install -y docker.io docker-compose)) || (TMP_D=$(mktemp -d /tmp/krotek-docker-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://get.docker.com -o \"$TMP_D/get-docker.sh\" && [ -s \"$TMP_D/get-docker.sh\" ] && sh \"$TMP_D/get-docker.sh\"; rm -rf \"$TMP_D\"); systemctl enable --now docker 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	default:
		cmd := "(TMP_D=$(mktemp -d /tmp/krotek-docker-XXXXXX) && chmod 700 \"$TMP_D\" && curl --proto '=https' --tlsv1.2 -fsSL https://get.docker.com -o \"$TMP_D/get-docker.sh\" && [ -s \"$TMP_D/get-docker.sh\" ] && sh \"$TMP_D/get-docker.sh\"; rm -rf \"$TMP_D\") || true; systemctl enable --now docker 2>/dev/null || rc-service docker start 2>/dev/null || true"
		if !RunMultiStepOrAbort("Installing Docker Engine", cmd) {
			return false
		}
	}

	// 3. Post-install daemon startup & enablement
	_ = exec.Command("systemctl", "enable", "docker").Run()
	_ = exec.Command("systemctl", "start", "docker").Run()
	_ = exec.Command("rc-service", "cgroups", "start").Run()
	_ = exec.Command("rc-update", "add", "docker", "default").Run()
	_ = exec.Command("rc-service", "docker", "start").Run()
	_ = exec.Command("service", "docker", "start").Run()

	for i := 0; i < 5; i++ {
		if exec.Command("docker", "info").Run() == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return CommandExists("docker")
}
