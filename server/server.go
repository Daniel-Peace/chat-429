package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// constants
const (
	// server details
	SERVER_HOST = "localhost"
	// SERVER_HOST = "192.168.200.129"
	SERVER_PORT = "7777"
	SERVER_TYPE = "tcp"

	// other
	MAX_CLIENTS     = 20
	MAX_PACKET_SIZE = 1024
	MAX_DATA_SIZE   = 512
)

// server states
const (
	OK    = iota // 0
	ABORT        // 1
	EXIT         // 2
)

// commands
const (
	DNE = iota
	HELP
	MAIN
	LOG_OUT
	LIST_C
	LIST_S
	DISCONNECT_C
	DISCONNECT_S
	BAN_C
	BAN_S
	CREATE
	DELETE
	CHANGE_TOPIC
	ADD_MOD
	RM_MOD
)

// client states
const (
	REGISTERING          = iota // registing account
	MESSAGING                   // messaging group chat
	QUITTING                    // quitting application
	LOGGING_IN                  // logging in to existing account
	CHOOSING_SIGN_IN_OPT        // selecing to log in or register
	IN_HELP_SCREEN
)

// packet types
const (
	ACCEPT             = iota // 0
	DENY                      // 1
	MESSAGE                   // 2
	REGISTRATION              // 3
	QUIT                      // 4
	LOGIN                     // 5
	CHOOSE_SIGN_IN_OPT        // 6
	JOINED_MESSAGE            // 7
	LEFT_MESSAGE              // 8
	COMMAND
	CONNECT
)

// roles that a user can have
const (
	PLEB		=	 iota 	// 0
	MODERATOR				// 1
	ADMIN					// 2
)

// custom errors
const (
	OUT_OF_SYNC     = iota // 0
	UNEXPECTED_DATA        // 1
	UNKNOWN                // 2
)

// struct for holding account information of a client
type account_info struct {
	Username string
	Password string
}

// struct for holding client data
type client struct {
	Account_info account_info
	Id           int
	connection   net.Conn
	State        int
	Logged_in    bool
	Role       	 int
}

type packet struct {
	Type     int
	Username string
	Data     []byte
}

type Command struct{
	Type 	int
	args	[]string
}

// creating array of client structs
var (
	active_clients       []*client
	active_clients_mutex sync.Mutex
)

var (
	num_of_active_clients       int
	num_of_active_clients_mutex sync.Mutex
)

var (
	registered_accounts       []account_info
	registered_accounts_mutex sync.Mutex
)

// var stringToInt = map[string]int{
// 	"/help":  			0,
// 	"/main":   			2,
// 	"/log-out": 		3,
// 	"/list-c":			4,
// 	"/list-s": 			5,
// 	"/disconnect-c":	6,
// 	"/disconnect-s":	7,
// 	"/ban-c":			8,
// 	"/ban-s": 			9,
// 	"/create":			10,
// 	"/delete":			11,
// 	"/add-mod":			12,
// 	"/rm-mod":			13,
// }

/*
 * This is the main function of the server
 */
func main() {
	passive_socket := start_server()
	defer passive_socket.Close()

	for {
		handle_incoming_clients(passive_socket)
	}
}

/*
 * This function initializes everything in order to start the server
 */
func start_server()(net.Listener) {
	fmt.Println("system: Starting server...")

	// initializing the counter for the number of active clients
	init_num_of_active_clients()

	// initializing the array to hold active clients
	init_active_clients()

	// reading in saved accounts
	read_account_json()

	// creating passive socket
	command_socket := create_socket()

	// setting up signal catcher for ctrl-c
	setup_signal_handler(command_socket)

	// displaying server status
	fmt.Println("system: Server listening on " + SERVER_HOST + ":" + SERVER_PORT)
	fmt.Println("system: Successfully initialized srever")
	fmt.Println("system: Waiting for client...")

	return command_socket
}

/*
 * This function initiates num_of_active_clients
 */
func init_num_of_active_clients() {
	num_of_active_clients_mutex.Lock()
	defer num_of_active_clients_mutex.Unlock()
	num_of_active_clients = 0
}

