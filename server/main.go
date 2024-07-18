package main

import (
	"context"
	chats "distributed-realtime-chat/proto"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type server struct {
	chats.UnimplementedChatServiceServer
	rabbitMQURL  string
	rabbitMQConn *amqp.Connection
	rabbitMQChan *amqp.Channel
	clients      sync.Map
	mu           sync.Mutex
	queueName    string
}

func newServer(rabbitMQURL string) *server {
	s := &server{
		rabbitMQURL: rabbitMQURL,
		queueName:   fmt.Sprintf("chat_queue_%d", time.Now().UnixNano()),
	}
	s.connectRabbitMQ()
	go s.maintainRabbitMQConnection()
	return s
}

func (s *server) connectRabbitMQ() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	s.rabbitMQConn, err = amqp.Dial(s.rabbitMQURL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %v", err)
	}

	s.rabbitMQChan, err = s.rabbitMQConn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel: %v", err)
	}

	err = s.rabbitMQChan.ExchangeDeclare(
		"chat_broadcast", // name
		"fanout",         // type
		true,             // durable
		false,            // auto-deleted
		false,            // internal
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare an exchange: %v", err)
	}

	_, err = s.rabbitMQChan.QueueDeclare(
		s.queueName, // name
		false,       // durable
		false,       // delete when unused
		true,        // exclusive
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %v", err)
	}

	err = s.rabbitMQChan.QueueBind(
		s.queueName,      // queue name
		"",               // routing key
		"chat_broadcast", // exchange
		false,
		nil)
	if err != nil {
		return fmt.Errorf("failed to bind a queue: %v", err)
	}

	go s.consumeMessages()

	return nil
}

func (s *server) maintainRabbitMQConnection() {
	for {
		<-time.After(5 * time.Second)
		if s.rabbitMQConn == nil || s.rabbitMQConn.IsClosed() {
			log.Println("RabbitMQ connection lost. Reconnecting...")
			for {
				err := s.connectRabbitMQ()
				if err == nil {
					log.Println("Reconnected to RabbitMQ")
					break
				}
				log.Printf("Failed to reconnect to RabbitMQ: %v. Retrying in 5 seconds...", err)
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (s *server) consumeMessages() {
	msgs, err := s.rabbitMQChan.Consume(
		s.queueName, // queue
		"",          // consumer
		true,        // auto-ack
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		log.Printf("Failed to register a consumer: %v", err)
		return
	}

	for d := range msgs {
		s.clients.Range(func(key, value interface{}) bool {
			clientChan := value.(chan *chats.ChatMessage)
			select {
			case clientChan <- &chats.ChatMessage{Message: string(d.Body)}:
			default:
				// If the channel is full, we skip this message for this client
			}
			return true
		})
	}
}

func (s *server) Chat(stream chats.ChatService_ChatServer) error {
	clientChan := make(chan *chats.ChatMessage, 100)
	clientID := fmt.Sprintf("%p", stream)
	s.clients.Store(clientID, clientChan)
	defer s.clients.Delete(clientID)

	log.Printf("New client connected: %s", clientID)
	defer log.Printf("Client disconnected: %s", clientID)

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("Context done for client: %s", clientID)
				return
			case msg := <-clientChan:
				if err := stream.Send(msg); err != nil {
					log.Printf("Failed to send message to client %s: %v", clientID, err)
					return
				}
				log.Printf("Sent message to client: %s", clientID)
			}
		}
	}()

	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Printf("Client %s closed the stream", clientID)
			} else {
				log.Printf("Failed to receive message from client %s: %v", clientID, err)
			}
			return err
		}
		log.Printf("Received message from client %s: %s", clientID, in.Message)

		err = s.rabbitMQChan.Publish(
			"chat_broadcast", // exchange
			"",               // routing key
			false,            // mandatory
			false,            // immediate
			amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(in.Message),
			})
		if err != nil {
			log.Printf("Failed to publish a message from client %s: %v", clientID, err)
			return err
		}
		log.Printf("Published message from client %s to RabbitMQ", clientID)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Minute,
			MaxConnectionAge:      30 * time.Second,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  5 * time.Second,
			Timeout:               1 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxConcurrentStreams(1000000),
	)
	chats.RegisterChatServiceServer(s, newServer("amqp://guest:guest@rabbitmq:5672/"))

	go func() {
		log.Printf("Server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("Shutting down server...")
	s.GracefulStop()
}
