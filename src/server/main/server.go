package main

import (
	"os"
	"fmt"
	"../main/services"
	"net/rpc"
)

func main() {
	if len(os.Args)!=1{
		fmt.Printf("Usage: go run server.go\n")
	}
	fmt.Printf("Server Waiting For Connection\n")

	service := services.InitializeService()
	rpc.Register(service)
	rpc.HandleHTTP()
	services.ListenHttpConnection()
}
