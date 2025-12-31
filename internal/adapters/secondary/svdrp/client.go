package svdrp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/githubixx/vdradmin-go/internal/domain"
)

// Client implements the SVDRP protocol for VDR communication
type Client struct {
	host    string
	port    int
	timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
	rw   *bufio.ReadWriter
}

// NewClient creates a new SVDRP client
func NewClient(host string, port int, timeout time.Duration) *Client {
	return &Client{
		host:    host,
		port:    port,
		timeout: timeout,
	}
}

// Connect establishes a connection to VDR
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // already connected
	}

	dialer := &net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", c.host, c.port))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Read welcome message
	if _, err := c.readResponse(); err != nil {
		c.conn.Close()
		c.conn = nil
		return fmt.Errorf("failed to read welcome: %w", err)
	}

	return nil
}

// Close closes the connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	// Send QUIT command (ignore errors if connection is broken)
	c.rw.WriteString("QUIT\r\n")
	c.rw.Flush()

	err := c.conn.Close()
	c.conn = nil
	c.rw = nil
	return err
}

// Ping checks if VDR is reachable
func (c *Client) Ping(ctx context.Context) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Send a simple command to check connectivity
	if err := c.sendCommand("STAT disk"); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// GetChannels retrieves all channels
func (c *Client) GetChannels(ctx context.Context) ([]domain.Channel, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand("LSTC"); err != nil {
		return nil, err
	}

	lines, err := c.readResponse()
	if err != nil {
		return nil, err
	}

	channels := make([]domain.Channel, 0, len(lines))
	for i, line := range lines {
		channel := parseChannel(i+1, line)
		channels = append(channels, channel)
	}

	return channels, nil
}

// GetEPG retrieves EPG data
func (c *Client) GetEPG(ctx context.Context, channelID string, at time.Time) ([]domain.EPGEvent, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cmd := "LSTE"
	if channelID != "" {
		if !at.IsZero() {
			cmd = fmt.Sprintf("LSTE %s %d", channelID, at.Unix())
		} else {
			cmd = fmt.Sprintf("LSTE %s", channelID)
		}
	} else if !at.IsZero() {
		// For all channels at a specific time, need to use different approach
		// LSTE with just timestamp may not work as expected in all VDR versions
		// For now, just get all EPG data
		cmd = "LSTE"
	}

	if err := c.sendCommand(cmd); err != nil {
		return nil, err
	}

	lines, err := c.readResponse()
	if err != nil {
		// Some channels simply have no EPG (e.g. no schedule). Treat as empty result.
		if channelID != "" && isSVDRPNoSchedule(err) {
			return []domain.EPGEvent{}, nil
		}

		// Some VDR versions don't support the optional timestamp argument.
		// Example: "SVDRP error 501: Unknown option: <timestamp>"
		if channelID != "" && !at.IsZero() && isSVDRPUnknownOption(err) {
			// Retry without timestamp.
			cmd = fmt.Sprintf("LSTE %s", channelID)
			if err := c.sendCommand(cmd); err != nil {
				return nil, err
			}
			lines, err = c.readResponse()
			if err != nil {
				if isSVDRPNoSchedule(err) {
					return []domain.EPGEvent{}, nil
				}
				return nil, err
			}
			return parseEPGEvents(lines), nil
		}
		return nil, err
	}

	return parseEPGEvents(lines), nil
}

func isSVDRPUnknownOption(err error) bool {
	if err == nil {
		return false
	}
	var target error = err
	if u := errors.Unwrap(err); u != nil {
		target = u
	}
	msg := target.Error()
	return strings.Contains(msg, "SVDRP error 501") || strings.Contains(msg, "Unknown option")
}

func isSVDRPNoSchedule(err error) bool {
	if err == nil {
		return false
	}
	var target error = err
	if u := errors.Unwrap(err); u != nil {
		target = u
	}
	msg := target.Error()
	return strings.Contains(msg, "SVDRP error 550") || strings.Contains(msg, "No schedule found")
}

