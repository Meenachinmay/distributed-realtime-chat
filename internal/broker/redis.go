package broker

import (
	"context"
	"distributed-realtime-chat/internal/chat"
	"encoding/json"
	"github.com/redis/go-redis/v9"
	"log"
)

type RedisBroker struct {
	client *redis.Client
}

func NewRedisBroker(addr string) *RedisBroker {
	return &RedisBroker{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
	}
}

func (rb *RedisBroker) Publish(ctx context.Context, channel string, message *chat.ChatMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return rb.client.Publish(ctx, channel, data).Err()
}

func (rb *RedisBroker) Subscribe(ctx context.Context, channel string) (<-chan *chat.ChatMessage, error) {
	pubsub := rb.client.Subscribe(ctx, channel)
	ch := make(chan *chat.ChatMessage)

	go func() {
		defer close(ch)
		for {
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				log.Printf("Error receiving message from Redis: %v", err)
				return
			}

			var chatMsg chat.ChatMessage
			if err := json.Unmarshal([]byte(msg.Payload), &chatMsg); err != nil {
				log.Printf("Error unmarshaling message: %v", err)
				continue
			}

			ch <- &chatMsg
		}
	}()

	return ch, nil
}
