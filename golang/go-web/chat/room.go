package main

import (
	"log"
	"net/http"

	"./trace"
	"github.com/gorilla/websocket"
	"github.com/stretchr/objx"
)

type room struct {
	// forward は他のクライアントに転送するためのメッセージを保持するためのチャネル
	forward chan *message
	// join はチャットルームに参加しているクライアントのためのチャネル
	join chan *client
	// leave はチャットルームから退出しようとしているクライアントのためのチャネル
	leave chan *client
	// clients には材質しているすべてのクライアントが保持される
	clients map[*client]bool
	// tracer はチャットルーム上で行われた操作ログを受け取る
	tracer trace.Tracer
	// avatar はアバターの情報
	avatar Avatar
}

// 1.2.5 ヘルパー関数
// これを呼び出すだけでチャットルームを生成できるから便利
func newRoom(avatar Avatar) *room {
	return &room{
		forward: make(chan *message),
		join:    make(chan *client),
		leave:   make(chan *client),
		clients: make(map[*client]bool),
		tracer:  trace.Off(),
		avatar:  avatar,
	}
}

// 1.2.3 並行プログラミングで使われるGoのイディオム
func (r *room) run() {
	for {
		select {
		case client := <-r.join:
			// 参加
			r.clients[client] = true
			r.tracer.Trace("新しいクライアントが参加しました")
		case client := <-r.leave:
			// 退室
			delete(r.clients, client)
			close(client.send)
			r.tracer.Trace("クライアントが退室しました")
		case msg := <-r.forward:
			r.tracer.Trace("メッセージを受信しました: ", msg.Message)
			// すべてのクライアントに対しメッセージを転送
			for client := range r.clients {
				select {
				case client.send <- msg:
					// メッセージを送信
					r.tracer.Trace(" -- クライアントに送信されました")
				default:
					// 送信に失敗
					delete(r.clients, client)
					close(client.send)
					r.tracer.Trace(" -- 送信に失敗しました.クライアントをクリーンアップします")
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

	authCookie, err := req.Cookie("auth")
	if err != nil {
		log.Fatal("クッキーの取得に失敗しました:", err)
		return
	}

	client := &client{
		socket:   socket,
		send:     make(chan *message, messageBufferSize),
		room:     r,
		userData: objx.MustFromBase64(authCookie.Value),
	}
	r.join <- client
	defer func() { r.leave <- client }()
	go client.write()
	client.read()
}
