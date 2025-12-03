package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	defaultPreludeCap = 512
	defaultTLSCap     = 4096
	maxPreludeCap     = 8192
	maxTLSCap         = 65536
)

type initialBuffers struct {
	prelude    []byte
	tlsInitial []byte
}

var (
	initialBufPool = sync.Pool{
		New: func() any {
			return &initialBuffers{
				prelude:    make([]byte, 0, defaultPreludeCap),
				tlsInitial: make([]byte, 0, defaultTLSCap),
			}
		},
	}
	readerPool = sync.Pool{
		New: func() any {
			return bufio.NewReaderSize(nil, 4096)
		},
	}
)

func getInitialBuffers() *initialBuffers {
	bufs := initialBufPool.Get().(*initialBuffers)
	bufs.prelude = bufs.prelude[:0]
	bufs.tlsInitial = bufs.tlsInitial[:0]
	return bufs
}

func putInitialBuffers(bufs *initialBuffers) {
	if bufs == nil {
		return
	}
	if cap(bufs.prelude) > maxPreludeCap {
		bufs.prelude = make([]byte, 0, defaultPreludeCap)
	} else {
		bufs.prelude = bufs.prelude[:0]
	}
	if cap(bufs.tlsInitial) > maxTLSCap {
		bufs.tlsInitial = make([]byte, 0, defaultTLSCap)
	} else {
		bufs.tlsInitial = bufs.tlsInitial[:0]
	}
	initialBufPool.Put(bufs)
}

func getReader(conn net.Conn) *bufio.Reader {
	br := readerPool.Get().(*bufio.Reader)
	br.Reset(conn)
	return br
}

func putReader(br *bufio.Reader) {
	if br == nil {
		return
	}
	br.Reset(nil)
	readerPool.Put(br)
}

// extractSNI reads the initial bytes (handling PROXY headers and PostgreSQL SSLRequest) and returns
// the parsed SNI plus the bytes that must be replayed to the backend.
func extractSNI(conn net.Conn) (string, *initialBuffers, bool, error) {
	reader := getReader(conn)
	defer putReader(reader)
	bufs := getInitialBuffers() // holds prelude + TLS bytes to replay

	if err := maybeConsumeProxyHeader(reader, &bufs.prelude); err != nil {
		return "", bufs, false, err
	}

	sawPGSSLRequest, err := maybeHandlePostgresSSLRequest(reader, &bufs.prelude, conn)
	if err != nil {
		return "", bufs, sawPGSSLRequest, err
	}

	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", bufs, sawPGSSLRequest, fmt.Errorf("reading TLS header: %w", err)
	}
	bufs.tlsInitial = append(bufs.tlsInitial, header...)

	if header[0] != 0x16 { // TLS Handshake
		return "", bufs, sawPGSSLRequest, errors.New("not a TLS handshake record")
	}

	length := int(header[3])<<8 | int(header[4])
	if length <= 0 || length > 1<<15 {
		return "", bufs, sawPGSSLRequest, fmt.Errorf("invalid TLS record length %d", length)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return "", bufs, sawPGSSLRequest, fmt.Errorf("reading TLS body: %w", err)
	}
	bufs.tlsInitial = append(bufs.tlsInitial, body...)

	sni, err := parseClientHelloForSNI(body)
	if err != nil {
		return "", bufs, sawPGSSLRequest, err
	}
	if sni == "" {
		return "", bufs, sawPGSSLRequest, errors.New("no SNI present")
	}

	// Preserve any bytes bufio.Reader has already pulled from the socket so the backend sees an unbroken stream.
	if buffered := reader.Buffered(); buffered > 0 {
		extra := make([]byte, buffered)
		if _, err := io.ReadFull(reader, extra); err == nil {
			bufs.tlsInitial = append(bufs.tlsInitial, extra...)
		}
	}

	return sni, bufs, sawPGSSLRequest, nil
}

// maybeHandlePostgresSSLRequest consumes a PostgreSQL SSLRequest prefix (if present) and sends the acceptance byte.
func maybeHandlePostgresSSLRequest(r *bufio.Reader, consumed *[]byte, conn net.Conn) (bool, error) {
	const sslRequestLen = 8

	peek, err := r.Peek(sslRequestLen)
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) || errors.Is(err, io.EOF) {
			return false, nil
		}
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			// Let the caller hit the TLS read timeout instead.
			return false, nil
		}
		return false, fmt.Errorf("peek postgres SSLRequest: %w", err)
	}
	if len(peek) < sslRequestLen {
		return false, nil
	}

	length := binary.BigEndian.Uint32(peek[0:4])
	magic := binary.BigEndian.Uint32(peek[4:8])
	if length != 8 || magic != 80877103 {
		return false, nil
	}

	log.Printf("PostgreSQL SSLRequest detected; responding with acceptance")
	req := make([]byte, sslRequestLen)
	if _, err := io.ReadFull(r, req); err != nil {
		return true, fmt.Errorf("read postgres SSLRequest: %w", err)
	}
	*consumed = append(*consumed, req...)

	if _, err := conn.Write([]byte{'S'}); err != nil {
		return true, fmt.Errorf("write postgres SSL response: %w", err)
	}
	// Give the client a fresh window to send the subsequent TLS ClientHello.
	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	return true, nil
}