// GetTimers retrieves all timers
func (c *Client) GetTimers(ctx context.Context) ([]domain.Timer, error) {
	return c.withRetry(ctx, func() ([]domain.Timer, error) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.sendCommand("LSTT"); err != nil {
			return nil, err
		}

		lines, err := c.readResponse()
		if err != nil {
			return nil, err
		}

		timers := make([]domain.Timer, 0, len(lines))
		for _, line := range lines {
			timer, err := parseTimer(line)
			if err == nil {
				timers = append(timers, timer)
			}
		}

		return timers, nil
	})
}

// CreateTimer creates a new timer
func (c *Client) CreateTimer(ctx context.Context, timer *domain.Timer) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	timerStr := formatTimer(timer)
	if err := c.sendCommand(fmt.Sprintf("NEWT %s", timerStr)); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// UpdateTimer updates an existing timer
func (c *Client) UpdateTimer(ctx context.Context, timer *domain.Timer) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	timerStr := formatTimer(timer)
	if err := c.sendCommand(fmt.Sprintf("MODT %d %s", timer.ID, timerStr)); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// DeleteTimer deletes a timer
func (c *Client) DeleteTimer(ctx context.Context, timerID int) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand(fmt.Sprintf("DELT %d", timerID)); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// GetRecordings retrieves all recordings
func (c *Client) GetRecordings(ctx context.Context) ([]domain.Recording, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand("LSTR"); err != nil {
		return nil, err
	}

	lines, err := c.readResponse()
	if err != nil {
		return nil, err
	}

	recordings := make([]domain.Recording, 0, len(lines))
	for _, line := range lines {
		recording, err := parseRecording(line)
		if err == nil {
			recordings = append(recordings, recording)
		}
	}

	return recordings, nil
}

// DeleteRecording deletes a recording
func (c *Client) DeleteRecording(ctx context.Context, path string) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand(fmt.Sprintf("DELR %s", path)); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// GetCurrentChannel returns the current channel
func (c *Client) GetCurrentChannel(ctx context.Context) (string, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return "", err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand("CHAN"); err != nil {
		return "", err
	}

	lines, err := c.readResponse()
	if err != nil || len(lines) == 0 {
		return "", err
	}

	return strings.TrimSpace(lines[0]), nil
}

// SetCurrentChannel switches to a channel
func (c *Client) SetCurrentChannel(ctx context.Context, channelID string) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand(fmt.Sprintf("CHAN %s", channelID)); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// SendKey sends a remote control key
func (c *Client) SendKey(ctx context.Context, key string) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.sendCommand(fmt.Sprintf("HITK %s", key)); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// ensureConnected ensures the connection is established
func (c *Client) ensureConnected(ctx context.Context) error {
	c.mu.Lock()
	connected := c.conn != nil
	c.mu.Unlock()

	if !connected {
		return c.Connect(ctx)
	}

	return nil
}

// withRetry executes a function with automatic reconnection on connection errors
func (c *Client) withRetry(ctx context.Context, fn func() ([]domain.Timer, error)) ([]domain.Timer, error) {
	// Ensure connection before first try
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	// First attempt
	result, err := fn()
	if err == nil {
		return result, nil
	}

	// If connection error, try to reconnect once
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	// Retry once
	return fn()
}

// sendCommand sends a command to VDR
func (c *Client) sendCommand(cmd string) error {
	if c.conn == nil {
		return domain.ErrConnection
	}

	if _, err := c.rw.WriteString(cmd + "\r\n"); err != nil {
		// Connection broken, close it
		c.closeConnection()
		return err
	}

	if err := c.rw.Flush(); err != nil {
		// Connection broken, close it
		c.closeConnection()
		return err
	}

	return nil
}

// readResponse reads a response from VDR
func (c *Client) readResponse() ([]string, error) {
	if c.conn == nil {
		return nil, domain.ErrConnection
	}

	var lines []string
	for {
		line, err := c.rw.ReadString('\n')
		if err != nil {
			// Connection broken, close it
			c.closeConnection()
			return nil, err
		}

		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}

		// Parse response code
		code, err := strconv.Atoi(line[0:3])
		if err != nil {
			continue
		}

		// Check for error codes
		if code >= 400 {
			return nil, fmt.Errorf("SVDRP error %d: %s", code, line[4:])
		}

		// Check if this is a continuation line (-)
		if len(line) > 3 && line[3] == '-' {
			if len(line) > 4 {
				lines = append(lines, line[4:])
			}
			continue
		}

		// This is the last line (space after code)
		if len(line) > 4 {
			lines = append(lines, line[4:])
		}
		break
	}

	return lines, nil
}

