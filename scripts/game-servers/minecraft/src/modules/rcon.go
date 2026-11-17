// ==============================================================================
// KROTEK SCRIPT HUB — RCON REMOTE CONSOLE PROTOCOL CLIENT
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/rcon.go
//
// Implements the binary RCON protocol to send administrative commands to dedicated game servers.
// ==============================================================================

package modules

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
	"krotek-minecraft-installer/system"
	"krotek-minecraft-installer/tui"
)

const (
	RconPacketAuth         int32 = 3
	RconPacketExecCommand  int32 = 2
	RconPacketResponse     int32 = 0
	RconPacketAuthResponse int32 = 2
)

type RconClient struct {
	conn  net.Conn
	reqID int32
}

func generateSecureTokenFromReader(r io.Reader, length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid token length: %d", length)
	}
	if r == nil {
		return "", fmt.Errorf("entropy reader must not be nil")
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		idx, err := rand.Int(r, max)
		if err != nil || idx == nil {
			var oneByte [1]byte
			if _, rErr := io.ReadFull(r, oneByte[:]); rErr == nil {
				b[i] = charset[int(oneByte[0])%len(charset)]
				continue
			}
			return "", fmt.Errorf("cryptographic randomness unavailable: %w", err)
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b), nil
}

func generateSecureToken(length int) (string, error) {
	return generateSecureTokenFromReader(rand.Reader, length)
}

// EnsureRconConfigured validates that RCON remote console is enabled in server.properties with a secure password.
func EnsureRconConfigured() (port string, password string, err error) {
	serverDir := getServerDataDir()
	propFile := filepath.Join(serverDir, "server.properties")

	data, err := os.ReadFile(propFile)
	if err != nil {
		return "", "", fmt.Errorf("could not read server.properties: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	props := make(map[string]string)
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			parts := strings.SplitN(l, "=", 2)
			if len(parts) == 2 {
				props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	enabled := props["enable-rcon"] == "true"
	port = props["rcon.port"]
	if port == "" {
		port = "25575"
	}
	password = props["rcon.password"]

	if !enabled || password == "" {
		if password == "" {
			tok, tokErr := generateSecureToken(24)
			if tokErr != nil {
				return "", "", tokErr
			}
			password = tok
		}
		updateServerProp(propFile, "enable-rcon", "true")
		updateServerProp(propFile, "rcon.ip", "127.0.0.1")
		updateServerProp(propFile, "rcon.port", port)
		updateServerProp(propFile, "rcon.password", password)
		_ = os.Chmod(propFile, 0600)

		// Prompt user to restart service if currently running
		if system.IsServiceInstalled() {
			_ = exec.Command("systemctl", "restart", "minecraft").Run()
			time.Sleep(2 * time.Second)
		}
	}

	return port, password, nil
}

// DialRcon establishes a TCP socket connection to the game server RCON port and executes authentication handshake.
func DialRcon(address, password string) (*RconClient, error) {
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not connect to RCON at %s: %w", address, err)
	}

	c := &RconClient{conn: conn, reqID: 1}
	if err := c.auth(password); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *RconClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *RconClient) sendPacket(packetType int32, payload string) (int32, error) {
	c.reqID++
	reqID := c.reqID

	payloadBytes := []byte(payload)
	// #nosec G115 - RCON protocol packet length is within standard payload bounds
	length := int32(4 + 4 + len(payloadBytes) + 2)

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, length)
	_ = binary.Write(buf, binary.LittleEndian, reqID)
	_ = binary.Write(buf, binary.LittleEndian, packetType)
	buf.Write(payloadBytes)
	buf.Write([]byte{0x00, 0x00})

	_ = c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return reqID, nil
}

func (c *RconClient) readPacket() (reqID int32, packetType int32, payload string, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(4 * time.Second))

	var length int32
	if err = binary.Read(c.conn, binary.LittleEndian, &length); err != nil {
		return 0, 0, "", err
	}

	if length < 10 || length > 40960 {
		return 0, 0, "", fmt.Errorf("invalid RCON packet length: %d", length)
	}

	body := make([]byte, length)
	if _, err = io.ReadFull(c.conn, body); err != nil {
		return 0, 0, "", err
	}

	buf := bytes.NewReader(body)
	_ = binary.Read(buf, binary.LittleEndian, &reqID)
	_ = binary.Read(buf, binary.LittleEndian, &packetType)

	payloadBytes := body[8 : len(body)-2]
	return reqID, packetType, string(payloadBytes), nil
}

