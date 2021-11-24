package impl

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/rpc"
	"os"
	chord "progetto-sdcc/node/chord/api"
	mongo "progetto-sdcc/node/mongo/api"
	"progetto-sdcc/node/mongo/communication"
	"progetto-sdcc/utils"
	"sync"
	"time"
)

var first bool
var sendMutex *sync.Mutex
var recvMutex *sync.Mutex
var migrMutex *sync.Mutex

/*
Esegue tutte le attività per rendere il nodo UP & Running
*/
func InitNode(node *Node) {
	utils.PrintHeaderL1("NODE SETUP")
	InitHealthyNode(node)
	InitListeningServices(node)
	time.Sleep(1 * time.Millisecond)

	InitChordDHT(node)

	GetPredecessorEntries(node)
	InitRPCService(node)
	utils.PrintLineL1()
}

/*
Permette al nodo di essere rilevato come Healthy Instance dal Load Balancer e configura il DB locale
*/
func InitHealthyNode(node *Node) {
	utils.PrintHeaderL2("Initializing EC2 node")
	// Configura il sistema di storage locale
	node.MongoClient = mongo.InitLocalSystem()

	// Inizia a ricevere gli HeartBeat dal LB
	go StartHeartBeatListener()

	// Inizia a inviare valori poco acceduti su S3
	go node.MongoClient.CheckRarelyAccessed()

	// Attende di diventare healthy per il Load Balancer
	utils.PrintTs("Waiting for ELB Health Checking...")
	time.Sleep(utils.NODE_HEALTHY_TIME)
	utils.PrintTs("EC2 Node Up & Running!")
}

/*
Permette al nodo di entrare a far parte della DHT Chord in base alle informazioni ottenute dal Service Registry.
Inizia anche due routine per aggiornamento periodico delle FT del nodo stesso e degli altri nodi della rete
*/
func InitChordDHT(node *Node) {
	utils.PrintHeaderL2("Initializing Chord DHT")

	// Setup dei Flags
	addressPtr := flag.String("addr", "", "the port you will listen on for incomming messages")
	joinPtr := flag.String("join", "", "an address of a server in the Chord network to join to")
	flag.Parse()

	// Ottiene l'indirizzo IP dell'host utilizzato nel VPC
	*addressPtr = GetOutboundIP().String()
	node.ChordClient = new(chord.ChordNode)

	// Controlla le istanze attive contattando il Service Registry per entrare nella rete
waitLB:
	result := JoinDHT(utils.REGISTRY_IP)
	for {
		if len(result) == 0 {
			result = JoinDHT(utils.REGISTRY_IP)
		} else {
			break
		}
	}

	// Unica istanza attiva, se è il nodo stesso crea la DHT Chord, se non è lui
	// allora significa che non è ancora healthy per il LB e aspettiamo ad entrare nella rete
	if len(result) == 1 {
		if result[0] == *addressPtr {
			utils.PrintTs("Creating Chord Ring")
			node.ChordClient = chord.Create(*addressPtr + utils.CHORD_PORT)
			first = true
		} else {
			goto waitLB
		}
	} else {
		// Se c'è più di un'istanza attiva viene contattato un altro nodo random per fare la Join
		utils.PrintTs("Joining Chord Ring")
		*joinPtr = result[rand.Intn(len(result))]
		for {
			if *joinPtr == *addressPtr {
				*joinPtr = result[rand.Intn(len(result))]
			} else {
				break
			}
		}
		node.ChordClient, _ = chord.Join(*addressPtr+utils.CHORD_PORT, *joinPtr+utils.CHORD_PORT)
		first = false
	}
	utils.PrintTs("My address is: " + *addressPtr)
	utils.PrintTs("Join address is: " + *joinPtr)
	utils.PrintTs("Chord Node Started Succesfully!")
}

/*
Inizializza il listener delle chiamate RPC per il funzionamento del sistema di storage distribuito.
Và invocata dopo aver inizializzato sia MongoDB che la DHT Chord in modo da poter gestire correttamente la comunicazione
tra i nodi del sistema.
*/
func InitRPCService(node *Node) {
	utils.PrintHeaderL2("Starting RPC Service")

	srv := &http.Server{
		Addr:    utils.RPC_PORT,
		Handler: http.DefaultServeMux,
	}

	rpc.Register(node)
	rpc.HandleHTTP()

	utils.PrintTs("Start Serving RPC request on port " + utils.RPC_PORT)
	utils.PrintTs("RPC Service Correctly Started")
	go srv.ListenAndServe()
}

