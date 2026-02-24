package channel

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// CLIChannel implements the Channel interface for terminal interaction.
type CLIChannel struct {
	inbound chan InboundMessage
	status  ChannelStatus
	cancel  context.CancelFunc
	mu      sync.Mutex
}

// NewCLIChannel creates a new CLI channel adapter.
func NewCLIChannel() *CLIChannel {
	return &CLIChannel{
		inbound: make(chan InboundMessage, 10),
		status:  StatusDisconnected,
	}
}

func (c *CLIChannel) Name() string { return "cli" }

func (c *CLIChannel) Connect(ctx context.Context) error {
	c.mu.Lock()
	c.status = StatusConnected
	ctx, c.cancel = context.WithCancel(ctx)
	c.mu.Unlock()

	go c.readLoop(ctx)
	return nil
}

func (c *CLIChannel) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = StatusDisconnected
	if c.cancel != nil {
		c.cancel()
	}
	return nil
}

func (c *CLIChannel) Send(_ context.Context, msg OutboundMessage) error {
	fmt.Println(msg.Text)
	return nil
}

func (c *CLIChannel) Receive() <-chan InboundMessage {
	return c.inbound
}

func (c *CLIChannel) Status() ChannelStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *CLIChannel) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	lines := make(chan string, 1)

	go func() {
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	for {
		fmt.Print("\n> ")
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			text := strings.TrimSpace(line)
			if text == "" {
				continue
			}
			if text == "/quit" || text == "/exit" {
				return
			}
			c.inbound <- InboundMessage{
				Channel:    "cli",
				ChatType:   ChatTypeDirect,
				SenderID:   "local",
				SenderName: "User",
				Text:       text,
				Timestamp:  time.Now(),
			}
		}
	}
}
