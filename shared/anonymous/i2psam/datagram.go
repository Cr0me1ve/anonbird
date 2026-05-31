package i2psam

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DatagramStyleRepliable = "DATAGRAM"
	DatagramStyleRaw       = "RAW"

	DefaultSAMUDPPort       = "7655"
	DefaultRawProtocol      = 18
	MaxRepliableDatagramLen = 31744
	MaxRawDatagramLen       = 32768
)

type DatagramConfig struct {
	SAMAddress       string
	SAMUDPAddress    string
	Timeout          time.Duration
	Style            string
	SessionID        string
	PrivateKey       string
	FromPort         uint16
	ToPort           uint16
	Protocol         uint8
	InboundLength    uint8
	OutboundLength   uint8
	InboundQuantity  uint8
	OutboundQuantity uint8
}

type SendOptions struct {
	FromPort uint16
	ToPort   uint16
	Protocol uint8
}

type Datagram struct {
	Source       string
	Payload      []byte
	FromPort     uint16
	ToPort       uint16
	Protocol     uint8
	AnonymousRaw bool
}

type DatagramSession struct {
	id     string
	style  string
	reader *bufio.Reader

	control net.Conn
	udp     net.Conn

	mu   sync.Mutex
	once sync.Once
	err  error
}

func NewDatagramSession(ctx context.Context, cfg DatagramConfig) (*DatagramSession, error) {
	cfg = normalizeDatagramConfig(cfg)
	if err := validateDatagramConfig(cfg); err != nil {
		return nil, err
	}

	control, reader, err := openCommandConn(ctx, cfg.SAMAddress, cfg.Timeout)
	if err != nil {
		return nil, err
	}
	controlOK := false
	defer func() {
		if !controlOK {
			_ = control.Close()
		}
	}()

	if err := hello(control, reader); err != nil {
		return nil, err
	}
	if err := sendCommand(control, datagramSessionCreateCommand(cfg)); err != nil {
		return nil, err
	}
	reply, err := readReply(reader)
	if err != nil {
		return nil, err
	}
	if err := reply.OK(); err != nil {
		return nil, fmt.Errorf("create i2p SAM %s datagram session: %w", cfg.Style, err)
	}
	_ = control.SetDeadline(time.Time{})
	controlOK = true

	udpDialer := &net.Dialer{Timeout: cfg.Timeout}
	udp, err := udpDialer.DialContext(ctx, "udp", cfg.SAMUDPAddress)
	if err != nil {
		_ = control.Close()
		return nil, fmt.Errorf("dial i2p SAM UDP bridge %s: %w", cfg.SAMUDPAddress, err)
	}

	return &DatagramSession{
		id:      cfg.SessionID,
		style:   cfg.Style,
		reader:  reader,
		control: control,
		udp:     udp,
	}, nil
}

func (s *DatagramSession) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *DatagramSession) Send(ctx context.Context, destination string, payload []byte, opts SendOptions) error {
	if s == nil {
		return errors.New("i2p SAM datagram session is nil")
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return errors.New("i2p SAM datagram destination is empty")
	}
	if err := validateDatagramPayload(s.style, len(payload)); err != nil {
		return err
	}
	if s.style == DatagramStyleRaw && opts.Protocol != 0 && protocolIsDisallowedForRaw(opts.Protocol) {
		return fmt.Errorf("i2p SAM raw datagram protocol %d is reserved and not allowed", opts.Protocol)
	}

	header := "3.0 " + s.id + " " + destination
	if opts.FromPort != 0 {
		header += " FROM_PORT=" + strconv.FormatUint(uint64(opts.FromPort), 10)
	}
	if opts.ToPort != 0 {
		header += " TO_PORT=" + strconv.FormatUint(uint64(opts.ToPort), 10)
	}
	if s.style == DatagramStyleRaw {
		protocol := opts.Protocol
		if protocol == 0 {
			protocol = DefaultRawProtocol
		}
		header += " PROTOCOL=" + strconv.FormatUint(uint64(protocol), 10)
	}
	packet := make([]byte, 0, len(header)+1+len(payload))
	packet = append(packet, header...)
	packet = append(packet, '\n')
	packet = append(packet, payload...)

	if deadline, ok := ctx.Deadline(); ok {
		_ = s.udp.SetWriteDeadline(deadline)
		defer s.udp.SetWriteDeadline(time.Time{})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.udp.Write(packet); err != nil {
		return fmt.Errorf("send i2p SAM datagram: %w", err)
	}
	return nil
}

func (s *DatagramSession) Receive(ctx context.Context) (Datagram, error) {
	if s == nil {
		return Datagram{}, errors.New("i2p SAM datagram session is nil")
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = s.control.SetReadDeadline(deadline)
		defer s.control.SetReadDeadline(time.Time{})
	}
	reply, err := readReply(s.reader)
	if err != nil {
		return Datagram{}, err
	}
	size, err := parseReplyUint16(reply, "SIZE")
	if err != nil {
		return Datagram{}, err
	}
	if err := validateDatagramPayload(s.style, int(size)); err != nil {
		return Datagram{}, err
	}
	datagram := Datagram{
		Source:       reply.Values["DESTINATION"],
		FromPort:     parseOptionalPort(reply, "FROM_PORT"),
		ToPort:       parseOptionalPort(reply, "TO_PORT"),
		Protocol:     parseOptionalProtocol(reply),
		AnonymousRaw: strings.HasPrefix(strings.ToUpper(reply.Raw), "RAW RECEIVED"),
		Payload:      make([]byte, int(size)),
	}
	if strings.HasPrefix(strings.ToUpper(reply.Raw), "DATAGRAM RECEIVED") {
		datagram.AnonymousRaw = false
	}
	if _, err := io.ReadFull(s.reader, datagram.Payload); err != nil {
		return Datagram{}, fmt.Errorf("read i2p SAM datagram payload: %w", err)
	}
	return datagram, nil
}

func (s *DatagramSession) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		udpErr := s.udp.Close()
		controlErr := s.control.Close()
		if udpErr != nil {
			s.err = udpErr
			return
		}
		s.err = controlErr
	})
	return s.err
}

