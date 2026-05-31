package i2psam

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	DefaultAddress        = "127.0.0.1:7656"
	DefaultTimeout        = 30 * time.Second
	DefaultTunnelLength   = 1
	DefaultTunnelQuantity = 3
	MaxTunnelLength       = 7
	MaxTunnelQuantity     = 16
)

type Dialer struct {
	SAMAddress     string
	Timeout        time.Duration
	TunnelLength   uint8
	TunnelQuantity uint8
}

type Destination struct {
	Public  string
	Private string
}

func Check(ctx context.Context, samAddress string) error {
	if strings.TrimSpace(samAddress) == "" {
		samAddress = DefaultAddress
	}
	conn, reader, err := openCommandConn(ctx, samAddress, DefaultTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	return hello(conn, reader)
}

func GenerateDestination(ctx context.Context, samAddress string) (Destination, error) {
	if strings.TrimSpace(samAddress) == "" {
		samAddress = DefaultAddress
	}
	conn, reader, err := openCommandConn(ctx, samAddress, DefaultTimeout)
	if err != nil {
		return Destination{}, err
	}
	defer conn.Close()

	if err := hello(conn, reader); err != nil {
		return Destination{}, err
	}
	if err := sendCommand(conn, "DEST GENERATE SIGNATURE_TYPE=7"); err != nil {
		return Destination{}, err
	}
	reply, err := readReply(reader)
	if err != nil {
		return Destination{}, err
	}
	destination := Destination{
		Public:  strings.TrimSpace(reply.Values["PUB"]),
		Private: strings.TrimSpace(reply.Values["PRIV"]),
	}
	if destination.Public == "" || destination.Private == "" {
		return Destination{}, fmt.Errorf("i2p SAM destination generation returned incomplete reply: %s", reply.Raw)
	}
	return destination, nil
}

func (d Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("i2p SAM stream supports tcp network only, got %q", network)
	}
	samAddress := strings.TrimSpace(d.SAMAddress)
	if samAddress == "" {
		samAddress = DefaultAddress
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if err := validateTunnelSettings(d.TunnelLength, d.TunnelQuantity); err != nil {
		return nil, err
	}
	return dialStream(ctx, samAddress, address, timeout, d.TunnelLength, d.TunnelQuantity)
}

func Lookup(ctx context.Context, samAddress, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("i2p SAM lookup name is empty")
	}
	if strings.TrimSpace(samAddress) == "" {
		samAddress = DefaultAddress
	}

	conn, reader, err := openCommandConn(ctx, samAddress, DefaultTimeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := hello(conn, reader); err != nil {
		return "", err
	}
	if err := sendCommand(conn, "NAMING LOOKUP NAME="+name); err != nil {
		return "", err
	}
	reply, err := readReply(reader)
	if err != nil {
		return "", err
	}
	if err := reply.OK(); err != nil {
		return "", err
	}
	value := strings.TrimSpace(reply.Values["VALUE"])
	if value == "" {
		return "", fmt.Errorf("i2p SAM naming lookup for %q returned empty destination", name)
	}
	return value, nil
}

func dialStream(ctx context.Context, samAddress, address string, timeout time.Duration, tunnelLength, tunnelQuantity uint8) (net.Conn, error) {
	destination, toPort, err := splitDestination(address)
	if err != nil {
		return nil, err
	}
	sessionID, err := newSessionID()
	if err != nil {
		return nil, err
	}

	control, controlReader, err := openCommandConn(ctx, samAddress, timeout)
	if err != nil {
		return nil, err
	}
	controlOK := false
	defer func() {
		if !controlOK {
			_ = control.Close()
		}
	}()

	if err := hello(control, controlReader); err != nil {
		return nil, err
	}
	if err := sendCommand(control, sessionCreateCommand("STREAM", sessionID, sessionTunnelOptions(tunnelLength, tunnelQuantity)...)); err != nil {
		return nil, err
	}
	reply, err := readReply(controlReader)
	if err != nil {
		return nil, err
	}
	if err := reply.OK(); err != nil {
		return nil, fmt.Errorf("create i2p SAM stream session: %w", err)
	}
	_ = control.SetDeadline(time.Time{})
	controlOK = true

	data, dataReader, err := openCommandConn(ctx, samAddress, timeout)
	if err != nil {
		return nil, err
	}
	dataOK := false
	defer func() {
		if !dataOK {
			_ = data.Close()
		}
	}()

	if err := hello(data, dataReader); err != nil {
		return nil, err
	}
	command := "STREAM CONNECT ID=" + sessionID + " DESTINATION=" + destination + " SILENT=false"
	if toPort != "" {
		command += " TO_PORT=" + toPort
	}
	if err := sendCommand(data, command); err != nil {
		return nil, err
	}
	reply, err = readReply(dataReader)
	if err != nil {
		return nil, err
	}
	if err := reply.OK(); err != nil {
		return nil, fmt.Errorf("connect i2p SAM stream to %s: %w", address, err)
	}
	_ = data.SetDeadline(time.Time{})
	dataOK = true

	return &sessionConn{Conn: data, control: control}, nil
}

func sessionTunnelOptions(tunnelLength, tunnelQuantity uint8) []string {
	if tunnelLength == 0 {
		tunnelLength = DefaultTunnelLength
	}
	if tunnelQuantity == 0 {
		tunnelQuantity = DefaultTunnelQuantity
	}
	return []string{
		"inbound.length=" + strconv.FormatUint(uint64(tunnelLength), 10),
		"outbound.length=" + strconv.FormatUint(uint64(tunnelLength), 10),
		"inbound.quantity=" + strconv.FormatUint(uint64(tunnelQuantity), 10),
		"outbound.quantity=" + strconv.FormatUint(uint64(tunnelQuantity), 10),
	}
}

func validateTunnelSettings(tunnelLength, tunnelQuantity uint8) error {
	if tunnelLength > MaxTunnelLength {
		return fmt.Errorf("i2p SAM tunnel length %d is out of range 0..%d", tunnelLength, MaxTunnelLength)
	}
	if tunnelQuantity > MaxTunnelQuantity {
		return fmt.Errorf("i2p SAM tunnel quantity %d is out of range 1..%d", tunnelQuantity, MaxTunnelQuantity)
	}
	return nil
}

func openCommandConn(ctx context.Context, samAddress string, timeout time.Duration) (net.Conn, *bufio.Reader, error) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", samAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("dial i2p SAM bridge %s: %w", samAddress, err)
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	return conn, bufio.NewReader(conn), nil
}

