package client

import (
	"context"
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"log"
	"math/rand"
	"sync"
	"time"
)

type ChatClient struct {
	pool   *utils.ConnPool
	conn   *grpc.ClientConn
	client chat.ChatServiceClient
	stream chat.ChatService_ChatClient
	mu     sync.Mutex
}

func NewChatClient(serverAddr string, poolSize int) (*ChatClient, error) {
	pool := utils.NewConnPool(func() (*grpc.ClientConn, error) {
		return grpc.Dial(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}, poolSize)

	return &ChatClient{
		pool: pool,
	}, nil
}

func (c *ChatClient) Connect(ctx context.Context) error {
	backoffDuration := 100 * time.Millisecond
	maxBackoff := 5 * time.Second

	for {
		conn, err := c.pool.Get(ctx)
		if err == nil {
			c.client = chat.NewChatServiceClient(conn)
			stream, err := c.client.Chat(ctx)
			if err == nil {
				c.stream = stream
				return nil
			}
			c.pool.Put(conn)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoffDuration):
			backoffDuration = time.Duration(float64(backoffDuration) * (1.5 + rand.Float64()*0.5))
			if backoffDuration > maxBackoff {
				backoffDuration = maxBackoff
			}
		}
	}
}

func (c *ChatClient) SendMessage(msg *chat.ChatMessage) error {
	return c.stream.Send(msg)
}

func (c *ChatClient) ReceiveMessages(msgChan chan<- *chat.ChatMessage) {
	for {
		msg, err := c.stream.Recv()
		if err == io.EOF {
			close(msgChan)
			return
		}
		if err != nil {
			log.Printf("Error receiving message: %v", err)
			close(msgChan)
			return
		}
		msgChan <- msg
	}
}

func (c *ChatClient) SendMessageBatch(messages []*chat.ChatMessage) error {
	for _, msg := range messages {
		err := c.stream.Send(msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *ChatClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream != nil {
		c.stream.CloseSend()
	}
	c.pool.Close()
}