/*
 * This function initializes the list of active clients
 */
func init_active_clients() {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()
	active_clients = make([]*client, MAX_CLIENTS)
	for index := range active_clients {
		active_clients[index] = &client{
			Account_info: account_info{},
			Id:           -1,
			connection:   nil,
			State:        CHOOSING_SIGN_IN_OPT,
			Logged_in:    false,
			Role: 		  0,	
		}
	}
}

func read_account_json() {
	// reading directory with account files
	json_files := read_accounts_directory()

	// chcking if there are files in the directory
	if json_files == nil {
		return
	}

	// looping through files in folder
	for _, current_file := range json_files {
		// creating file path
		file_path := "./users/" + current_file.Name()

		// opening file
		file := open_file(file_path)

		// creating new scanner
		scanner := bufio.NewScanner(file)

		// scanning current json file
		if scanner.Scan() {
			// getting json data from file
			json_data := scanner.Bytes()

			// unmarshaling json data
			var account_info account_info
			err := json.Unmarshal(json_data, &account_info)
			if err != nil {
				error_exit(err)
			}

			// closing file
			close_file(file)

			// adding current account to list of accounts
			registered_accounts = append(registered_accounts, account_info)
		}
	}
}

/*
 * This function reads the contents of a directory into an array and handles the possible errors
 */
func read_accounts_directory() []fs.DirEntry {
	files, err := os.ReadDir("./users")
	if err != nil {
		error_exit(err)
	}
	return files
}

/*
 * This function opens a file and handles the possible errors
 */
func open_file(path string) *os.File {
	file, err := os.Open(path)
	if err != nil {
		error_exit(err)
	}
	return file
}

/*
 * This function closes a file and handles the possible errors
 */
func close_file(file *os.File) {
	err := file.Close()
	if err != nil {
		error_exit(err)
	}
}

/*
 * This function creates a listening socket and handles the possible errors
 */
func create_socket() net.Listener {
	fmt.Println("system: Creating socket...")

	// creating socket
	listener, err := net.Listen(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)

	// handling erros
	if err != nil {
		error_exit(err)
	}

	fmt.Println("system: Successfully created socket")

	return listener
}

/*
 * This function creates a signal channel
 */
func setup_signal_handler(socket net.Listener) {
	// Create a channel to receive signals
	signal_channel := make(chan os.Signal, 1)

	// Notify the sigChan whenever a SIGINT signal is received
	signal.Notify(signal_channel, os.Interrupt, syscall.SIGINT)

	// starting routine to close server on ctrl-c
	go handleSigInt(signal_channel, socket)
}

/*
 * This function closes the server if ctrl-c is detected
 */
func handleSigInt(signale chan os.Signal, server net.Listener) {
	<-signale
	save_accounts()
	server.Close()
	os.Exit(0)
}

/*
 * This function updates the current account being saved
 */
func save_accounts() {
	// removing preexising files
	remove_existing_json_files()

	// writing account info to json files
	store_accounts_to_json()
}

/*
 * This function removes existing account files
 */
func remove_existing_json_files() {
	// opening user account directory
	files := read_accounts_directory()

	// chcking if there are files in the directory
	if files == nil {
		return
	}

	// looping through files in folder
	for _, file := range files {

		// creating file path
		file_path := "./users/" + file.Name()

		// removing file
		remove_file(file_path)
	}
}

/*
 * This function removes a file
 */
func remove_file(path string) {
	err := os.Remove(path)
	if err != nil {
		error_exit(err)
	}
}

/*
 * This function handles accepting incoming clients
 * and sends them to be served if the server has space.
 */
func handle_incoming_clients(passive_socket net.Listener) {
	command_socket := accept_client(passive_socket)
	serve_client_if_space(command_socket)
}

/*
 * This function accepts a client's connection
 * It exits if there is an error and returns the net.Conn upon success
 */
func accept_client(server net.Listener) net.Conn {
	command_socket, err := server.Accept()
	if err != nil {
		fmt.Println("Error accepting: ", err.Error())
		os.Exit(1)
	}
	fmt.Println("system: client connected")
	return command_socket
}