func hello(conn net.Conn, reader *bufio.Reader) error {
	if err := sendCommand(conn, "HELLO VERSION MIN=3.0 MAX=3.3"); err != nil {
		return err
	}
	reply, err := readReply(reader)
	if err != nil {
		return err
	}
	if err := reply.OK(); err != nil {
		return fmt.Errorf("i2p SAM hello: %w", err)
	}
	return nil
}

func sendCommand(conn net.Conn, command string) error {
	if _, err := fmt.Fprintf(conn, "%s\n", command); err != nil {
		return fmt.Errorf("send i2p SAM command: %w", err)
	}
	return nil
}

func sessionCreateCommand(style, sessionID string, options ...string) string {
	return sessionCreateCommandWithDestination(style, sessionID, "TRANSIENT", options...)
}

func sessionCreateCommandWithDestination(style, sessionID, destination string, options ...string) string {
	if strings.TrimSpace(destination) == "" {
		destination = "TRANSIENT"
	}
	command := "SESSION CREATE STYLE=" + style +
		" ID=" + sessionID +
		" DESTINATION=" + destination +
		" SIGNATURE_TYPE=7"
	if !hasOptionPrefix(options, "inbound.length=") {
		command += " inbound.length=" + strconv.Itoa(DefaultTunnelLength)
	}
	if !hasOptionPrefix(options, "outbound.length=") {
		command += " outbound.length=" + strconv.Itoa(DefaultTunnelLength)
	}
	if !hasOptionPrefix(options, "inbound.quantity=") {
		command += " inbound.quantity=" + strconv.Itoa(DefaultTunnelQuantity)
	}
	if !hasOptionPrefix(options, "outbound.quantity=") {
		command += " outbound.quantity=" + strconv.Itoa(DefaultTunnelQuantity)
	}
	if len(options) > 0 {
		command += " " + strings.Join(options, " ")
	}
	return command
}

func hasOptionPrefix(options []string, prefix string) bool {
	for _, option := range options {
		if strings.HasPrefix(option, prefix) {
			return true
		}
	}
	return false
}

type Reply struct {
	Raw    string
	Values map[string]string
}

func readReply(reader *bufio.Reader) (Reply, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return Reply{}, fmt.Errorf("read i2p SAM reply: %w", err)
	}
	line = strings.TrimSpace(line)
	reply := Reply{
		Raw:    line,
		Values: make(map[string]string),
	}
	for _, field := range splitFields(line) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		reply.Values[strings.ToUpper(key)] = value
	}
	return reply, nil
}

func splitFields(line string) []string {
	var (
		fields  []string
		current strings.Builder
		quoted  bool
		escaped bool
	)
	for _, r := range line {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case quoted && r == '\\':
			escaped = true
		case r == '"':
			quoted = !quoted
		case unicode.IsSpace(r) && !quoted:
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}

func (r Reply) OK() error {
	if strings.EqualFold(r.Values["RESULT"], "OK") {
		return nil
	}
	result := r.Values["RESULT"]
	if result == "" {
		result = "UNKNOWN"
	}
	message := r.Values["MESSAGE"]
	if message == "" {
		message = r.Raw
	}
	return fmt.Errorf("i2p SAM result %s: %s", result, message)
}

func splitDestination(address string) (destination, toPort string, err error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", "", errors.New("i2p SAM destination is empty")
	}

	host, port, splitErr := net.SplitHostPort(address)
	if splitErr == nil {
		host = strings.Trim(host, "[]")
		if port != "" {
			if _, err := strconv.ParseUint(port, 10, 16); err != nil {
				return "", "", fmt.Errorf("invalid i2p SAM destination port %q: %w", port, err)
			}
		}
		return host, port, nil
	}
	if strings.Contains(splitErr.Error(), "missing port in address") {
		return strings.Trim(address, "[]"), "", nil
	}
	return "", "", fmt.Errorf("parse i2p SAM destination %q: %w", address, splitErr)
}

func newSessionID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate i2p SAM session id: %w", err)
	}
	return "anonbird-" + hex.EncodeToString(raw[:]), nil
}

type sessionConn struct {
	net.Conn
	control net.Conn
	once    sync.Once
	err     error
}

func (c *sessionConn) Close() error {
	c.once.Do(func() {
		dataErr := c.Conn.Close()
		controlErr := c.control.Close()
		if dataErr != nil {
			c.err = dataErr
			return
		}
		c.err = controlErr
	})
	return c.err
}
