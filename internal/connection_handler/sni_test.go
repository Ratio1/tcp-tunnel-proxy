package connectionhandler

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseClientHelloForSNI(t *testing.T) {
	host := "db.ratio1.link"
	record := buildClientHelloRecord(host, true)

	got, err := parseClientHelloForSNI(record)
	if err != nil {
		t.Fatalf("parseClientHelloForSNI returned error: %v", err)
	}
	if got != host {
		t.Fatalf("parseClientHelloForSNI = %q, want %q", got, host)
	}
}

func TestParseClientHelloForSNIMissing(t *testing.T) {
	record := buildClientHelloRecord("ignored", false)

	if _, err := parseClientHelloForSNI(record); err == nil {
		t.Fatalf("parseClientHelloForSNI unexpectedly succeeded without SNI")
	}
}

func TestMaybeConsumeProxyHeaderVariants(t *testing.T) {
	var consumed []byte
	proxyLine := "PROXY TCP4 1.1.1.1 2.2.2.2 1234 80\r\n"
	reader := bufio.NewReader(strings.NewReader(proxyLine + "rest"))
	if err := maybeConsumeProxyHeader(reader, &consumed); err != nil {
		t.Fatalf("maybeConsumeProxyHeader v1 error: %v", err)
	}
	if string(consumed) != proxyLine {
		t.Fatalf("proxy v1 consumed=%q, want %q", string(consumed), proxyLine)
	}

	consumed = consumed[:0]
	v2hdr := buildProxyV2Header()
	reader = bufio.NewReader(bytes.NewReader(append(v2hdr, []byte("payload")...)))
	if err := maybeConsumeProxyHeader(reader, &consumed); err != nil {
		t.Fatalf("maybeConsumeProxyHeader v2 error: %v", err)
	}
	if got := consumed; !bytes.Equal(got, v2hdr) {
		t.Fatalf("proxy v2 consumed=%x, want %x", got, v2hdr)
	}

	consumed = consumed[:0]
	reader = bufio.NewReader(strings.NewReader("HELLO"))
	if err := maybeConsumeProxyHeader(reader, &consumed); err != nil {
		t.Fatalf("maybeConsumeProxyHeader none error: %v", err)
	}
	if len(consumed) != 0 {
		t.Fatalf("expected no bytes consumed without proxy header")
	}
}

func TestMaybeHandlePostgresSSLRequest(t *testing.T) {
	req := make([]byte, 8)
	binary.BigEndian.PutUint32(req[0:4], 8)
	binary.BigEndian.PutUint32(req[4:8], 80877103)

	conn := newMockConn(req)
	reader := bufio.NewReader(bytes.NewReader(req))
	var consumed []byte
	saw, err := maybeHandlePostgresSSLRequest(reader, &consumed, conn)
	if err != nil {
		t.Fatalf("maybeHandlePostgresSSLRequest error: %v", err)
	}
	if !saw {
		t.Fatalf("expected SSLRequest to be detected")
	}
	if string(consumed) != string(req) {
		t.Fatalf("consumed=%x, want %x", consumed, req)
	}
	if conn.writes.String() != "S" {
		t.Fatalf("expected acceptance byte written, got %q", conn.writes.String())
	}
}

func TestMaybeHandlePostgresSSLRequestIgnoresNonSSLRequest(t *testing.T) {
	data := []byte{0, 0, 0, 8, 1, 2, 3, 4}
	conn := newMockConn(data)
	reader := bufio.NewReader(bytes.NewReader(data))
	var consumed []byte
	saw, err := maybeHandlePostgresSSLRequest(reader, &consumed, conn)
	if err != nil {
		t.Fatalf("maybeHandlePostgresSSLRequest error: %v", err)
	}
	if saw {
		t.Fatalf("unexpected SSLRequest detection")
	}
	if conn.writes.Len() != 0 {
		t.Fatalf("unexpected writes for non-SSLRequest")
	}
}

func TestConsumeBackendPostgresSSLResponse(t *testing.T) {
	acceptConn := newMockConn([]byte("S"))
	prefix, err := consumeBackendPostgresSSLResponse(acceptConn)
	if err != nil && err != io.EOF {
		t.Fatalf("consumeBackendPostgresSSLResponse accept error: %v", err)
	}
	if len(prefix) != 0 {
		t.Fatalf("expected no prefix for acceptance, got %x", prefix)
	}

	rejectConn := newMockConn([]byte("N"))
	prefix, err = consumeBackendPostgresSSLResponse(rejectConn)
	if err != nil && err != io.EOF {
		t.Fatalf("consumeBackendPostgresSSLResponse reject error: %v", err)
	}
	if !bytes.Equal(prefix, []byte("N")) {
		t.Fatalf("expected rejection byte to be replayed, got %x", prefix)
	}
}

func buildClientHelloRecord(host string, includeSNI bool) []byte {
	var body bytes.Buffer
	body.Write([]byte{0x03, 0x03})             // version
	body.Write(bytes.Repeat([]byte{0x01}, 32)) // random
	body.WriteByte(0x00)                       // session id len
	body.Write([]byte{0x00, 0x02, 0x13, 0x01}) // cipher suites len + single suite
	body.Write([]byte{0x01, 0x00})             // compression methods (len=1, null)

	if includeSNI {
		name := []byte(host)
		sniListLen := 3 + len(name)
		extDataLen := 2 + sniListLen

		var ext bytes.Buffer
		ext.Write([]byte{0x00, 0x00})                              // extension type server_name
		ext.Write([]byte{byte(extDataLen >> 8), byte(extDataLen)}) // ext data len
		ext.Write([]byte{byte(sniListLen >> 8), byte(sniListLen)}) // server name list len
		ext.WriteByte(0x00)                                        // host_name type
		ext.Write([]byte{byte(len(name) >> 8), byte(len(name))})
		ext.Write(name)

		extBytes := ext.Bytes()
		body.Write([]byte{byte(len(extBytes) >> 8), byte(len(extBytes))})
		body.Write(extBytes)
	} else {
		body.Write([]byte{0x00, 0x00}) // extensions length zero
	}

	handshakeLen := body.Len()
	record := make([]byte, 4+handshakeLen)
	record[0] = 0x01
	record[1] = byte(handshakeLen >> 16)
	record[2] = byte(handshakeLen >> 8)
	record[3] = byte(handshakeLen)
	copy(record[4:], body.Bytes())
	return record
}

func buildProxyV2Header() []byte {
	sig := []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a}
	header := make([]byte, 16)
	copy(header, sig)
	header[12] = 0x20 // ver/cmd: 2<<4 | 0x0 (local) to avoid addr block
	header[13] = 0x00 // fam/proto: UNSPEC
	header[14] = 0x00
	header[15] = 0x00 // len=0
	return header
}

type mockConn struct {
	r      *bytes.Reader
	writes bytes.Buffer
}

func newMockConn(data []byte) *mockConn {
	return &mockConn{r: bytes.NewReader(data)}
}

func (m *mockConn) Read(b []byte) (int, error)       { return m.r.Read(b) }
func (m *mockConn) Write(b []byte) (int, error)      { return m.writes.Write(b) }
func (m *mockConn) Close() error                     { return nil }
func (m *mockConn) LocalAddr() net.Addr              { return mockAddr("local") }
func (m *mockConn) RemoteAddr() net.Addr             { return mockAddr("remote") }
func (m *mockConn) SetDeadline(time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(time.Time) error { return nil }

type mockAddr string

func (m mockAddr) Network() string { return string(m) }
func (m mockAddr) String() string  { return string(m) }