func store_accounts_to_json() {
	for _, account := range registered_accounts {

		// marshaling data
		json_data, err := json.Marshal(account)
		if err != nil {
			error_exit(err)
		}

		// creating path
		file_path := "./users/" + account.Username

		// creating file
		file := create_file(file_path)
		defer file.Close()

		write_to_file(file, json_data)
	}
}

/*
 * This function creates a file given a file path
 */
func create_file(path string) *os.File {
	file, err := os.Create(path)
	if err != nil {
		error_exit(err)
	}
	return file
}

/*
 * This function writes to a file
 */
func write_to_file(file *os.File, json_data []byte) {
	_, err := file.Write(json_data)
	if err != nil {
		error_exit(err)
	}
}

/*
 * This function checks if there is space on the server to serve the client.
 * If there is, it starts a routine to serve teh client and returns true.
 * If there is not it returns false
 * It exits if there is an error and returns the net.Conn upon success
 */
func serve_client_if_space(command_socket net.Conn) {
	// checking if the server is full
	num_of_active_clients_mutex.Lock()
	if num_of_active_clients >= 10 {
		fmt.Println("system: server is full, disconnecting client")
		packet := packet{Type: QUIT, Data: []byte("Server is full. Try again later")}
		send_packet(packet, command_socket)
		command_socket.Close()
		return
	}

	num_of_active_clients_mutex.Unlock()

	// incrementing the number of clients online
	increment_active_clients()

	// connect data socket
	json_data, amount_read:= read_from_connection(command_socket)

	fmt.Println(string(json_data))
	var packet packet
	json.Unmarshal(json_data[:amount_read], &packet)
	fmt.Printf("%d --------- %s\n", packet.Type, string(packet.Data))
	if packet.Type != CONNECT {
		custom_error_exit(OUT_OF_SYNC)
	}

	args := strings.Split(string(packet.Data), " ")

	data_socket,_ := net.Dial(args[0], args[1])

	message := make([]byte, MAX_PACKET_SIZE)
	amount_read, err := data_socket.Read(message)
	if err != nil {
		error_exit(err)
	}
	fmt.Println(string(message[:amount_read]))
	


	// finding a free space in the list of clients
	index := find_free_space()

	// initializing the space
	active_clients_mutex.Lock()
	active_clients[index].Id = index
	active_clients[index].connection = command_socket
	active_clients[index].State = CHOOSING_SIGN_IN_OPT
	active_clients_mutex.Unlock()

	// serving the client
	fmt.Println("system: serving client")
	go serve_client(index, command_socket)
}

/*
 * This function checks if the server is full and increments
 * the number of active clients if there is space.
 * It return true it successfully incremented
 */
func increment_active_clients() {
	num_of_active_clients_mutex.Lock()
	defer num_of_active_clients_mutex.Unlock()
	num_of_active_clients++
}

/*
 * This function looks for a free space in the list of active clients.
 * It returns the index on success and -1 on failure.
 */
func find_free_space() int {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()
	for index, current_client := range active_clients {
		fmt.Printf("The current value is: %d\n", current_client.Id)
		if current_client.Id < 0 {
			return index
		}
	}
	custom_error_exit(UNKNOWN)
	return -1
}

/*
 * This function provides the core loop for serving a client
 */
func serve_client(client_id int, connection net.Conn) {
	for {
		// getting latest state if client
		active_clients_mutex.Lock()
		client_state := active_clients[client_id].State
		active_clients_mutex.Unlock()

		switch client_state {
		case CHOOSING_SIGN_IN_OPT:
			choose_sign_in_opt(client_id, connection)
		case REGISTERING:
			register_client(client_id, connection)
		case LOGGING_IN:
			login(client_id, connection)
		case MESSAGING:
			message(client_id, connection)
		case QUITTING:
			sub_client(client_id, connection)
			return
		}
	}
}

/*
 * This function handles the sign in screen
 */
