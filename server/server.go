package main

import(
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"sync"
)

// constants
const (
	// server details
	SERVER_HOST = "localhost"
	SERVER_PORT = "7777"
	SERVER_TYPE = "tcp"

	// other
	MAX_CLIENTS = 20
	MAX_PACKET_SIZE = 1024
	MAX_DATA_SIZE = 512
)

// server states
const (
	OK     	= iota	// 0
	ABORT			// 1
	EXIT			// 2
)

// client states
const (
	REGISTERING	= iota	// 0
	MESSAGING 			// 1
	QUITTING 			// 2
)

// packet types
const (
	ACCEPT	= iota	// 0
	DENY			// 1
	MESSAGE 		// 2
	REGISTRATION	// 3
	QUIT			// 4
)

// struct for holding client data
type client struct {
	Username	string
	Id 			int
	connection	net.Conn
	In_use		bool
	State		int
}

type packet struct {
	Type				int
	Sender_username 	string
	Sender_id			int
	Data				[]byte
}

// creating array of client structs
var (
	clients      	[MAX_CLIENTS]client
	clients_mutex 	sync.Mutex
)

var (
	active_clients 			int
	active_clients_mutex	sync.Mutex
)


func main(){

	// initialize global vars
	initialize_server()

	// creating socket and setting it to listen
	server, err := net.Listen(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		fmt.Println("server: Error listening:", err.Error())
		os.Exit(1)
	}
	// defering the closing of the server till the main function ends
	defer server.Close()

	// socket setup successfully
	fmt.Println("server: Listening on " + SERVER_HOST + ":" + SERVER_PORT)
	fmt.Println("server: Waiting for client...")

	for {
		if active_clients < 10 {
			// accepting client
			connection, err := server.Accept()
			if err != nil {
				fmt.Println("Error accepting: ", err.Error())
				os.Exit(1)
			}
			fmt.Println("server: client connected")

			// adding client to clients
			client_id, status :=add_client(connection)
			if status == ABORT {
				// TODO: create abort method to shut down server
				break
			}

			// serving client
			go serve_client(client_id, connection)
		}
	}
}

func initialize_server() {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	clients_mutex.Lock()
	defer clients_mutex.Unlock()

	active_clients = 0

	for _, current_client  := range clients {
		current_client.Username = ""
		current_client.Id = -1
		current_client.connection = nil
		current_client.In_use = false
		current_client.State = REGISTERING
	}
}

func add_client(connection net.Conn)(int, int) {
	increment_active_clients()

	clients_mutex.Lock()
	defer clients_mutex.Unlock()

	index := find_free_space()
	if index == -1 {
		fmt.Println("server: Failed to find free space for client")
		return -1, ABORT
	}

	clients[index].Id = index
	clients[index].connection = connection
	clients[index].In_use = true

	return index, OK
}

func increment_active_clients() {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	active_clients++
}

func find_free_space()(int){
	for index, current_client := range clients {
		if !current_client.In_use {
			return index
		}
	}

	return -1
}

func serve_client(client_id int, connection net.Conn)(int) {
	for {
		var packet packet
		json_packet := make([]byte, MAX_PACKET_SIZE)
	

		// reading packet from user
		amount_read, err := connection.Read(json_packet)
		if err != nil {
			fmt.Println("server: Error reading:", err.Error())
		}
		fmt.Printf("server: Recieved packet from client: %d\n", client_id)
		fmt.Printf("server: \"%s\"\n", string(json_packet))

		// unmarshaling json packet
		err = json.Unmarshal(json_packet[:amount_read], &packet)
		if err != nil {
			fmt.Println("server: Error unmarshaling JSON at handle_inbound:", err)
			os.Exit(1)
		}

		if(packet.Type == QUIT) {
			sub_client(client_id, connection)
			return	OK
		}

		// getting latest state if client
		clients_mutex.Lock()
		client_state := clients[client_id].State
		clients_mutex.Unlock()

		switch client_state {
			case REGISTERING:
				register_client(client_id, string(packet.Data), connection)
			case MESSAGING:
				clients_mutex.Lock()
				for i := 0; i < MAX_CLIENTS; i++ {
					if clients[i].In_use && clients[i].Id != client_id {
						_, err = clients[i].connection.Write(json_packet)
						if err != nil {
							fmt.Println("server: Error writing:", err)
						}
					}
				}
				clients_mutex.Unlock()
		}
	}
}

func register_client(client_id int, username string, connection net.Conn)(){
	fmt.Printf("server: validating the username \"%s\"\n", username)
	var packet packet

	// creating regex
	regex_pattern := "^[A-Za-z][A-Za-z0-9-_]{3,18}[A-Za-z0-9]$"
	regex, err := regexp.Compile(regex_pattern)
	if err != nil {
		fmt.Println("Error compiling regex:", err)
	}

	// varifying name
	if name_is_taken(username) {
		fmt.Println("server: Username already taken")
		packet.Type = DENY
		packet.Data = []byte("Username already taken")
	} else if !regex.MatchString(username) {
		fmt.Println("server: Username has invalid character or formatting")
		packet.Type = DENY
		packet.Data = []byte("Username has invalid character or formatting")
	} else {
		fmt.Println("server: Username is valid")
		packet.Type = ACCEPT
		msg := "You have been registered with the username \"" + username + "\""
		packet.Data = []byte(msg)

		clients_mutex.Lock()
		clients[client_id].Username = username
		clients[client_id].State = MESSAGING
		clients_mutex.Unlock()
	}

	// marshaling data
	json_packet, err := json.Marshal(packet)
	if err != nil {
		fmt.Println("server: Error marshaling data-", err.Error())
	}

	// sending packet
	_, err = connection.Write(json_packet)
	if err != nil {
		fmt.Println("server: Error writing-", err.Error())
	}
}

func name_is_taken(username string)(bool) {
	clients_mutex.Lock()
	defer clients_mutex.Unlock()

	for _, current_client := range clients {
		if current_client.Username == username {
			return true
		}
	}

	return false
}

func sub_client(client_id int, connection net.Conn) {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	active_clients--

	clients_mutex.Lock()
	defer clients_mutex.Unlock()

	clients[client_id].connection = nil
	clients[client_id].Username = ""
	clients[client_id].In_use = false
	clients[client_id].State = REGISTERING
	clients[client_id].Id = -1

	connection.Close()
}