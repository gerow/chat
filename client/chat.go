package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gerow/chat/client/screen"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	pb "github.com/gerow/chat/proto"
)

func run(c pb.ChatClient, name string) error {
	ctx := context.Background()

	g, ctx := errgroup.WithContext(ctx)
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
	s, err := screen.New()
	if err != nil {
		return err
	}

	// And then spawn off two routines: one to handle input and one to print messages we receive.
	g.Go(func() error {
		for {
			msg, err := stream.Recv()
			if err != nil {
				log.Printf("got recv err %v", err)
				return err
			}
			switch m := msg.Message.(type) {
			case *pb.Message_ChatMessage:
				channel := m.ChatMessage.Channel
				content := m.ChatMessage.Content
				sender := m.ChatMessage.Sender
				s.AddLine(channel, sender+": "+content)
			case *pb.Message_Join:
				channel := m.Join.Channel
				user := m.Join.User
				s.AddLine(channel, "* "+user+" has joined the channel *")
			case *pb.Message_Part:
				channel := m.Part.Channel
				user := m.Part.User
				s.AddLine(channel, "* "+user+" has parted the channel *")
			default:
				return fmt.Errorf("message has unexpected type %T", m)
			}
		}
	})
	g.Go(func() error {
		for {
			log.Print("attempting to get entry")
			e := s.GetEntry()
			log.Printf("got entry %v", e)
			t := e.Line
			channel := e.Channel
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
				s.JoinChannel(channel)
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
			case t == "/quit":
				s.Fini()
				os.Exit(0)
			default:
				if err := stream.Send(&pb.Message{Message: &pb.Message_ChatMessage{ChatMessage: &pb.ChatMessage{
					Channel: channel,
					Content: t,
				}}}); err != nil {
					return err
				}
				s.AddLine(channel, name+": "+t)
			}
		}
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