func (c *RconClient) auth(password string) error {
	reqID, err := c.sendPacket(RconPacketAuth, password)
	if err != nil {
		return fmt.Errorf("error sending RCON auth: %w", err)
	}

	respID, _, _, err := c.readPacket()
	if err != nil {
		return fmt.Errorf("error receiving RCON auth response: %w", err)
	}

	if respID == -1 || respID != reqID {
		return fmt.Errorf("RCON authentication failed: invalid password")
	}
	return nil
}

func (c *RconClient) Execute(cmd string) (string, error) {
	reqID, err := c.sendPacket(RconPacketExecCommand, cmd)
	if err != nil {
		return "", err
	}

	respID, _, payload, err := c.readPacket()
	if err != nil {
		return "", err
	}

	if respID != reqID {
		return "", fmt.Errorf("RCON response ID mismatch")
	}
	return strings.TrimSpace(payload), nil
}

// ExecuteConsoleCommand executes a single command on the game server via RCON and returns the console response.
func ExecuteConsoleCommand(cmdStr string) (string, error) {
	port, pass, err := EnsureRconConfigured()
	if err != nil {
		return "", err
	}

	rcon, err := DialRcon("127.0.0.1:"+port, pass)
	if err != nil {
		return "", err
	}
	defer rcon.Close()

	return rcon.Execute(cmdStr)
}

// InteractiveConsoleCLI launches an interactive live terminal prompt to send commands directly to the Minecraft server console.
func InteractiveConsoleCLI() {
	tui.ClearScreen()
	port, pass, err := EnsureRconConfigured()
	if err != nil {
		tui.PrintInBox("RCON Error", []string{core.RED + err.Error() + core.NC})
		return
	}

	tui.StartSpinner("Connecting to Minecraft Server Console (RCON)...")
	rcon, err := DialRcon("127.0.0.1:"+port, pass)
	tui.StopSpinner(0)

	if err != nil {
		tui.PrintInBox("RCON Connection Failed", []string{
			core.RED + "[ERROR] Could not connect to RCON interface on 127.0.0.1:" + port + core.NC,
			err.Error(),
			"",
			"Ensure Minecraft server is running (`minecraft-manager start`) and RCON is enabled.",
		})
		return
	}
	defer rcon.Close()

	headerLines := []string{
		core.BOLD + core.GREEN + "=== MINECRAFT IN-GAME INTERACTIVE CONSOLE (RCON) ===" + core.NC,
		fmt.Sprintf("Connected to: %s127.0.0.1:%s%s (Secure Local Pipe)", core.CYAN, port, core.NC),
		"",
		core.YELLOW + "Common Commands:" + core.NC,
		"  " + core.Sym.Pointer + " " + core.BOLD + "say <message>" + core.NC + "  - Broadcast in-game announcement",
		"  " + core.Sym.Pointer + " " + core.BOLD + "gamemode <mode> <player>" + core.NC + "  - Change player game mode",
		"  " + core.Sym.Pointer + " " + core.BOLD + "weather <clear|rain|thunder>" + core.NC + "  - Change weather",
		"  " + core.Sym.Pointer + " " + core.BOLD + "time set <day|night|1000>" + core.NC + "  - Change world time",
		"  " + core.Sym.Pointer + " " + core.BOLD + "save-all" + core.NC + "  - Flush chunks to disk",
		"  " + core.Sym.Pointer + " " + core.BOLD + "list" + core.NC + "  - Show online players",
		"",
		core.DIM + "Type your command below, or leave empty/press [Esc] to exit console." + core.NC,
	}
	tui.PrintInBox("Minecraft Console Prompt", headerLines)

	for {
		cmdInput, st := tui.ReadTextInput("mc-console >", "")
		if st != 0 || strings.TrimSpace(cmdInput) == "" || strings.EqualFold(cmdInput, "exit") || strings.EqualFold(cmdInput, "quit") {
			break
		}

		cleanCmd := strings.TrimPrefix(strings.TrimSpace(cmdInput), "/")
		response, rErr := rcon.Execute(cleanCmd)
		if rErr != nil {
			fmt.Printf("  %s[ERROR]%s %s\r\n", core.RED, core.NC, rErr.Error())
		} else if response != "" {
			fmt.Printf("  %s[SERVER]%s %s\r\n", core.GREEN, core.NC, response)
		} else {
			fmt.Printf("  %s[OK]%s Command executed successfully.\r\n", core.GREEN, core.NC)
		}
	}
}
