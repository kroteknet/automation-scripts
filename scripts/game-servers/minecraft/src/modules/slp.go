// ==============================================================================
// KROTEK SCRIPT HUB — SERVER LIST PING & QUERY PROTOCOL ENGINE
// Service: MINECRAFT (game-servers)
// File:    game-servers/minecraft/src/modules/slp.go
//
// Implements the network handshake protocol to query server status, player counts, latency, and MOTD strings.
// ==============================================================================

package modules

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"krotek-minecraft-installer/core"
)

type SLPResponse struct {
	Version struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
	} `json:"version"`
	Players struct {
		Max    int `json:"max"`
		Online int `json:"online"`
		Sample []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"sample"`
	} `json:"players"`
	Description interface{} `json:"description"`
	LatencyMS   int64       `json:"-"`
}

func writeVarInt(buf *bytes.Buffer, value int) {
	// #nosec G115 - Minecraft VarInt wire protocol requires 32-bit uint truncation
	uval := uint32(value)
	for {
		if (uval & ^uint32(0x7F)) == 0 {
			buf.WriteByte(byte(uval))
			return
		}
		buf.WriteByte(byte((uval & 0x7F) | 0x80))
		uval >>= 7
	}
}

func readVarInt(r io.Reader) (int, error) {
	var value int
	var shift uint
	b := make([]byte, 1)
	for {
		if _, err := r.Read(b); err != nil {
			return 0, err
		}
		value |= int(b[0]&0x7F) << shift
		if (b[0] & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 32 {
			return 0, fmt.Errorf("VarInt is too big")
		}
	}
	return value, nil
}

func writeString(buf *bytes.Buffer, s string) {
	writeVarInt(buf, len(s))
	buf.WriteString(s)
}

func readString(r io.Reader) (string, error) {
	length, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || length > 327670 {
		return "", fmt.Errorf("string length out of bounds: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	return string(data), nil
}

func QueryServerListPing(host, portStr string) (*SLPResponse, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	if portStr == "" {
		portStr = core.DefaultServerPort
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		port = core.DefaultServerPortInt
	}

	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, portStr), 1500*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	// Step 1: Handshake packet
	handshakeBuf := new(bytes.Buffer)
	writeVarInt(handshakeBuf, 0x00) // Packet ID 0: Handshake
	writeVarInt(handshakeBuf, 767)  // Protocol version
	writeString(handshakeBuf, host) // Server address
	// #nosec G115 - Port is clamped to network uint16 range
	_ = binary.Write(handshakeBuf, binary.BigEndian, uint16(port)) // Port
	writeVarInt(handshakeBuf, 1)                                   // Next state: 1 (Status)

	packetBuf := new(bytes.Buffer)
	writeVarInt(packetBuf, handshakeBuf.Len())
	packetBuf.Write(handshakeBuf.Bytes())

	if _, err := conn.Write(packetBuf.Bytes()); err != nil {
		return nil, err
	}

	// Step 2: Status Request packet
	statusReqBuf := new(bytes.Buffer)
	writeVarInt(statusReqBuf, 1)    // Length 1
	writeVarInt(statusReqBuf, 0x00) // Packet ID 0: Status Request
	if _, err := conn.Write(statusReqBuf.Bytes()); err != nil {
		return nil, err
	}

	// Step 3: Read Status Response packet
	_, err = readVarInt(conn) // Packet length
	if err != nil {
		return nil, err
	}

	packetID, err := readVarInt(conn)
	if err != nil || packetID != 0x00 {
		return nil, fmt.Errorf("unexpected SLP response packet ID: %d", packetID)
	}

	jsonStr, err := readString(conn)
	if err != nil {
		return nil, err
	}

	latency := time.Since(start).Milliseconds()

	var slp SLPResponse
	if err := json.Unmarshal([]byte(jsonStr), &slp); err != nil {
		return nil, fmt.Errorf("failed to parse SLP JSON: %w", err)
	}
	slp.LatencyMS = latency

	return &slp, nil
}

func FormatPlayerList(slp *SLPResponse) string {
	if slp == nil {
		return "Offline / Not Responding"
	}
	if slp.Players.Online == 0 {
		return fmt.Sprintf("0 / %d players online", slp.Players.Max)
	}

	var names []string
	for _, p := range slp.Players.Sample {
		if p.Name != "" {
			names = append(names, p.Name)
		}
	}

	if len(names) > 0 {
		return fmt.Sprintf("%d / %d players [%s]", slp.Players.Online, slp.Players.Max, strings.Join(names, ", "))
	}
	return fmt.Sprintf("%d / %d players online", slp.Players.Online, slp.Players.Max)
}
