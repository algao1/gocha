package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	pb "github.com/algao1/watcher/proto"

	_ "image/jpeg"
	_ "image/png"

	"github.com/rivo/tview"
	"google.golang.org/grpc"

	ts "google.golang.org/protobuf/types/known/timestamppb"
)

type chatClient struct {
	user   string
	client pb.ChatClient
	mu     sync.RWMutex

	// Configurations.
	palette   map[string]string
	files     map[string]string
	chunkSize int
}

// newClient initializes a new chatClient.
// Sets up connection with server, and initializes UI.
func newClient(user, target string) *chatClient {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithBlock())
	opts = append(opts, grpc.WithInsecure())

	// Cancel context if connection times out after 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}

	return &chatClient{
		user:   user,
		client: pb.NewChatClient(conn),
		palette: map[string]string{
			"background": "C5C3C6",
			"border":     "4C5C68",
			"title":      "46494C",
			"date":       "E76F51",
			"name":       "1985A1",
			"text":       "577399",
		},
		files:     make(map[string]string),
		chunkSize: 128 * 1024, // 128 KiB
	}
}

// substr returns the substring between start and end.
func substr(s string, start, end int) string {
	counter, startIdx := 0, 0
	for i := range s {
		if counter == start {
			startIdx = i
		}
		if counter == end {
			return s[startIdx:i]
		}
		counter++
	}

	return s[startIdx:]
}

// chunk breaks the file into 'chunks' of specified sizes, and streams it
// to the server.
func (cc *chatClient) chunk(filename string, outCh chan<- *pb.Note, errc chan<- error) {
	data, err := os.ReadFile(fmt.Sprintf("savedfiles/%s", filename))
	if err != nil {
		errc <- fmt.Errorf("%q: %w", "unable to chunk", err)
	}

	// Find the image format.
	fmtReg := regexp.MustCompile(`\.(?:jpg|png)$`)
	format := fmtReg.FindString(filename)

	// Generates a shortened filename for loading locally.
	name := fmtReg.Split(filename, -1)[0]
	if len(name) > 3 {
		name = substr(name, 0, 4)
	}

	cc.mu.Lock()
	cc.files[filename] = fmt.Sprintf("%s_%d%s", name, len(cc.files)+1, format)
	cc.mu.Unlock()

	cc.mu.RLock()
	for cByte := 0; cByte < len(data); cByte += cc.chunkSize {
		var chunk []byte
		if cByte+cc.chunkSize > len(data) {
			chunk = data[cByte:]
		} else {
			chunk = data[cByte : cByte+cc.chunkSize]
		}

		outCh <- &pb.Note{
			Sender: "ftransfer_" + cc.user,
			Event: &pb.Note_Chunk_{
				Chunk: &pb.Note_Chunk{
					Name:   cc.files[filename],
					Chunk:  chunk,
					Format: format,
				},
			},
			TimeStamp: ts.Now(),
		}
	}
	cc.mu.RUnlock()

	errc <- nil
}

// newMessageHandler returns an InputHandler that handles outgoing messages.
func (cc *chatClient) newMessageHandler(outCh chan<- *pb.Note) InputHandler {
	fTransfer := regexp.MustCompile(`\[.*\.(?:jpg|png)\]`)

	handler := func(message string) {
		links := fTransfer.FindAllString(message, -1)
		for _, link := range links {
			errc := make(chan error)
			go cc.chunk(strings.Trim(link, "[]"), outCh, errc)

			if err := <-errc; err != nil {
				message = strings.Replace(message, link,
					fmt.Sprintf("%s: [::du]%s[::-]", link, err.Error()), -1)
			} else {
				cc.mu.RLock()
				message = strings.Replace(message, link,
					fmt.Sprintf("%s: [::du]%s[::-]", link, cc.files[strings.Trim(link, "[]")]), -1)
				cc.mu.RUnlock()
			}
		}

		cc.mu.RLock()
		outCh <- &pb.Note{
			Sender:    cc.user,
			Event:     &pb.Note_Message{Message: message},
			TimeStamp: ts.Now(),
		}
		cc.mu.RUnlock()
	}

	return handler
}

// updateChatBox updates chatBox component with incoming messages.
func (cc *chatClient) updateChatBox(inCh <-chan *pb.Note, app *tview.Application,
	chatBox *tview.TextView) {
	var prevSender string
	var prevTime time.Time

	for in := range inCh {
		inTime := in.TimeStamp.AsTime().UTC()

		switch in.GetEvent().(type) {
		case *pb.Note_Message:
			cc.mu.RLock()
			app.QueueUpdateDraw(func() {
				if in.Sender != prevSender || inTime.After(prevTime.Add(5*time.Minute)) {
					fmt.Fprintf(chatBox, "[#%s]%s - [#%s]%s\n",
						cc.palette["name"],
						in.Sender,
						cc.palette["date"],
						inTime.Format(time.UnixDate),
					)
				}
				fmt.Fprintf(chatBox, "[#%s]%s", cc.palette["text"], in.GetMessage())
			})
			cc.mu.RUnlock()
		case *pb.Note_Chunk_:
			f, err := os.OpenFile(fmt.Sprintf("savedfiles/%s", in.GetChunk().GetName()),
				os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write(in.GetChunk().GetChunk()); err != nil {
				log.Fatal(err)
			}
			if err := f.Close(); err != nil {
				log.Fatal(err)
			}
		}

		prevSender = in.Sender
		prevTime = inTime
	}
}

// setScreen initializes the layout and UI components.
func (cc *chatClient) setScreen(inCh <-chan *pb.Note, outCh chan<- *pb.Note) {
	app := tview.NewApplication()

	// Generate UI components.
	cc.mu.RLock()
	chatBox := NewChatBox(cc.palette)
	inputField := NewChatInput(cc.palette, cc.newMessageHandler(outCh))
	cc.mu.RUnlock()

	// Layout the widgets in flex view.
	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(chatBox, 0, 1, false).
		AddItem(inputField, 3, 0, false)

	go cc.updateChatBox(inCh, app, chatBox)

	err := app.SetRoot(flex, true).SetFocus(inputField).EnableMouse(true).Run()
	if err != nil {
		log.Fatal(err)
	}
}

// Open bi-directional stream between client and server.
func (cc *chatClient) runChat(inCh chan<- *pb.Note, outCh <-chan *pb.Note) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := cc.client.Stream(ctx)
	if err != nil {
		log.Fatalf("%v.RouteChat(_) = _, %v", cc.client, err)
	}

	// Sends a blank Note to the server to add current user to collection.
	cc.mu.RLock()
	err = stream.Send(&pb.Note{Sender: cc.user})
	if err != nil {
		panic(err)
	}
	cc.mu.RUnlock()

	// Receives incoming messages on a separate goroutine to be non-blocking.
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Fatalf("failed to receive a note: %v", err)
			}

			inCh <- in
		}
	}()

	// Forward/send outgoing messages.
	for msg := range outCh {
		if err := stream.Send(msg); err != nil {
			log.Fatalf("failed to send note: %v", err)
		}
	}
}

func main() {
	name := os.Args[1]

	go http.ListenAndServe(":1747", InitRouter())

	// Initialize necessary channels.
	inCh := make(chan *pb.Note)
	outCh := make(chan *pb.Note)

	// Initialize client.
	cc := newClient(name, "localhost:10000")
	go cc.runChat(inCh, outCh)
	cc.setScreen(inCh, outCh)
}