func normalizeDatagramConfig(cfg DatagramConfig) DatagramConfig {
	if strings.TrimSpace(cfg.SAMAddress) == "" {
		cfg.SAMAddress = DefaultAddress
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	cfg.Style = strings.ToUpper(strings.TrimSpace(cfg.Style))
	if cfg.Style == "" {
		cfg.Style = DatagramStyleRaw
	}
	if strings.TrimSpace(cfg.SessionID) == "" {
		if id, err := newSessionID(); err == nil {
			cfg.SessionID = id
		}
	}
	if strings.TrimSpace(cfg.SAMUDPAddress) == "" {
		cfg.SAMUDPAddress = defaultSAMUDPAddress(cfg.SAMAddress)
	}
	if cfg.Protocol == 0 {
		cfg.Protocol = DefaultRawProtocol
	}
	if cfg.InboundLength == 0 {
		cfg.InboundLength = DefaultTunnelLength
	}
	if cfg.OutboundLength == 0 {
		cfg.OutboundLength = DefaultTunnelLength
	}
	if cfg.InboundQuantity == 0 {
		cfg.InboundQuantity = DefaultTunnelQuantity
	}
	if cfg.OutboundQuantity == 0 {
		cfg.OutboundQuantity = DefaultTunnelQuantity
	}
	return cfg
}

func validateDatagramConfig(cfg DatagramConfig) error {
	if strings.TrimSpace(cfg.SessionID) == "" {
		return errors.New("i2p SAM datagram session id is empty")
	}
	switch cfg.Style {
	case DatagramStyleRepliable, DatagramStyleRaw:
	default:
		return fmt.Errorf("unsupported i2p SAM datagram style %q", cfg.Style)
	}
	if cfg.Style == DatagramStyleRaw && protocolIsDisallowedForRaw(cfg.Protocol) {
		return fmt.Errorf("i2p SAM raw datagram protocol %d is reserved and not allowed", cfg.Protocol)
	}
	if err := validateTunnelSettings(cfg.InboundLength, cfg.InboundQuantity); err != nil {
		return err
	}
	if err := validateTunnelSettings(cfg.OutboundLength, cfg.OutboundQuantity); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.SAMUDPAddress) == "" {
		return errors.New("i2p SAM UDP address is empty")
	}
	return nil
}

func datagramSessionCreateCommand(cfg DatagramConfig) string {
	options := []string{
		"inbound.quantity=" + strconv.FormatUint(uint64(cfg.InboundQuantity), 10),
		"outbound.quantity=" + strconv.FormatUint(uint64(cfg.OutboundQuantity), 10),
	}
	options = append([]string{
		"inbound.length=" + strconv.FormatUint(uint64(cfg.InboundLength), 10),
		"outbound.length=" + strconv.FormatUint(uint64(cfg.OutboundLength), 10),
	}, options...)
	if cfg.FromPort != 0 {
		options = append(options, "FROM_PORT="+strconv.FormatUint(uint64(cfg.FromPort), 10))
	}
	if cfg.ToPort != 0 {
		options = append(options, "TO_PORT="+strconv.FormatUint(uint64(cfg.ToPort), 10))
	}
	if cfg.Style == DatagramStyleRaw && cfg.Protocol != 0 {
		options = append(options, "PROTOCOL="+strconv.FormatUint(uint64(cfg.Protocol), 10))
	}
	destination := strings.TrimSpace(cfg.PrivateKey)
	if destination == "" {
		destination = "TRANSIENT"
	}
	return sessionCreateCommandWithDestination(cfg.Style, cfg.SessionID, destination, options...)
}

func defaultSAMUDPAddress(samAddress string) string {
	host, _, err := net.SplitHostPort(samAddress)
	if err != nil || strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, DefaultSAMUDPPort)
}

func validateDatagramPayload(style string, length int) error {
	if length < 1 {
		return errors.New("i2p SAM datagram payload is empty")
	}
	limit := MaxRepliableDatagramLen
	if style == DatagramStyleRaw {
		limit = MaxRawDatagramLen
	}
	if length > limit {
		return fmt.Errorf("i2p SAM %s datagram payload is %d bytes, max %d", style, length, limit)
	}
	return nil
}

func parseReplyUint16(reply Reply, key string) (uint16, error) {
	value := strings.TrimSpace(reply.Values[key])
	if value == "" {
		return 0, fmt.Errorf("i2p SAM reply missing %s: %s", key, reply.Raw)
	}
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse i2p SAM reply %s=%q: %w", key, value, err)
	}
	return uint16(parsed), nil
}

func parseOptionalPort(reply Reply, key string) uint16 {
	port, err := parseReplyUint16(reply, key)
	if err != nil {
		return 0
	}
	return port
}

func parseOptionalProtocol(reply Reply) uint8 {
	value := strings.TrimSpace(reply.Values["PROTOCOL"])
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return 0
	}
	return uint8(parsed)
}

func protocolIsDisallowedForRaw(protocol uint8) bool {
	switch protocol {
	case 6, 17, 19, 20:
		return true
	default:
		return false
	}
}
