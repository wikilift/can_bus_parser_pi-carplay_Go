package main

import (
	"log"
	"net/http"

	"can-service/internal/canbus"
	"can-service/internal/repository"
	"can-service/internal/websockets"
)

func main() {
	log.Println("starting can-service")

	repo := repository.NewInMemoryRepository()
	canListener := canbus.NewCanListener("can0")
	wsServer := websockets.NewServer(repo, canListener)

	wsServer.ConsumeReadings(canListener.Readings())

	http.Handle("/ws", wsServer)

	log.Println("listening on ws://0.0.0.0:8080/ws")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