func choose_sign_in_opt(client_id int, connection net.Conn) {
	// reading packet from client
	packet := read_packet(client_id, connection)

	// checking if the packet has the expected type
	if packet.Type != CHOOSE_SIGN_IN_OPT && packet.Type != QUIT {
		custom_error_exit(OUT_OF_SYNC)
	}

	// quitting if user types /exit
	if packet.Type == QUIT {
		active_clients_mutex.Lock()
		active_clients[client_id].State = QUITTING
		active_clients_mutex.Unlock()
		return
	}

	// checking if user selected login or register
	if string(packet.Data) == "login" {
		active_clients_mutex.Lock()
		active_clients[client_id].State = LOGGING_IN
		active_clients_mutex.Unlock()
	} else if string(packet.Data) == "register" {
		active_clients_mutex.Lock()
		active_clients[client_id].State = REGISTERING
		active_clients_mutex.Unlock()
	} else {
		fmt.Println("system: Unexpected option found in \"choose_sign_in_opt\"")
		custom_error_exit(UNEXPECTED_DATA)
	}
}

/*
 * This function handles registering the client
 */
func register_client(client_id int, connection net.Conn) {
	// getting and validating username
	var username string
	for {
		fmt.Println("system: looking for password")
		// reading packet from client
		packet := read_packet(client_id, connection)

		// checking if the packet has the expected type
		if packet.Type != REGISTRATION && packet.Type != QUIT && packet.Type != COMMAND {
			custom_error_exit(OUT_OF_SYNC)
		}

		// quitting if user types /exit
		if packet.Type == QUIT {
			active_clients_mutex.Lock()
			active_clients[client_id].State = QUITTING
			active_clients_mutex.Unlock()
			return
		}

		// checking if user enterd a command
		if packet.Type == COMMAND {
			if execute_command(packet.Data, client_id) {
				return
			}
			continue
		}

		username = string(packet.Data)

		// validating username
		is_valid, packet := validate_username(string(packet.Data))

		// sending packet to client
		send_packet(packet, connection)

		// checking if username was valid
		if is_valid {
			active_clients_mutex.Lock()
			active_clients[client_id].Account_info.Username = username
			active_clients_mutex.Unlock()
			break
		}
	}

	// getting and validating password
	var password string
	for {
		fmt.Println("system: looking for password")
		// reading packet from client
		packet := read_packet(client_id, connection)

		// checking if the packet has the expected type
		if packet.Type != REGISTRATION && packet.Type != QUIT && packet.Type != COMMAND {
			custom_error_exit(OUT_OF_SYNC)
		}

		// quitting if user types /exit
		if packet.Type == QUIT {
			active_clients_mutex.Lock()
			active_clients[client_id].State = QUITTING
			active_clients_mutex.Unlock()
			return
		}

		// checking if user enterd a command
		if packet.Type == COMMAND {
			if execute_command(packet.Data, client_id) {
				return
			}
			continue
		}

		password = string(packet.Data)

		// validating username
		is_valid, packet := validate_password(string(packet.Data))

		send_packet(packet, connection)

		// checking if username was valid
		if is_valid {
			active_clients_mutex.Lock()
			defer active_clients_mutex.Unlock()

			registered_accounts_mutex.Lock()
			defer registered_accounts_mutex.Unlock()

			// updating info in active clients list
			active_clients[client_id].Account_info.Password = password
			active_clients[client_id].State = MESSAGING

			// adding account to list of accounts
			tempAccount := account_info{Username: username, Password: password}
			registered_accounts = append(registered_accounts, tempAccount)

			break
		}
	}
}

/*
 * This function validates usernames
 */
