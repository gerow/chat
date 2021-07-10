package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	pb "github.com/gerow/chat/proto"
)

func run(c pb.ChatClient, name string) error {
	ctx := context.Background()

	stream, err := c.Chat(ctx)
	if err != nil {
		return err
	}
	// Say hello.
	if err := stream.Send(&pb.Message{Message: &pb.Message_Hello{Hello: &pb.Hello{
		Name: name,
	}}}); err != nil {
		return err
	}

	// And then spawn off two routines: one to handle input and one to print messages we receive.
	var g errgroup.Group
	g.Go(func() error {
		for {
			msg, err := stream.Recv()
			if err != nil {
				return err
			}
			switch m := msg.Message.(type) {
			case *pb.Message_ChatMessage:
				channel := m.ChatMessage.Channel
				content := m.ChatMessage.Content
				sender := m.ChatMessage.Sender
				fmt.Printf("[%s] %s: %s\n", channel, sender, content)
			case *pb.Message_Join:
				channel := m.Join.Channel
				user := m.Join.User
				fmt.Printf("[%s] * %s has joined the channel *\n", channel, user)
			case *pb.Message_Part:
				channel := m.Part.Channel
				user := m.Part.User
				fmt.Printf("[%s] * %s has parted the channel *\n", channel, user)
			default:
				return fmt.Errorf("message has unexpected type %T", m)
			}
		}
	})
	g.Go(func() error {
		s := bufio.NewScanner(os.Stdin)
		log.Print("beginning scan...")
		for s.Scan() {
			log.Print("scanned a line")
			t := s.Text()
			switch {
			case strings.HasPrefix(t, "/join"):
				fields := strings.Fields(t)
				if len(fields) != 2 {
					log.Printf("/join expects exactly 1 argument")
					continue
				}
				channel := fields[1]
				if err := stream.Send(&pb.Message{Message: &pb.Message_Join{Join: &pb.Join{
					Channel: channel,
				}}}); err != nil {
					return err
				}
			case strings.HasPrefix(t, "/part"):
				fields := strings.Fields(t)
				if len(fields) != 2 {
					log.Printf("/part expects exactly 1 argument")
					continue
				}
				channel := fields[1]
				if err := stream.Send(&pb.Message{Message: &pb.Message_Part{Part: &pb.Part{
					Channel: channel,
				}}}); err != nil {
					return err
				}
			default:
				if err := stream.Send(&pb.Message{Message: &pb.Message_ChatMessage{ChatMessage: &pb.ChatMessage{
					Channel: "test",
					Content: t,
				}}}); err != nil {
					return err
				}
			}
		}
		return s.Err()
	})

	return g.Wait()
}

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := pb.NewChatClient(conn)

	if err := run(c, os.Args[1]); err != nil {
		log.Fatal(err)
	}
}