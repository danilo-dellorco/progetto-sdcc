package main

import (
	"fmt"
	chord "progetto-sdcc/node/chord/net"
	"progetto-sdcc/node/localsys/structures"
	"progetto-sdcc/utils"
)

/*
Pseudo-Interfaccia che verrà registrata dal server in modo tale che il client possa invocare i metodi tramite RPC
ciò che si registra realmente è un oggetto che prevede l'implementazione di quei metodi specifici
*/
type RPCservice struct {
	node chord.ChordNode
	db   structures.MongoClient
}

/*
Parametri per le operazioni di Get e Delete
*/
type Args1 struct {
	key string
}

/*
Parametri per le operazioni di Put e Update
*/
type Args2 struct {
	key   string
	value string
}

/*
Effettua la RPC per la Get di una key.
 1) Lookup per trovare il nodo che hosta una risorsa
 2) RPC effettiva di GET verso quel nodo chord
*/
func (s *RPCservice) GetRPC(args *Args1, reply *[]string) error {
	node := s.node
	// TODO vedere se può partire anche dal nodo stesso invece di node.GetSuccessor().GetIpAddr()
	addr, err := chord.Lookup(utils.HashString(args.key), node.GetIpAddress())
	if addr == node.GetIpAddress() {

	}
	// [TODO] rpc.call(GetFuncRPC,addr)
	fmt.Println(addr, err)

	return nil
}

/*
Effettua la RPC per inserire un'entry nello storage.
 1) Lookup per trovare il nodo che deve hostare la risorsa
 2) RPC effettiva di PUT verso quel nodo chord
*/
func (s *RPCservice) PutRPC(args *Args2, reply *[]string) error {
	node := s.node
	// TODO vedere se può partire anche dal nodo stesso invece di node.GetSuccessor().GetIpAddr()
	addr, err := chord.Lookup(utils.HashString(args.key), node.GetIpAddress())
	// [TODO] rpc.call(GetFuncRPC,addr)
	fmt.Println(addr, err)

	return nil
}

/*
Effettua la RPC per aggiornare un'entry nello storage.
 1) Lookup per trovare il nodo che hosta la risorsa
 2) RPC effettiva di UPDATE verso quel nodo chord
*/
func (s *RPCservice) UpdateRPC(args *Args1, reply *[]string) error {
	node := s.node
	// TODO vedere se può partire anche dal nodo stesso invece di node.GetSuccessor().GetIpAddr()
	addr, err := chord.Lookup(utils.HashString(args.key), node.GetIpAddress())
	// [TODO] rpc.call(GetFuncRPC,addr)
	fmt.Println(addr, err)

	return nil
}

/*
Effettua la RPC per eliminare un'entry nello storage.
 1) Lookup per trovare il nodo che hosta la risorsa
 2) RPC effettiva di DELETE verso quel nodo chord
*/
func (s *RPCservice) DeleteRPC(args *Args1, reply *[]string) error {
	node := s.node
	// TODO vedere se può partire anche dal nodo stesso invece di node.GetSuccessor().GetIpAddr()
	addr, err := chord.Lookup(utils.HashString(args.key), node.GetIpAddress())
	// [TODO] rpc.call(GetFuncRPC,addr)
	fmt.Println(addr, err)

	return nil
}

/*
Effettua il get. Scrive in reply la stringa contenente l'entry richiesta. Se l'entry
non è stata trovata restituisce un messaggio di errore.
*/
func (s *RPCservice) get_impl(args *Args1, reply string) {
	//entry := s.db.GetEntry(args.key)
	//reply =
}

/*
Effettua il PUT. Ritorna 0 se l'operazione è avvenuta con successo, altrimenti l'errore specifico
*/
func (s *RPCservice) put_impl(args *Args2) error {
	arg1 := args.key
	arg2 := args.value
	err := s.db.PutEntry(arg1, arg2)
	return err
}

func (s *RPCservice) update_impl(args *Args1, reply *structures.MongoEntry) {
	entry := s.db.GetEntry(args.key)
	reply = entry
}

func (s *RPCservice) delete_impl(args *Args1, reply *structures.MongoEntry) {
	entry := s.db.GetEntry(args.key)
	reply = entry
}