func validate_username(username string) (bool, packet) {
	// checking if a user already has this name
	if _, exists := name_is_exists(string(username)); exists {
		fmt.Println("server: Username already taken")

		// creating packet
		packet := packet{Type: DENY, Data: []byte("Username already taken")}

		return false, packet
	}

	// creating regex
	regex_username_pattern := "^[A-Za-z][A-Za-z0-9-_]{3,18}[A-Za-z0-9]$"
	regex_username, err := regexp.Compile(regex_username_pattern)
	if err != nil {
		fmt.Println("Error compiling regex:", err)
	}

	// checking if username matches the regular expression
	if !regex_username.MatchString(username) {
		fmt.Println("server: Username has invalid character or formatting")

		// creating packet
		packet := packet{Type: DENY, Data: []byte("Username has invalid character or formatting")}

		return false, packet
	}

	// username is valid
	fmt.Println("server: Username is valid")

	// creating response for packet
	msg := "You have been registered with the username \"" + username + "\""

	// creating packet
	packet := packet{Type: ACCEPT, Data: []byte(msg)}

	return true, packet
}

/*
 * This function validates passwords
 */
func validate_password(password string) (bool, packet) {
	// Check for at least one uppercase letter
	has_uppercase := regexp.MustCompile("[A-Z]").MatchString(password)

	// Check for at least one digit
	has_digit := regexp.MustCompile("[0-9]").MatchString(password)

	// Check for at least one special character
	has_special_char := regexp.MustCompile("[!@#$%?]").MatchString(password)

	// Check for minimum length of 7 characters
	is_minimum_length := len(password) >= 7

	// checking if password matches regular expression
	if has_uppercase && has_digit && has_special_char && is_minimum_length {
		fmt.Println("system: password is valid")
		packet := packet{Type: ACCEPT, Data: []byte("account successfully created")}
		return true, packet
	} else {
		fmt.Println("server: Password has invalid character or formatting")
		packet := packet{Type: DENY, Data: []byte("Password has invalid character or formatting")}
		return false, packet
	}
}

/*
 * This function handles logging in a client
 */
func login(client_id int, connection net.Conn) {
	var index int
	var exists bool

	// checking for account with name
	for {
		fmt.Println("system: looking for username")
		packet := read_packet(client_id, connection)

		// checking if the packet has the expected type
		if packet.Type != LOGIN && packet.Type != QUIT && packet.Type != COMMAND {
			custom_error_exit(OUT_OF_SYNC)
		}

		// quitting if user types /exit
		if packet.Type == QUIT {
			active_clients_mutex.Lock()
			active_clients[client_id].State = QUITTING
			active_clients_mutex.Unlock()
			return
		}

		// checking if user enterd a command
		if packet.Type == COMMAND {
			if execute_command(packet.Data, client_id) {
				return
			}
			continue
		}

		// checking if an account exists with the given username
		index, exists = name_is_exists(string(packet.Data))

		// checking if an account exists with the given username
		if !exists {
			packet.Type = DENY
			packet.Data = []byte("No account found was found with that username")
			send_packet(packet, connection)
			continue
		}

		// checking if the account is already logged in
		if exists && is_logged_in(index) {
			packet.Type = DENY
			packet.Data = []byte("This account is already logged in somewhere")
			send_packet(packet, connection)
			continue
		}

		active_clients_mutex.Lock()
		active_clients[client_id].Account_info.Username = string(packet.Data)
		active_clients_mutex.Unlock()
		fmt.Println("system: Found account for the given name")
		packet.Type = ACCEPT
		packet.Data = []byte("found account with that username")
		send_packet(packet, connection)
		break
	}

	for {
		fmt.Println("system: looking for password")
		packet := read_packet(client_id, connection)

		// checking if the packet has the expected type
		if packet.Type != LOGIN && packet.Type != QUIT && packet.Type != COMMAND{
			custom_error_exit(OUT_OF_SYNC)
		}

		// quitting if user types /exit
		if packet.Type == QUIT {
			active_clients_mutex.Lock()
			active_clients[client_id].State = QUITTING
			active_clients_mutex.Unlock()
			return
		}

		// checking if user enterd a command
		if packet.Type == COMMAND {
			if execute_command(packet.Data, client_id) {
				return
			}
			continue
		}

		// checking if password matches account password
		if string(packet.Data) == registered_accounts[index].Password {
			packet.Type = ACCEPT
			packet.Data = []byte("successfully logged in")
			send_packet(packet, connection)
			active_clients_mutex.Lock()
			active_clients[client_id].State = MESSAGING
			active_clients[client_id].Account_info.Password = string(packet.Data)
			active_clients[client_id].Logged_in = true
			active_clients_mutex.Unlock()
			return
		} else {
			packet.Type = DENY
			packet.Data = []byte("incorrect password")
			send_packet(packet, connection)
		}
	}
}