// consumeBackendPostgresSSLResponse reads the backend's single-byte SSL response so we can inject it before TLS bytes.
func consumeBackendPostgresSSLResponse(conn net.Conn) ([]byte, error) {
	var buf [1]byte

	_ = conn.SetReadDeadline(time.Now().Add(readHelloTimeout))
	n, err := conn.Read(buf[:])
	_ = conn.SetReadDeadline(time.Time{})

	if n == 0 {
		return nil, err
	}
	if buf[0] == 'S' {
		log.Printf("Backend Postgres SSL response: accepted TLS (S)")
		return nil, err
	}

	log.Printf("Backend Postgres first byte after SSLRequest: 0x%02x (%q)", buf[0], buf[0])
	return buf[:1], err
}

// maybeConsumeProxyHeader consumes PROXY protocol v1/v2 headers if present.
func maybeConsumeProxyHeader(r *bufio.Reader, consumed *[]byte) error {
	const proxyV2Len = 12
	sig, err := r.Peek(proxyV2Len)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			// Timed out waiting for data; proceed so TLS read reports the timeout instead.
			return nil
		}
		return fmt.Errorf("peek proxy header: %w", err)
	}
	// PROXY protocol v1 (text)
	if bytes.HasPrefix(sig, []byte("PROXY ")) {
		line, err := r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read proxy v1 header: %w", err)
		}
		if len(line) > 107 { // spec limit plus CRLF
			return errors.New("proxy v1 header too long")
		}
		*consumed = append(*consumed, line...)
		return nil
	}

	// PROXY protocol v2 (binary)
	proxyV2Sig := []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a}
	if len(sig) >= proxyV2Len && bytes.Equal(sig[:proxyV2Len], proxyV2Sig) {
		hdr := make([]byte, 16)
		if _, err := io.ReadFull(r, hdr); err != nil {
			return fmt.Errorf("read proxy v2 header: %w", err)
		}
		*consumed = append(*consumed, hdr...)
		addrLen := int(binary.BigEndian.Uint16(hdr[14:16]))
		if addrLen > 0 {
			addr := make([]byte, addrLen)
			if _, err := io.ReadFull(r, addr); err != nil {
				return fmt.Errorf("read proxy v2 address block: %w", err)
			}
			*consumed = append(*consumed, addr...)
		}
		return nil
	}
	return nil
}

// parseClientHelloForSNI extracts the SNI from a TLS ClientHello record payload.
func parseClientHelloForSNI(record []byte) (string, error) {
	if len(record) < 4 {
		return "", errors.New("TLS record too short for handshake")
	}
	if record[0] != 0x01 {
		return "", errors.New("first handshake message is not ClientHello")
	}

	handshakeLen := int(record[1])<<16 | int(record[2])<<8 | int(record[3])
	if handshakeLen+4 > len(record) {
		return "", errors.New("truncated ClientHello")
	}
	data := record[4 : 4+handshakeLen]
	offset := 0

	if len(data) < 34 {
		return "", errors.New("ClientHello too short")
	}
	offset += 2  // version
	offset += 32 // random

	if offset >= len(data) {
		return "", errors.New("malformed ClientHello (session id length missing)")
	}
	sidLen := int(data[offset])
	offset++
	if offset+sidLen > len(data) {
		return "", errors.New("malformed ClientHello (session id)")
	}
	offset += sidLen

	if offset+2 > len(data) {
		return "", errors.New("malformed ClientHello (cipher suites length)")
	}
	csLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	if offset+csLen > len(data) {
		return "", errors.New("malformed ClientHello (cipher suites)")
	}
	offset += csLen

	if offset >= len(data) {
		return "", errors.New("malformed ClientHello (compression length)")
	}
	compLen := int(data[offset])
	offset++
	if offset+compLen > len(data) {
		return "", errors.New("malformed ClientHello (compression methods)")
	}
	offset += compLen

	if offset+2 > len(data) {
		return "", errors.New("ClientHello missing extensions length")
	}
	extLen := int(data[offset])<<8 | int(data[offset+1])
	offset += 2
	if offset+extLen > len(data) {
		return "", errors.New("ClientHello extensions truncated")
	}
	exts := data[offset : offset+extLen]

	for len(exts) >= 4 {
		extType := int(exts[0])<<8 | int(exts[1])
		extDataLen := int(exts[2])<<8 | int(exts[3])
		exts = exts[4:]
		if extDataLen > len(exts) {
			return "", errors.New("extension length overflow")
		}
		extData := exts[:extDataLen]
		exts = exts[extDataLen:]

		if extType == 0 { // server_name
			if len(extData) < 2 {
				return "", errors.New("SNI extension too short")
			}
			listLen := int(extData[0])<<8 | int(extData[1])
			if listLen+2 > len(extData) {
				return "", errors.New("SNI list length invalid")
			}
			names := extData[2 : 2+listLen]
			for len(names) >= 3 {
				nameType := names[0]
				nameLen := int(names[1])<<8 | int(names[2])
				names = names[3:]
				if nameLen > len(names) {
					return "", errors.New("SNI name length invalid")
				}
				name := string(names[:nameLen])
				names = names[nameLen:]
				if nameType == 0 {
					return name, nil
				}
			}
			return "", errors.New("SNI extension present but no host name found")
		}
	}

	return "", errors.New("SNI not found in ClientHello")
}
