package main

import (
	"context"
	chats "distributed-realtime-chat/proto"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

func runClient(id int, address string, wg *sync.WaitGroup) {
	defer wg.Done()

	kacp := keepalive.ClientParameters{
		Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
		Timeout:             time.Second,      // wait 1 second for ping ack before considering the connection dead
		PermitWithoutStream: true,             // send pings even without active streams
	}

	conn, err := grpc.Dial(address,
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		log.Printf("Client %d failed to connect: %v", id, err)
		return
	}
	defer conn.Close()

	client := chats.NewChatServiceClient(conn)

	for {
		stream, err := client.Chat(context.Background())
		if err != nil {
			log.Printf("Client %d error on chat: %v", id, err)
			time.Sleep(5 * time.Second) // Wait before retrying
			continue
		}

		waitc := make(chan struct{})

		go func() {
			for {
				in, err := stream.Recv()
				if err == io.EOF {
					close(waitc)
					return
				}
				if err != nil {
					log.Printf("Client %d failed to receive a message: %v", id, err)
					close(waitc)
					return
				}
				log.Printf("Client %d received: %s", id, in.Message)
			}
		}()

		for i := 0; ; i++ {
			message := chats.ChatMessage{Message: fmt.Sprintf("Hello from client %d, message %d", id, i)}
			if err := stream.Send(&message); err != nil {
				log.Printf("Client %d failed to send a message: %v", id, err)
				break
			}
			time.Sleep(5 * time.Second)
		}

		<-waitc
		log.Printf("Client %d restarting connection", id)
	}
}

func main() {
	var wg sync.WaitGroup
	for i := 0; i < 100000; i++ {
		wg.Add(1)
		go runClient(i, "localhost:80", &wg)
		if i%1000 == 0 {
			time.Sleep(time.Second) // Rate limit client creation
		}
	}
	wg.Wait()
}
