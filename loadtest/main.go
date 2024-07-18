package main

import (
	"context"
	chats "distributed-realtime-chat/proto"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

const (
	numClients = 10000
	address    = "localhost:80"
)

var (
	successfulConnections int64
	failedConnections     int64
)

func runClient(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Printf("Client %d starting", id)

	kacp := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             time.Second,
		PermitWithoutStream: true,
	}

	conn, err := grpc.Dial(address,
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		log.Printf("Client %d failed to connect: %v", id, err)
		atomic.AddInt64(&failedConnections, 1)
		return
	}
	defer conn.Close()

	client := chats.NewChatServiceClient(conn)

	stream, err := client.Chat(context.Background())
	if err != nil {
		log.Printf("Client %d error on chat: %v", id, err)
		atomic.AddInt64(&failedConnections, 1)
		return
	}

	// Send messages
	go func() {
		for i := 0; ; i++ {
			message := fmt.Sprintf("Message %d from client %d", i, id)
			if err := stream.Send(&chats.ChatMessage{Message: message}); err != nil {
				log.Printf("Client %d failed to send message: %v", id, err)
				return
			}
			log.Printf("Client %d sent: %s", id, message)
			time.Sleep(5 * time.Second)
		}
	}()
	log.Printf("Client %d sent message", id)

	// Wait for a response
	for {
		resp, err := stream.Recv()
		if err != nil {
			log.Printf("Client %d failed to receive a message: %v", id, err)
			atomic.AddInt64(&failedConnections, 1)
			return
		}
		log.Printf("Client %d received response: %s", id, resp.Message)

		atomic.AddInt64(&successfulConnections, 1)
		log.Printf("Client %d successfully sent and received a message", id)
	}

	// Close the stream explicitly
	stream.CloseSend()
}

func main() {
	start := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go runClient(i, &wg)

		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()
	elapsed := time.Since(start)

	log.Printf("Test completed in %s", elapsed)
	log.Printf("Successful connections: %d", atomic.LoadInt64(&successfulConnections))
	log.Printf("Failed connections: %d", atomic.LoadInt64(&failedConnections))
}
