package main

import (
	"fmt"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"

	pb "watcher/proto"

	ts "google.golang.org/protobuf/types/known/timestamppb"
)

type chatServer struct {
	pb.UnimplementedChatServer

	mu      sync.RWMutex
	clients map[string]chan *pb.Note
}

// newServer initializes a chatServer.
func newServer() *chatServer {
	s := &chatServer{clients: make(map[string]chan *pb.Note)}
	return s
}

// broadcast popoulates all available channels with the Note.
func (s *chatServer) broadcast(note *pb.Note) {
	for _, ch := range s.clients {
		ch <- note
	}
}

// listen receives Notes from the stream, and broadcasts it.
func (s *chatServer) listen(sender string, stream pb.Chat_StreamServer,
	errc chan<- error) {
	for {
		in, err := stream.Recv()
		if err != nil {
			errc <- err
			return
		}

		log.Println("received note from: " + sender)
		s.broadcast(in)
	}
}

// forward forwards Notes from the corresponding client channel through
// the bi-directional stream.
func (s *chatServer) forward(sender string, stream pb.Chat_StreamServer,
	errc <-chan error) error {
	for {
		select {
		case err := <-errc:
			return err
		case msg := <-s.clients[sender]:
			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}

// StreamChat implements the bi-directional chat stream.
func (s *chatServer) Stream(stream pb.Chat_StreamServer) error {
	// The first incoming message contains only sender information.
	in, err := stream.Recv()
	if err != nil {
		return err
	}

	sender := in.GetSender()
	errc := make(chan error)

	// Adds a corresponding channel to listen for messages.
	s.mu.Lock()
	s.clients[sender] = make(chan *pb.Note)
	log.Printf("%s logged in", sender)
	s.mu.Unlock()

	// Listens to messages on a separate goroutine.
	go s.listen(sender, stream, errc)

	// Broadcasts the 'login' message on a separate goroutine to prevent blocking.
	loginMsg := &pb.Note_Message{
		Message: fmt.Sprintf("%s logged in!\n", sender),
	}
	go s.broadcast(&pb.Note{
		Sender:    "SERVER",
		Event:     loginMsg,
		TimeStamp: ts.Now()},
	)

	// Defer broadcasting of 'logout' message.
	defer func() {
		s.mu.Lock()
		delete(s.clients, sender)
		logoutMsg := &pb.Note_Message{
			Message: fmt.Sprintf("%s logged out!\n", sender),
		}
		s.broadcast(&pb.Note{
			Sender:    "SERVER",
			Event:     logoutMsg,
			TimeStamp: ts.Now()},
		)
		log.Printf("%s logged out\n", sender)
		s.mu.Unlock()
	}()

	return s.forward(sender, stream, errc)
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 10000))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	pb.RegisterChatServer(grpcServer, newServer())
	grpcServer.Serve(lis)
}