/*
 * This function handles messaging
 */
func message(client_id int, connection net.Conn) {
	for {
		// reading packet from client
		packet := read_packet(client_id, connection)

		// checking if the packet has the expected type
		if packet.Type != MESSAGE && packet.Type != QUIT && packet.Type != JOINED_MESSAGE && packet.Type != COMMAND{
			custom_error_exit(OUT_OF_SYNC)
		}

		// quitting if user types /exit
		if packet.Type == QUIT {
			active_clients_mutex.Lock()
			active_clients[client_id].State = QUITTING
			active_clients_mutex.Unlock()
			return
		}

		// checking if user enterd a command
		if packet.Type == COMMAND {
			if execute_command(packet.Data, client_id) {
				return
			}
			continue
		}

		// checking if user enterd a command
		if packet.Type == COMMAND {
			if execute_command(packet.Data, client_id) {
				return
			}
		}

		// sending message to everyone in the default chat
		active_clients_mutex.Lock()
		for i := 0; i < MAX_CLIENTS; i++ {
			if active_clients[i].Id > -1 && active_clients[i].Id != client_id && active_clients[i].State == MESSAGING {
				send_packet(packet, active_clients[i].connection)
			}
		}
		active_clients_mutex.Unlock()
	}
}

/*
 * This function checks to see if a username is already taken
 */
func name_is_exists(username string) (int, bool) {
	registered_accounts_mutex.Lock()
	defer registered_accounts_mutex.Unlock()

	// looping over exising accounts
	for index, current_account := range registered_accounts {
		// checking if username exists
		if current_account.Username == username {
			fmt.Println("Found name!")
			return index, true
		}
	}

	return -1, false
}

/*
 * This function sends a packet to a specified client
 */
func send_packet(packet packet, connection net.Conn) {
	// marshaling data
	json_data := marshal_packet(packet)

	// sending packet
	write_to_connection(connection, json_data)
}

/*
 * This function writes to a specified net.Conn
 */
func write_to_connection(connection net.Conn, data []byte) {
	_, err := connection.Write(data)
	if err != nil {
		error_exit(err)
	}
}

/*
 * This function reads a packet from a client
 */
func read_packet(client_id int, connection net.Conn) packet {
	json_data, amount_read := read_from_connection(connection)
	fmt.Printf("server: Recieved packet from client: %d\n", client_id)
	fmt.Printf("server: \"%s\"\n", string(json_data))

	var packet packet

	// checking if the packet is empty
	if amount_read > 0 {
		// unmarshaling json packet
		packet = unmarshal_packet(json_data[:amount_read])
	}

	return packet
}

/*
 * This function reads from a specified net.Conn
 */
func read_from_connection(connection net.Conn) ([]byte, int) {
	// reading packet from user
	data := make([]byte, MAX_PACKET_SIZE)
	amount_read, err := connection.Read(data)
	if err != nil {
		error_exit(err)
	}
	return data, amount_read
}

/*
 * This function marshals a packet into a json file
 */
func marshal_packet(packet packet) []byte {
	// marshaling data
	json_data, err := json.Marshal(packet)
	if err != nil {
		fmt.Println("server: Error marshaling data-", err.Error())
	}
	return json_data
}

/*
 * This function unmarshals json data into a packetand handles the possible errors
 */
func unmarshal_packet(json_data []byte) packet {
	// unmarshaling json packet
	//json_data = trim_null_characters(json_data)
	var packet packet
	err := json.Unmarshal(json_data, &packet)
	if err != nil {
		error_exit(err)
	}
	return packet
}

/*
 * This function handles closing the server is a predefined error occures
 */
func error_exit(err error) {
	fmt.Println("system: ERROR -", err)
	fmt.Println("system: shutting down server...")
}

