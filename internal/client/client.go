package client

import (
	"context"
	"distributed-realtime-chat/internal/chat"
	"google.golang.org/grpc"
	"io"
	"log"
)

type ChatClient struct {
	conn   *grpc.ClientConn
	client chat.ChatServiceClient
	stream chat.ChatService_ChatClient
}

func NewChatClient(serverAddr string) (*ChatClient, error) {
	conn, err := grpc.Dial(serverAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	client := chat.NewChatServiceClient(conn)
	return &ChatClient{
		conn:   conn,
		client: client,
	}, nil
}

func (c *ChatClient) Connect(ctx context.Context) error {
	stream, err := c.client.Chat(ctx)
	if err != nil {
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
	c.conn.Close()
}
