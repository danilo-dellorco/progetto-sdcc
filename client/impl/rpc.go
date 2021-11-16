package impl

import (
	"errors"
	"fmt"
	"log"
	"net/rpc"
	"progetto-sdcc/utils"
	"time"
)

var GET string = "RPCservice.GetRPC"
var PUT string = "RPCservice.PutRPC"
var DEL string = "RPCservice.DeleteRPC"
var APP string = "RPCservice.AppendRPC"

/*
Struttura che mantiene i parametri delle RPC
*/
type Args struct {
	Key     string
	Value   string
	Handler string
	Deleted bool
}

/*
Effettua la RPC per la GET
*/
func GetRPC(key string) {
	args := Args{}
	args.Key = key

	var reply *string

	c := make(chan error)

	client, _ := HttpConnect()
	go CallRPC(GET, client, args, reply, c)
	rr1_timeout(GET, client, args, reply, c)
}

/*
Effettua la RPC per il PUT
*/
func PutRPC(key string, value string) {
	args := Args{}
	args.Key = key
	args.Value = value

	var reply *string

	c := make(chan error)

	client, _ := HttpConnect()
	go CallRPC(PUT, client, args, reply, c)
	rr1_timeout(PUT, client, args, reply, c)
}

/*
Effettua la RPC per l'APPEND
*/
func AppendRPC(key string, value string) {
	args := Args{}
	args.Key = key
	args.Value = value

	var reply *string

	c := make(chan error)

	client, _ := HttpConnect()
	go CallRPC(APP, client, args, reply, c)
	rr1_timeout(APP, client, args, reply, c)
}

/*
Effettua la RPC per il DELETE
*/
func DeleteRPC(key string) {
	args := Args{}
	args.Key = key

	var reply *string

	c := make(chan error)

	client, _ := HttpConnect()
	go CallRPC(DEL, client, args, reply, c)
	rr1_timeout(DEL, client, args, reply, c)
}

/*
Goroutine per l'implementazione della semantica at-least-once.
La ritrasmissione viene effettuata fino a 5 volte, altrimenti si assume che il server sia crashato.
*/
func rr1_timeout(rpc string, client *rpc.Client, args Args, reply *string, c chan error) {
	check := make(chan bool)
	res := errors.New("")
	i := 0
	for i = 0; i < utils.RR1_RETRIES; i++ {
		go check_timeout(check)
		select {
		// scade timer per la ritrasmissione
		case <-check:
			fmt.Println("Timeout elapsed, start retransmitting...")
			go CallRPC(rpc, client, args, reply, c)
			continue
		// arriva risposta dal server
		case res = <-c:
			val := res.Error()
			fmt.Println(val)
		}
		//si interrompe la ritrasmissione quando si riceve la prima risposta
		if res.Error() == "Success" {
			break
		}
	}

	//effettuate tutte le ritrasmissioni possibili e non si riceve alcuna risposta
	if i == 5 && res.Error() != "Success" {
		fmt.Println("Server unreachable!")
	}
}

/*
Effettua una generica RPC, utilizzata per implementare il meccanismo RR1 per la semantica at-least-once
*/
func CallRPC(rpc string, client *rpc.Client, args Args, reply *string, c chan error) {
	err := client.Call(rpc, args, &reply)
	defer client.Close()
	if err != nil {
		c <- err
		log.Fatal("RPC error: ", err)
	} else {
		c <- errors.New("Success")
		fmt.Println("Risposta RPC:", *reply)
		return
	}
}

func check_timeout(check chan bool) {
	time.Sleep(utils.RR1_TIMEOUT)
	check <- true
}
