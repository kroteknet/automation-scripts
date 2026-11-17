// ==============================================================================
// KROTEK SCRIPT HUB — MULTI-DISTRO PACKAGE MANAGEMENT ABSTRACTION
// Service: OPENLITESPEED (web-server)
// File:    web-server/openlitespeed/src/system/packages.go
//
// Abstracts native package managers (apt-get, dnf/yum, apk) across Linux distributions, providing consistent APIs for installing, removing, updating, and querying software packages.
// ==============================================================================

package system

import (
	"os/exec"
	"time"

	"krotek-openlitespeed-installer/core"
)

// EnsureDockerEngine guarantees that Docker Engine and Docker Compose are installed and running.
// GetDpkgConfigFlags returns Debian dpkg configuration flags based on whether user chose to preserve or overwrite existing configs
func GetDpkgConfigFlags() []string {
	if core.OverwriteExistingConfig {
		return []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confnew"}
	}
	return []string{"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
}

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

// UpdatePackageIndex refreshes local package repository metadata cache (apt-get update, dnf makecache, apk update).
func UpdatePackageIndex() bool {
	switch core.SysInfo.Family {
	case "alpine":
		return exec.Command("apk", "update", "--quiet").Run() == nil
	case "rhel":
		mgr := "dnf"
		if !CommandExists("dnf") {
			mgr = "yum"
		}
		return exec.Command(mgr, "makecache", "-q").Run() == nil
	default:
		return exec.Command("apt-get", "update", "-qq").Run() == nil
	}
}