/*
Inizializza i servizi per il listening dei messaggi di update e reconciliation
*/
func InitListeningServices(node *Node) {
	utils.PrintHeaderL2("Starting Listening Services")
	recvMutex = new(sync.Mutex)
	sendMutex = new(sync.Mutex)
	go ListenReplicationMessages(node)
	node.Handler = false
	node.Round = 0
	go ListenReconciliationMessages(node)
	go ListenMigrationMessages(node)
}

/*
Gestisce gli hearthbeat del Load Balancer ed i messaggi di Terminazione dal Service Registry
*/
func lb_handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "SDCC Distributed Key-Value Storage")
}

/*
Inizializza un listener sulla porta 8888, su cui il Nodo riceve gli HeartBeat del Load Balancer,
ed i segnali di terminazione dal service registry.
*/
func StartHeartBeatListener() {
	utils.PrintTs("Start Listening Heartbeats from LB on port: " + utils.HEARTBEAT_PORT)
	http.HandleFunc("/", lb_handler)
	http.ListenAndServe(utils.HEARTBEAT_PORT, nil)
}

/*
Restituisce l'indirizzo IP in uscita preferito della macchina che hosta il nodo
*/
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

/*
Permette al nodo di inserirsi nell'anello chord contattando il server specificato
*/
func JoinDHT(registryAddr string) []string {
	args := Args{}
	var reply []string

	client, _ := utils.HttpConnect(registryAddr, utils.REGISTRY_PORT)
	err := client.Call("DHThandler.JoinRing", args, &reply)
	if err != nil {
		log.Fatal("RPC error: ", err)
	}
	return reply
}

/*
Resta in ascolto per messaggi di aggiornamento del database. Utilizzato per ricevere i database dai nodi
schedulati per la terminazione
*/
func ListenReplicationMessages(node *Node) {
	fileChannel := make(chan string)

	go communication.StartReceiver(fileChannel, recvMutex, utils.REPLN)
	utils.PrintTs("Started Update Message listening Service")
	for {
		received := <-fileChannel
		if received == "rcvd" {
			node.MongoClient.MergeCollection(utils.REPLICATION_EXPORT_FILE, utils.REPLICATION_RECEIVE_FILE)
			utils.ClearDir(utils.REPLICATION_RECEIVE_PATH)
			recvMutex.Unlock()
		}
	}
}

/*
Resta in ascolto per la ricezione dei messaggi di riconciliazione. Ogni volta che si riceve un messaggio vengono
risolti i conflitti aggiornando il database
*/
func ListenReconciliationMessages(node *Node) {
	fileChannel := make(chan string)

	go communication.StartReceiver(fileChannel, recvMutex, utils.RECON)
	utils.PrintTs("Started Reconciliation Message listening Service")
	for {
		// Si scrive sul canale per attivare la riconciliazione una volta ricevuto correttamente l'update dal predecessore
		received := <-fileChannel
		if received == "rcvd" {
			node.MongoClient.ReconciliateCollection(utils.RECONCILIATION_EXPORT_FILE, utils.RECONCILIATION_RECEIVE_FILE)
			utils.ClearDir(utils.RECONCILIATION_RECEIVE_PATH)
			recvMutex.Unlock()

			// Nodo non ha successore, aspettiamo la ricostruzione della DHT Chord finchè non viene
			// completato l'aggiornamento dell'anello
		retry:
			if node.ChordClient.GetSuccessor().String() == "" {
				utils.PrintTs("Node hasn't a successor, wait for the reconstruction...")
				time.Sleep(utils.WAIT_SUCC_TIME)
				goto retry
			}

			// Il nodo effettua export del DB e lo invia al successore
			addr := node.ChordClient.GetSuccessor().GetIpAddr()
			utils.PrintTs("DB forwarded to successor: " + addr)

			// Solamente per il nodo che ha iniziato l'aggiornamento incrementiamo il contatore che ci permette
			// di interrompere dopo 2 giri non effettuando la SendCollectionMsg
			if node.Handler {
				node.Round++
				if node.Round == 2 {
					utils.PrintTs("Request returned to the node invoked by the registry two times, ring updated correctly")
					// Ripristiniamo le variabili per le future riconciliazioni
					node.Handler = false
					node.Round = 0
				} else {
					SendUpdateMsg(node, addr, utils.RECON, "")
				}
				// Se il nodo è uno di quelli intermedi, si limita a propagare l'aggiornamento
			} else {
				SendUpdateMsg(node, addr, utils.RECON, "")
			}
		}
	}
}