// closeConnection closes the connection without locking (must be called with lock held)
func (c *Client) closeConnection() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.rw = nil
	}
}

// parseChannel parses a channel line from LSTC
func parseChannel(number int, line string) domain.Channel {
	chID, chNumber, chName, provider := parseSVDRPChannelHeader(strings.TrimSpace(line), number)
	if chID == "" && chName == "" {
		return domain.Channel{}
	}
	if chNumber == 0 {
		chNumber = number
	}
	if chID == "" {
		chID = strconv.Itoa(chNumber)
	}

	return domain.Channel{ID: chID, Number: chNumber, Name: chName, Provider: provider}
}

func parseSVDRPChannelHeader(text string, numberFallback int) (channelID string, channelNumber int, channelName string, provider string) {
	if text == "" {
		return "", 0, "", ""
	}

	parts := strings.SplitN(text, " ", 3)
	first := strings.TrimSpace(parts[0])

	// Cases we try to support:
	// 1) "<num> <channels.conf line>"
	// 2) "<num> <channel-id> <channels.conf line>"
	// 3) "<channel-id> <channels.conf line>"

	if n, err := strconv.Atoi(first); err == nil {
		channelNumber = n
		if len(parts) >= 2 {
			rest := strings.TrimSpace(text[len(first):])
			rest = strings.TrimSpace(rest)
			// If the next token looks like a channel-id, prefer it.
			r2 := strings.SplitN(rest, " ", 2)
			if len(r2) >= 1 && looksLikeVDRChannelID(r2[0]) {
				channelID = r2[0]
				if len(r2) == 2 {
					channelName, provider = parseChannelNameProvider(r2[1])
				}
			} else {
				// Otherwise, derive ID from channels.conf-like content if possible.
				channelID = deriveChannelIDFromChannelsConf(rest)
				channelName, provider = parseChannelNameProvider(rest)
				if channelID == "" {
					channelID = first
				}
			}
		}
		return channelID, channelNumber, channelName, provider
	}

	// first isn't numeric, so treat it as channel id.
	if looksLikeVDRChannelID(first) {
		channelID = first
		if len(parts) >= 2 {
			rest := strings.TrimSpace(text[len(first):])
			rest = strings.TrimSpace(rest)
			channelName, provider = parseChannelNameProvider(rest)
		}
		return channelID, numberFallback, channelName, provider
	}

	// Fallback: treat as channels.conf line.
	channelID = deriveChannelIDFromChannelsConf(text)
	channelName, provider = parseChannelNameProvider(text)
	return channelID, numberFallback, channelName, provider
}

func looksLikeVDRChannelID(s string) bool {
	// Common VDR channel id form: S19.2E-1-1010-11150 (source-nid-tid-sid)
	return strings.Contains(s, "-") && (strings.HasPrefix(s, "S") || strings.HasPrefix(s, "C") || strings.HasPrefix(s, "T") || strings.HasPrefix(s, "A"))
}

func parseChannelNameProvider(channelsConfLine string) (name string, provider string) {
	nameProvider := strings.SplitN(channelsConfLine, ":", 2)[0]
	nameParts := strings.SplitN(nameProvider, ";", 2)
	name = strings.TrimSpace(nameParts[0])
	if len(nameParts) == 2 {
		provider = strings.TrimSpace(nameParts[1])
	}
	return name, provider
}

func deriveChannelIDFromChannelsConf(channelsConfLine string) string {
	// channels.conf fields (common):
	// 0: Name;Provider
	// 1: Frequency
	// 2: Parameters
	// 3: Source
	// ...
	// 9:  SID
	// 10: NID
	// 11: TID
	fields := strings.Split(channelsConfLine, ":")
	if len(fields) < 12 {
		return ""
	}
	source := strings.TrimSpace(fields[3])
	sid := strings.TrimSpace(fields[9])
	nid := strings.TrimSpace(fields[10])
	tid := strings.TrimSpace(fields[11])
	if source == "" || sid == "" || nid == "" || tid == "" {
		return ""
	}
	return fmt.Sprintf("%s-%s-%s-%s", source, nid, tid, sid)
}

