package device

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

// WakeUpViaUDP sends a TR-111 UDP Connection Request (STUN Binding Request) to the CPE.
// It requires the target IP:port and the ConnectionRequest username/password.
func WakeUpViaUDP(address string, username string, password string) error {
	conn, err := net.DialTimeout("udp", address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to dial udp: %v", err)
	}
	defer conn.Close()

	// Build STUN Binding Request (RFC 3489)
	// Message Type: 0x0001 (Binding Request)
	// Message Length: calculated after attributes
	// Transaction ID: 128 bits (16 bytes) random

	txID := make([]byte, 16)
	rand.Read(txID)

	var attrs []byte

	// Attribute: USERNAME (Type 0x0006)
	if username != "" {
		attrs = appendAttribute(attrs, 0x0006, []byte(username))
	}

	// Attribute: MESSAGE-INTEGRITY (Type 0x0008)
	// According to TR-111, MESSAGE-INTEGRITY is required if auth is used.
	// It's HMAC-SHA1 of the STUN message (header + attributes up to this point) using the password.
	if password != "" {
		// Temporary header to calculate HMAC
		tempMsgLen := uint16(len(attrs) + 24) // +24 for MESSAGE-INTEGRITY attribute
		tempHeader := make([]byte, 20)
		binary.BigEndian.PutUint16(tempHeader[0:2], 0x0001)
		binary.BigEndian.PutUint16(tempHeader[2:4], tempMsgLen)
		copy(tempHeader[4:], txID)

		mac := hmac.New(sha1.New, []byte(password))
		mac.Write(tempHeader)
		mac.Write(attrs)
		sum := mac.Sum(nil)

		attrs = appendAttribute(attrs, 0x0008, sum)
	}

	// Final header
	msgLen := uint16(len(attrs))
	header := make([]byte, 20)
	binary.BigEndian.PutUint16(header[0:2], 0x0001)
	binary.BigEndian.PutUint16(header[2:4], msgLen)
	copy(header[4:], txID)

	// Combine header and attributes
	packet := append(header, attrs...)

	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	_, err = conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send udp packet: %v", err)
	}

	return nil
}

func appendAttribute(buf []byte, attrType uint16, value []byte) []byte {
	// Pad value to multiple of 4 bytes
	padding := (4 - (len(value) % 4)) % 4
	paddedValue := append(value, make([]byte, padding)...)

	attr := make([]byte, 4+len(paddedValue))
	binary.BigEndian.PutUint16(attr[0:2], attrType)
	binary.BigEndian.PutUint16(attr[2:4], uint16(len(value))) // length before padding
	copy(attr[4:], paddedValue)

	return append(buf, attr...)
}

// ExtractUDPAddress looks for the UDP connection request address in the Inform parameters
func ExtractUDPAddress(params map[string]string) string {
	for k, v := range params {
		if strings.HasSuffix(k, "ManagementServer.UDPConnectionRequestAddress") {
			return v
		}
	}
	return ""
}