/*
Resta in ascolto per messaggi di aggiornamento del database. Utilizzato per ricevere i database dai nodi
schedulati per la terminazione
*/
func ListenMigrationMessages(node *Node) {
	fileChannel := make(chan string)

	go communication.StartReceiver(fileChannel, migrMutex, utils.MIGRN)
	utils.PrintTs("Started Migration listening Service")
	for {
		received := <-fileChannel
		if received == "rcvd" {
			node.MongoClient.MergeCollection(utils.MIGRATION_EXPORT_FILE, utils.MIGRATION_RECEIVE_FILE)
			utils.ClearDir(utils.MIGRATION_RECEIVE_PATH)
			recvMutex.Unlock()
		}
	}
}

/*
Esporta il file CSV e lo invia al nodo remoto. Con mode specifichiamo se il nodo remoto dovrà fare il merge delle
entry ricevute o solo la riconciliazione
*/
func SendUpdateMsg(node *Node, address string, mode string, key string) error {
	var file string
	var path string
	var err error

	sendMutex.Lock()

	switch mode {
	case utils.REPLN:
		utils.PrintHeaderL2("Sending replica to successor " + address + " via TCP")
		file = utils.REPLICATION_SEND_FILE
		path = utils.REPLICATION_SEND_PATH
		err = node.MongoClient.ExportDocument(key, file)
	case utils.RECON:
		utils.PrintHeaderL2("Sending reconciliation message to successor " + address + " via TCP")
		file = utils.RECONCILIATION_SEND_FILE
		path = utils.RECONCILIATION_SEND_PATH
		err = node.MongoClient.ExportCollection(file)
	case utils.MIGRN:
		utils.PrintHeaderL3("Sending migration entries to: " + address)
		file = utils.RECONCILIATION_SEND_FILE
		path = utils.RECONCILIATION_SEND_PATH
		err = node.MongoClient.ExportCollection(file)
	}

	if err != nil {
		utils.ClearDir(path)
		utils.PrintTs("File not exported. Message not sent.")
		sendMutex.Unlock()
		return err
	}

	err = communication.StartSender(file, address, mode)

	if err != nil {
		utils.ClearDir(path)
		utils.PrintTs("Message not sent.")
		sendMutex.Unlock()
		return err
	}

	utils.ClearDir(path)
	utils.PrintTs("Message sent correctly.")
	sendMutex.Unlock()
	return nil
}

func SendReplicaToSuccessor(node *Node, key string) {
retry:
	succ := node.ChordClient.GetSuccessor().GetIpAddr()
	if succ == "" {
		utils.PrintTs("Node hasn't a successor yet, data will be replicated later")
		time.Sleep(utils.WAIT_SUCC_TIME)
		goto retry
	}
	err := SendUpdateMsg(node, succ, utils.REPLN, key)
	if err != nil {
		return
	}
	utils.PrintTs("Replica sent Correctly")
}

/*
Permette di propagare la richiesta di Delete a tutte le repliche
*/
func DeleteReplicas(node *Node, args *Args, reply *string) {
	utils.PrintTs("Forwarding delete request")
retry:
	succ := node.ChordClient.GetSuccessor().GetIpAddr()
	if succ == "" {
		utils.PrintTs("Node hasn't a successor yet, replicas will be deleted later")
		time.Sleep(utils.WAIT_SUCC_TIME)
		goto retry
	}
	client, _ := utils.HttpConnect(succ, utils.RPC_PORT)
	utils.PrintTs("Delete request forwarded to replication node: " + succ + utils.RPC_PORT)
	client.Call("Node.DeleteReplicating", args, &reply)
}

func GetPredecessorEntries(node *Node) {
	var reply string
	args := Args{}
	args.Value = node.ChordClient.GetIpAddress()

	utils.PrintHeaderL2("Asking Predecessor for his entries")
	if first {
		utils.PrintTs("First node of the ring, no predecessor!")
		return
	}

retry:
	pred := node.ChordClient.GetPredecessor().GetIpAddr()
	if pred == "" {
		utils.PrintTs("Wait to get predecessor...")
		time.Sleep(5 * time.Second)
		goto retry
	}

	client, _ := utils.HttpConnect(pred, utils.RPC_PORT)
	err := client.Call("Node.JoinRPC", args, &reply)
	if err != nil {
		utils.PrintTs("JoinRPC error: " + err.Error())
		os.Exit(1)
	}
	utils.PrintTs(pred + ": " + reply)
}
