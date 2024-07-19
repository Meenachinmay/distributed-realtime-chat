package client

import (
	"context"
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/pkg/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"log"
	"sync"
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
		return grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}, poolSize)

	return &ChatClient{
		pool: pool,
	}, nil
}

func (c *ChatClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, err := c.pool.Get(ctx)
	if err != nil {
		return err
	}

	c.client = chat.NewChatServiceClient(conn)
	stream, err := c.client.Chat(ctx)
	if err != nil {
		c.pool.Put(conn)
		return err
	}

	c.stream = stream
	return nil
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

func (c *ChatClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stream != nil {
		c.stream.CloseSend()
	}
	c.pool.Close()
}
