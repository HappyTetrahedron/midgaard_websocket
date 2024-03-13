/*
midgaard_matrix_bot, a Matrix bot which sets a bridge to MUD

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type WsConfig struct {
	Address string `short:"a" long:"address" description:"Local address at which to bind the websocket server" required:"true"`
}

type WsData struct {
	WsId    uuid.UUID
	SendChannel chan *string
	CancelFunc context.CancelFunc
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 8192

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Time to wait before force close on connection.
	closeGracePeriod = 10 * time.Second
)

var connections map[uuid.UUID]*WsData

func receiveWorker(ws *websocket.Conn, id uuid.UUID) {
	ws.SetReadLimit(maxMessageSize)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	session := getSession(id)
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println("receive error:", id, err)
			errorToSession(session, err)
			break
		}
		log.Println("received")
		message = append(message, '\n')
		msg := string(message)
		sendToSession(session, &msg)
	}
	cleanupWs(id)
}

func sendWorker(sendChannel chan *string, ws *websocket.Conn, id uuid.UUID, ctx context.Context) {
	for {
		select {
		case msg := <-sendChannel:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.TextMessage, []byte(*msg)); err != nil {
				log.Println("write error:", err)
			}
		case <-ctx.Done():
			log.Default().Println("Closing send channel:", id)
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			time.Sleep(closeGracePeriod)
			ws.Close()
			return
		}
	}
}

func ping(ws *websocket.Conn, id uuid.UUID, ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(writeWait)); err != nil {
				log.Println("ping:", id, err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func sendToWs(wsId uuid.UUID, body string) {
	connections[wsId].SendChannel <- &body
}

var upgrader = websocket.Upgrader{}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	id := uuid.New()

	log.Default().Println("adding new connection:", id)

	sendChannel := make(chan *string)
	ctx, cancel := context.WithCancel(context.Background())
	data := WsData{
		SendChannel: sendChannel,
		CancelFunc: cancel,
		WsId: id,
	}
	connections[id] = &data
	// TODO this is where we send shit

	go receiveWorker(ws, id)
	go sendWorker(sendChannel, ws, id, ctx)
	go ping(ws, id, ctx)
}

func cleanupWs(id uuid.UUID) {
	connections[id].CancelFunc()
	delete(connections, id)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func initWebsockets(config WsConfig) error {
	connections = make(map[uuid.UUID]*WsData)
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", serveWs)
	server := &http.Server{
		Addr:              config.Address,
		ReadHeaderTimeout: 3 * time.Second,
	}
	log.Fatal(server.ListenAndServe())

	return nil
}

func cancelWs(wsId uuid.UUID) {
	data, succ := connections[wsId]
	if succ {
		data.CancelFunc()
	}
}
