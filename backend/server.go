package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"

	pb "github.com/gerow/chat/proto"
)

const (
	port = ":50051"
)

type channel struct {
	sync.RWMutex
	name  string
	users map[string]*user
}

func (c *channel) send(user, message string) error {
	c.RLock()
	defer c.RUnlock()
	if c.users[user] == nil {
		return fmt.Errorf("user %s is not in channel %s", user, c.name)
	}
	for _, u := range c.users {
		if u.name == user {
			continue
		}
		if err := u.stream.Send(&pb.Message{Message: &pb.Message_ChatMessage{ChatMessage: &pb.ChatMessage{
			Channel: c.name,
			Sender:  user,
			Content: message}},
		}); err != nil {
			log.Printf("failed to notify %s of message %s from %s in channel %s: %v", u.name, message, user, c.name, err)
		}
	}

	log.Printf("user %s sent message %q in channel %s", user, message, c.name)

	return nil
}

func (c *channel) part(user string) error {
	c.Lock()
	defer c.Unlock()
	if c.users[user] == nil {
		return fmt.Errorf("user %s is not in channel %s", user, c.name)
	}
	for _, u := range c.users {
		if u.name == user {
			continue
		}
		if err := u.stream.Send(&pb.Message{Message: &pb.Message_Part{Part: &pb.Part{
			Channel: c.name,
			User:    user,
		}}}); err != nil {
			log.Printf("failed to notify %s that %s parted channel %s: %v", u.name, user, c.name, err)
		}
	}
	delete(c.users, user)

	log.Printf("user %s parted channel %s", user, c.name)

	return nil
}

func (c *channel) join(u *user) error {
	c.Lock()
	defer c.Unlock()
	if c.users[u.name] != nil {
		return fmt.Errorf("user %s is already in channel %s", u.name, c.name)
	}
	log.Printf("checking users for channel %s, map is %v", c.name, c.users)
	for _, u2 := range c.users {
		log.Printf("notifying %s that %s is parting from channel %s", u2.name, u.name, c.name)
		if err := u2.stream.Send(&pb.Message{Message: &pb.Message_Join{Join: &pb.Join{
			Channel: c.name,
			User:    u.name,
		}}}); err != nil {
			log.Printf("failed to notify %s that %s joined channel %s: %v", u2.name, u.name, c.name, err)
		}
	}
	c.users[u.name] = u

	log.Printf("user %s joined channel %s", u.name, c.name)

	return nil
}

type user struct {
	name     string
	channels map[string]*channel
	stream   pb.Chat_ChatServer
}

type server struct {
	pb.UnimplementedChatServer
	sync.RWMutex

	users    map[string]*user
	channels map[string]*channel
}

func (s *server) Chat(stream pb.Chat_ChatServer) error {
	// Expect to receive a Hello message first.
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	log.Print("new client connected")
	hm := msg.GetHello()
	if hm == nil {
		return errors.New("received non-hello message; please say hello first")
	}

	u := user{
		name:     hm.GetName(),
		channels: make(map[string]*channel),
		stream:   stream,
	}
	s.Lock()
	if s.users[u.name] != nil {
		return fmt.Errorf("user with name %s already logged in", u.name)
	}
	s.users[u.name] = &u
	s.Unlock()
	// cleanup
	defer func() {
		s.Lock()
		delete(s.users, u.name)
		s.Unlock()
		for _, c := range s.channels {
			c.part(u.name)
		}
	}()

	log.Printf("accepted new client %s", u.name)
	for {
		msg, err = stream.Recv()
		if err != nil {
			return err
		}
		switch m := msg.Message.(type) {
		case *pb.Message_ChatMessage:
			if err := func() error {
				s.RLock()
				defer s.RUnlock()
				channel := m.ChatMessage.Channel
				content := m.ChatMessage.Content
				// Ignore sender.
				if s.channels[channel] == nil {
					return fmt.Errorf("no such channel %s", channel)
				}
				return s.channels[channel].send(u.name, content)

			}(); err != nil {
				return err
			}
		case *pb.Message_Join:
			if err := func() error {
				// Full lock in case we need to add the channel.
				s.Lock()
				defer s.Unlock()
				name := m.Join.Channel
				if s.channels[name] == nil {
					s.channels[name] = &channel{
						name:  name,
						users: make(map[string]*user),
					}
				}
				return s.channels[name].join(&u)
			}(); err != nil {
				return err
			}
		case *pb.Message_Part:
			if err := func() error {
				s.RLock()
				defer s.RUnlock()
				name := m.Part.Channel
				if s.channels[name] == nil {
					return fmt.Errorf("no such channel %s", name)
				}
				return s.channels[name].part(u.name)
			}(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("message has unexpected type %T", m)
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	gs := grpc.NewServer()
	s := server{
		users:    make(map[string]*user),
		channels: make(map[string]*channel),
	}
	pb.RegisterChatServer(gs, &s)
	log.Printf("server listening at %v", lis.Addr())
	if err := gs.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
