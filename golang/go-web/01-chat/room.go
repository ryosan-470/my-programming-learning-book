package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

type room struct {
	// forward は他のクライアントに転送するためのメッセージを保持するためのチャネル
	forward chan []byte
	// join はチャットルームに参加しているクライアントのためのチャネル
	join chan *client
	// leave はチャットルームから退出しようとしているクライアントのためのチャネル
	leave chan *client
	// clients には材質しているすべてのクライアントが保持される
	clients map[*client]bool
}

// 1.2.5 ヘルパー関数
// これを呼び出すだけでチャットルームを生成できるから便利
func newRoom() *room {
	return &room{
		forward: make(chan []byte),
		join:    make(chan *client),
		leave:   make(chan *client),
		clients: make(map[*client]bool),
	}
}

// 1.2.3 並行プログラミングで使われるGoのイディオム
func (r *room) run() {
	for {
		select {
		case client := <-r.join:
			// 参加
			r.clients[client] = true
		case client := <-r.leave:
			// 退室
			delete(r.clients, client)
			close(client.send)
		case msg := <-r.forward:
			// すべてのクライアントに対しメッセージを転送
			for client := range r.clients {
				select {
				case client.send <- msg:
					// メッセージを送信
				default:
					// 送信に失敗
					delete(r.clients, client)
					close(client.send)
				}
			}
		}
	}
}

// 1.2.4 チャットルームをHTTPハンドラにする
const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

var upgrader = &websocket.Upgrader{ReadBufferSize: socketBufferSize, WriteBufferSize: socketBufferSize}

func (r *room) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	socket, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		log.Fatal("ServeHTTP:", err)
		return
	}
	client := &client{
		socket: socket,
		send:   make(chan []byte, messageBufferSize),
		room:   r,
	}
	r.join <- client
	defer func() { r.leave <- client }()
	go client.write()
	client.read()
}
