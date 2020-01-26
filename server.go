package main

import (
	"flag"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/fhs/gompd/mpd"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}
var server Server
var jukebox Jukebox

type Server struct {
	connections map[*websocket.Conn]bool
	mux         sync.Mutex
}

func initServer() Server {
	return Server{connections: make(map[*websocket.Conn]bool)}
}

func (server *Server) removeConnection(c *websocket.Conn) {
	server.mux.Lock()
	delete(server.connections, c)
	server.mux.Unlock()
}

func (server *Server) addConnection(c *websocket.Conn) {
	server.mux.Lock()
	server.connections[c] = true
	server.mux.Unlock()
}

func socketinit(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Websocket upgrade error: ", err)
	}
	server.addConnection(c)
	defer c.Close()
	c.SetCloseHandler(func(code int, text string) error {
		server.removeConnection(c)
		return nil
	})

	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println(err)
			server.removeConnection(c)
			return
		}

		if validateName(message) {
			c.WriteMessage(mt, []byte("ok"))
			defer socketHandle(c, message)
			return
		}
		c.WriteMessage(mt, []byte("error"))
		continue
	}
}

func socketHandle(c *websocket.Conn, name []byte) {
	sendState()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			server.removeConnection(c)
			log.Println(err)
			break
		}
		switch messageTokens := strings.Split(string(message), " "); messageTokens[0] {
		case "ping":
			c.WriteMessage(mt, []byte("pong"))
		case "skip":
			if err := jukebox.SkipSong(); err != nil {
				log.Println(err)
			}
		case "volume":
			if volume, err := strconv.Atoi(messageTokens[1]); err != nil {
				jukebox.SetVolume(volume)
			}
		case "remove":
			songPosition, err := strconv.Atoi(messageTokens[1])
			if err != nil {
				log.Println("Invalid queue position specified for removal")
				continue
			}
			jukebox.RemoveSong(string(name), songPosition)
		case "queue":
			songurl := messageTokens[1]
			err := jukebox.AddSongURL(string(name), songurl)
			if err != nil {
				log.Println("Error fetching song for url :", songurl, ", error: ", err)
				continue
			}
			c.WriteMessage(1, []byte("ok"))
		default:
			log.Print("Illegal command: ", messageTokens[0])
		}
		sendState()
	}
}

func sendState() {
	bytestate := []byte(jukebox.GetState())
	server.mux.Lock()
	defer server.mux.Unlock()
	for c := range server.connections {
		c.WriteMessage(1, bytestate)
	}
}

func validateName(name []byte) bool {
	if len(name) <= 20 && len(name) > 0 {
		match, _ := regexp.Match("^[a-zA-Z0-9]*$", name)
		return match
	}
	return false
}

func main() {
	var port string
	flag.StringVar(&port, "port", "8080", "Server port number")
	var mpdport string
	flag.StringVar(&mpdport, "mpdport", "6600", "MPD port number")
	flag.Parse()
	// MPD Client connection
	conn, err := mpd.Dial("tcp", "localhost:"+mpdport)
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()
	if err := conn.Consume(true); err != nil { // Remove song when finished playing
		log.Fatalln(err)
	}
	// Manages state and player
	jukebox = NewJukebox(conn)
	// Watcher set up - check when songs start and end
	w, err := mpd.NewWatcher("tcp", "localhost:"+mpdport, "", "player")
	if err != nil {
		log.Fatalln(err)
	}
	defer w.Close()
	go func() {
		for range w.Event {
			status, err := conn.Status()
			if err != nil {
				log.Println(err)
			}
			switch status["state"] {
			case "play":
			case "stop": // When music stops, move onto the next song
				if err := jukebox.CycleSong(); err != nil {
					log.Println(err)
				}
				sendState()
			case "pause":
			}
		}
	}()

	// Basic test #1 https://www.youtube.com/watch?v=otdOnrgtyfI
	// Basic test #2 https://soundcloud.com/futureisnow/perkys-calling-prod-by-southside
	// Playlist test https://www.youtube.com/watch?v=W3J9-OvxNpo&list=PLmIf0JO7SvbKxGuse9T19m_mHBm4oNG7y

	server = initServer()
	fs := http.FileServer(http.Dir("./priv"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", socketinit)
	log.Println("Starting up server on port: " + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