// parseEPGEvents parses EPG events from LSTE response
func parseEPGEvents(lines []string) []domain.EPGEvent {
	events := make([]domain.EPGEvent, 0)
	var currentEvent *domain.EPGEvent
	currentChannelID := ""
	currentChannelName := ""
	currentChannelNumber := 0

	for _, line := range lines {
		if strings.HasPrefix(line, "C ") {
			// Channel header line; applies to subsequent E-lines.
			chID, chNum, chName, _ := parseSVDRPChannelHeader(strings.TrimSpace(line[2:]), 0)
			currentChannelID = chID
			currentChannelName = chName
			currentChannelNumber = chNum
			continue
		}

		if strings.HasPrefix(line, "E ") {
			// New event
			if currentEvent != nil {
				events = append(events, *currentEvent)
			}
			currentEvent = parseEPGEventLine(line)
			if currentEvent != nil {
				currentEvent.ChannelID = currentChannelID
				currentEvent.ChannelNumber = currentChannelNumber
				currentEvent.ChannelName = currentChannelName
			}
		} else if currentEvent != nil {
			// Additional event data (T, S, D, etc.)
			if strings.HasPrefix(line, "T ") {
				currentEvent.Title = line[2:]
			} else if strings.HasPrefix(line, "S ") {
				currentEvent.Subtitle = line[2:]
			} else if strings.HasPrefix(line, "D ") {
				currentEvent.Description += line[2:] + "\n"
			}
		}
	}

	if currentEvent != nil {
		events = append(events, *currentEvent)
	}

	return events
}

// parseEPGEventLine parses an EPG event line
func parseEPGEventLine(line string) *domain.EPGEvent {
	// Format: E eventID startTime duration [tableID] [version]
	parts := strings.Fields(line[2:])
	if len(parts) < 3 {
		return nil
	}

	eventID, _ := strconv.Atoi(parts[0])
	startUnix, _ := strconv.ParseInt(parts[1], 10, 64)
	duration, _ := strconv.Atoi(parts[2])

	start := time.Unix(startUnix, 0)
	dur := time.Duration(duration) * time.Second

	return &domain.EPGEvent{
		EventID:  eventID,
		Start:    start,
		Stop:     start.Add(dur),
		Duration: dur,
	}
}

// parseTimer parses a timer from LSTT response
func parseTimer(line string) (domain.Timer, error) {
	// Format: ID active:channel:day:start:stop:priority:lifetime:title:aux
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return domain.Timer{}, fmt.Errorf("invalid timer format")
	}

	timerID, _ := strconv.Atoi(parts[0])
	fields := strings.Split(parts[1], ":")

	if len(fields) < 8 {
		return domain.Timer{}, fmt.Errorf("insufficient timer fields")
	}

	active := fields[0] == "1"
	priority, _ := strconv.Atoi(fields[5])
	lifetime, _ := strconv.Atoi(fields[6])

	return domain.Timer{
		ID:        timerID,
		Active:    active,
		ChannelID: fields[1],
		Priority:  priority,
		Lifetime:  lifetime,
		Title:     fields[7],
	}, nil
}

// formatTimer formats a timer for SVDRP commands
func formatTimer(timer *domain.Timer) string {
	active := "0"
	if timer.Active {
		active = "1"
	}

	// Simplified format - full implementation would handle day/time formatting
	return fmt.Sprintf("%s:%s:MTWTFSS:%s:%s:%d:%d:%s",
		active,
		timer.ChannelID,
		timer.Start.Format("1504"),
		timer.Stop.Format("1504"),
		timer.Priority,
		timer.Lifetime,
		timer.Title,
	)
}

// parseRecording parses a recording from LSTR response
func parseRecording(line string) (domain.Recording, error) {
	// Format: ID date time~title~description
	parts := strings.SplitN(line, " ", 2)
	if len(parts) != 2 {
		return domain.Recording{}, fmt.Errorf("invalid recording format")
	}

	path := parts[0]
	info := strings.Split(parts[1], "~")

	title := ""
	if len(info) > 0 {
		title = info[0]
	}

	return domain.Recording{
		Path:  path,
		Title: title,
	}, nil
}