/*
 * This function handles exiting the server if a custom error occurs
 */
func custom_error_exit(err int) {
	switch err {
	case OUT_OF_SYNC:
		fmt.Println("system: ERROR - Server and client out of sync")
	case UNEXPECTED_DATA:
		fmt.Println("system: ERROR - Unexpected data found in packet")
	case UNKNOWN:
		fmt.Println("system: ERROR - An unknown error occured")
	}

	fmt.Println("system: Shutting down...")
	os.Exit(1)
}

/*
 * This function handles disconnecting a client
 */
func sub_client(client_id int, connection net.Conn) {
	fmt.Println("system: Disconnecting client...")

	decrement_num_of_active_clients()

	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	active_clients[client_id].connection = nil
	active_clients[client_id].Account_info.Username = ""
	active_clients[client_id].Account_info.Password = ""
	active_clients[client_id].State = CHOOSING_SIGN_IN_OPT
	active_clients[client_id].Id = -1
	active_clients[client_id].Logged_in = false
	active_clients[client_id].Role = 0

	connection.Close()
}

/*
 * This function decrements the number of active clients
 */
func decrement_num_of_active_clients() {
	num_of_active_clients_mutex.Lock()
	defer num_of_active_clients_mutex.Unlock()
	num_of_active_clients--
}

/*
 * this function checks if an account is logged in
 * It returns true if an account is indeed logged in
 */
func is_logged_in(index int) bool {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	return active_clients[index].Logged_in
}

/*
 * Executes a command
 */
func execute_command(unparsed_command []byte, client_id int)(bool) {
	// parsing the command
	command := parse_command(unparsed_command)
	fmt.Printf("system: Recieved command of type \"%d\" with %d arguments\n", command.Type, len(command.args))

	active_clients_mutex.Lock()
	previous_state := active_clients[client_id].State
	active_clients_mutex.Unlock()

	fmt.Println("system: Saved current state")
	switch command.Type {
		case HELP:
			fmt.Println("system: Running help command")
			help_command(client_id)
			return false
		case MAIN:
		case LOG_OUT:
		case LIST_C:
		case LIST_S:
		case DISCONNECT_C:
		case DISCONNECT_S:
		case BAN_C:
		case BAN_S:
		case CREATE:
		case DELETE:
		case CHANGE_TOPIC:
		case ADD_MOD:
		case RM_MOD:
		default:
			custom_error_exit(UNKNOWN)
	}

	// checking if the state of the client was changed
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()
	return previous_state == active_clients[client_id].State
}

func parse_command(unparsed_command []byte)(Command) {
	// parsing command into an array of tokens
	tokens := strings.Split(string(unparsed_command), " ")

	// getting command type
	num, err := strconv.Atoi(tokens[0])
	if err != nil {
		error_exit(err)
	}

	// creating command struct
	var command Command
	command.Type = num

	// checking if the command had arguments
	if len(tokens) > 1 {
		// adding the arguments to the struct
		for i := 1; i < len(tokens); i++ {
			command.args = append(command.args, tokens[i])
		}
	} else {
		// setting args to nil
		command.args = nil
	}

	return command
}

func help_command(client_id int) {
	// saving current state and then setting state to IN_HELP_SCREEN
	active_clients_mutex.Lock()
	previous_state := active_clients[client_id].State
	active_clients[client_id].State = IN_HELP_SCREEN
	client := active_clients[client_id]
	active_clients_mutex.Unlock()

	// sending packet with user's role
	packet := packet{Type: COMMAND, Data: []byte(strconv.Itoa(client.Role))}
	send_packet(packet, client.connection)

	// reading packet from client
	packet = read_packet(client_id, client.connection)

	// varifying packet type
	if packet.Type != COMMAND {
		custom_error_exit(OUT_OF_SYNC)
	}

	// checking if data is expected keyword
	if string(packet.Data) != "DONE" {
		custom_error_exit(UNEXPECTED_DATA)
	}

	// restoring state prior to command
	active_clients_mutex.Lock()
	active_clients[client_id].State = previous_state
	active_clients_mutex.Unlock()
}