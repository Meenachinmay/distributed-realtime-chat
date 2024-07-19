package main

import (
	"context"
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/internal/client"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	serverAddr := "localhost:50051"
	userID := fmt.Sprintf("user_%d", os.Getpid())
	roomID := "FUJI123"

	chatClient, err := client.NewChatClient(serverAddr, 10)
	if err != nil {
		log.Fatalf("Failed to create chat client: %v", err)
	}
	defer chatClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = chatClient.Connect(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to server: %v", err)
	}

	msgChan := make(chan *chat.ChatMessage)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		chatClient.ReceiveMessages(msgChan)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for msg := range msgChan {
			fmt.Printf("Received message from %s: %s\n", msg.UserId, msg.Content)
		}
	}()

	// Send initial message to join the room
	err = chatClient.SendMessage(&chat.ChatMessage{
		UserId:  userID,
		Content: "Joined the chat",
		RoomId:  roomID,
	})
	if err != nil {
		log.Printf("Failed to send initial message: %v", err)
	}

	// Handle user input
	go func() {
		for {
			var input string
			fmt.Print("Enter message: ")
			fmt.Scanln(&input)
			err := chatClient.SendMessage(&chat.ChatMessage{
				UserId:  userID,
				Content: input,
				RoomId:  roomID,
			})
			if err != nil {
				log.Printf("Failed to send message: %v", err)
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	cancel()
	wg.Wait()
}
