package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultHTTPClientTimeout = 30 * time.Second

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if c == nil || c.reader == nil {
		return 0, io.EOF
	}
	return c.reader.Read(p)
}

func proxyAwareHTTPClient(timeout time.Duration, proxyURL string) *http.Client {
	trimmedProxyURL := strings.TrimSpace(proxyURL)
	if timeout <= 0 {
		timeout = defaultHTTPClientTimeout
	}

	client := &http.Client{Timeout: timeout}
	if trimmedProxyURL == "" {
		return client
	}

	transport := proxyAwareTransport(timeout, trimmedProxyURL)
	if transport != nil {
		client.Transport = transport
	}
	return client
}

func proxyAwareTransport(timeout time.Duration, proxyURL string) *http.Transport {
	parsedProxyURL, err := url.Parse(strings.TrimSpace(proxyURL))
	if err != nil || parsedProxyURL == nil {
		return nil
	}

	dialer := &net.Dialer{Timeout: timeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = dialer.DialContext

	switch strings.ToLower(strings.TrimSpace(parsedProxyURL.Scheme)) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsedProxyURL)
	case "socks5", "socks5h":
		transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
			return dialSOCKS5Proxy(ctx, dialer, parsedProxyURL, network, address)
		}
	default:
		return nil
	}

	return transport
}

func dialProxyConnection(ctx context.Context, address string, timeout time.Duration, proxyURL string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	trimmedProxyURL := strings.TrimSpace(proxyURL)
	if trimmedProxyURL == "" {
		return dialer.DialContext(ctx, "tcp", address)
	}

	parsedProxyURL, err := url.Parse(trimmedProxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy url: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(parsedProxyURL.Scheme)) {
	case "http":
		return dialHTTPProxyTunnel(ctx, dialer, parsedProxyURL, address)
	case "socks5", "socks5h":
		return dialSOCKS5Proxy(ctx, dialer, parsedProxyURL, "tcp", address)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", strings.TrimSpace(parsedProxyURL.Scheme))
	}
}

func dialHTTPProxyTunnel(ctx context.Context, dialer *net.Dialer, proxyURL *url.URL, targetAddr string) (net.Conn, error) {
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddress(proxyURL))
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else if dialer.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(dialer.Timeout))
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetAddr},
		Host:   targetAddr,
		Header: make(http.Header),
	}
	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("write proxy connect request: %w", err)
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read proxy connect response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = conn.Close()
		return nil, fmt.Errorf("proxy connect failed: %s %s", resp.Status, strings.TrimSpace(string(message)))
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &bufferedConn{
		Conn:   conn,
		reader: reader,
	}, nil
}

func dialSOCKS5Proxy(ctx context.Context, dialer *net.Dialer, proxyURL *url.URL, network string, targetAddr string) (net.Conn, error) {
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddress(proxyURL))
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else if dialer.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(dialer.Timeout))
	}

	username := ""
	password := ""
	if proxyURL.User != nil {
		username = proxyURL.User.Username()
		password, _ = proxyURL.User.Password()
	}

	if err := writeSOCKS5Greeting(conn, username != "" || password != ""); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := readSOCKS5MethodSelection(conn, username, password); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := writeSOCKS5Connect(conn, network, targetAddr); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := readSOCKS5ConnectResponse(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func proxyAddress(proxyURL *url.URL) string {
	if proxyURL == nil {
		return ""
	}
	host := strings.TrimSpace(proxyURL.Host)
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}

	port := "80"
	if strings.EqualFold(strings.TrimSpace(proxyURL.Scheme), "https") {
		port = "443"
	}
	return net.JoinHostPort(host, port)
}

func writeSOCKS5Greeting(conn net.Conn, withAuth bool) error {
	methods := []byte{0x00}
	if withAuth {
		methods = append(methods, 0x02)
	}

	payload := []byte{0x05, byte(len(methods))}
	payload = append(payload, methods...)
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write socks5 greeting: %w", err)
	}
	return nil
}

func readSOCKS5MethodSelection(conn net.Conn, username string, password string) error {
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("read socks5 method selection: %w", err)
	}
	if reply[0] != 0x05 {
		return fmt.Errorf("unexpected socks5 version: %d", reply[0])
	}

	switch reply[1] {
	case 0x00:
		return nil
	case 0x02:
		return authenticateSOCKS5UserPass(conn, username, password)
	case 0xFF:
		return errors.New("socks5 proxy rejected all auth methods")
	default:
		return fmt.Errorf("unsupported socks5 auth method: %d", reply[1])
	}
}

func authenticateSOCKS5UserPass(conn net.Conn, username string, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return errors.New("socks5 proxy credentials are too long")
	}

	payload := []byte{0x01, byte(len(username))}
	payload = append(payload, username...)
	payload = append(payload, byte(len(password)))
	payload = append(payload, password...)
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write socks5 auth payload: %w", err)
	}

	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("read socks5 auth response: %w", err)
	}
	if reply[1] != 0x00 {
		return errors.New("socks5 proxy authentication failed")
	}
	return nil
}

func writeSOCKS5Connect(conn net.Conn, network string, targetAddr string) error {
	if !strings.EqualFold(strings.TrimSpace(network), "tcp") {
		return fmt.Errorf("socks5 proxy only supports tcp network, got %s", network)
	}

	host, portValue, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return fmt.Errorf("split socks5 target address: %w", err)
	}

	port, err := strconv.Atoi(portValue)
	if err != nil {
		return fmt.Errorf("parse socks5 target port: %w", err)
	}

	payload := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			payload = append(payload, 0x01)
			payload = append(payload, ipv4...)
		} else {
			payload = append(payload, 0x04)
			payload = append(payload, ip.To16()...)
		}
	} else {
		if len(host) > 255 {
			return errors.New("socks5 target host is too long")
		}
		payload = append(payload, 0x03, byte(len(host)))
		payload = append(payload, host...)
	}

	payload = append(payload, byte(port>>8), byte(port))
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write socks5 connect payload: %w", err)
	}
	return nil
}

func readSOCKS5ConnectResponse(conn net.Conn) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read socks5 connect response: %w", err)
	}
	if header[0] != 0x05 {
		return fmt.Errorf("unexpected socks5 connect version: %d", header[0])
	}
	if header[1] != 0x00 {
		return fmt.Errorf("socks5 connect failed with reply code %d", header[1])
	}

	addrLength := 0
	switch header[3] {
	case 0x01:
		addrLength = net.IPv4len
	case 0x04:
		addrLength = net.IPv6len
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return fmt.Errorf("read socks5 domain length: %w", err)
		}
		addrLength = int(length[0])
	default:
		return fmt.Errorf("unsupported socks5 bind address type: %d", header[3])
	}

	if addrLength > 0 {
		if _, err := io.CopyN(io.Discard, conn, int64(addrLength)); err != nil {
			return fmt.Errorf("read socks5 bind address: %w", err)
		}
	}
	if _, err := io.CopyN(io.Discard, conn, 2); err != nil {
		return fmt.Errorf("read socks5 bind port: %w", err)
	}
	return nil
}
