package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------------------------------

// constants
const (
	// server details
	SERVER_HOST = "localhost"
	SERVER_PORT = "7777"
	SERVER_TYPE = "tcp"

	// other
	MAX_CLIENTS     = 20
	MAX_PACKET_SIZE = 1024
	MAX_CHANNELS    = 5
)

// ansi text styles
const (
	RED         = "\x1b[31m"
	GREEN       = "\x1b[32m"
	YELLOW      = "\x1b[33m"
	BLUE        = "\x1b[34m"
	MAGENTA     = "\x1b[35m"
	CYAN        = "\x1b[36m"
	WHTIE       = "\x1b[37m"
	RESET       = "\x1b[0m"
	BOLD        = "\x1b[1m"
	FAINT       = "\x1b[2m"
	ITALIC      = "\x1b[3m"
	UNDERLINE   = "\x1b[4m"
	INVERSE     = "\x1b[7m"
	CROSSED_OUT = "\x1b[9m"
)

// commands types
const (
	// public commands
	DNE     = iota // command does not exist
	HELP           // brings up help menu
	EXIT           // disconnects the client from a server
	MAIN           // takes you to the main menu
	LOG_OUT        // logs a user out and brings them to the sign in menu
	LIST_C         // lists all users in a channel
	LIST_S         // lists all users on the server

	// moderator commands
	DISCONNECT_C // disconnects a user from a channel
	DISCONNECT_S // disconnects a user from the server
	BAN_C        // bans a user from a channel
	BAN_S        // bans a user from the server
	CREATE       // creates a channel
	DELETE       // deletes a channel
	CHANGE_TOPIC // changes topic of a channel

	// admin commands
	ADD_MOD // gives user the moderator role
	RM_MOD  // removes the moderator role from a user

	// system
	CONNECT // used to establish data socket connection
)

// client states
const (
	CHOOSING_SIGN_IN_OPT = iota // selecting to log in register or exit
	REGISTERING                 // registing account
	LOGGING_IN                  // logging in to existing account
	MESSAGING                   // messaging group chat
	QUITTING                    // quitting application
	IN_HELP_SCREEN              // using the help command
	IN_MAIN_MENU                // in main menu
)

// packet types
const (
	ACCEPT       = iota // 0	Used to indicate a name or password was accepted
	DENY                // 1	Used to indicate a name or password was denied
	MESSAGE             // 2	Used to send a standard message to a channel
	JOIN_MSG            // 3 Used to send a joining message to a chat
	LEAVE_MSG           // 4 Used to send a leaving message to a chat
	REGISTRATION        // 5	Used to send a username or password for registering a user
	LOGIN               // 6	Used to send a username or password for loggin in
	MENU_OPTION         // 7	Used to send menu options
	CLOSE               // 8 Used to close a function if the state changes of the client
	ESC                 // 9 used when a user uses escape to go back
)

// user roles
const (
	PUBLIC    = iota // 0	default role of every user
	MODERATOR        // 1	a role that can be assigned by the admin to give additional capabilities
	ADMIN            // 2	only one admin exists.....
)

// custom errors
const (
	OUT_OF_SYNC     = iota // 0		packet type does not match expeceted type
	UNEXPECTED_DATA        // 1		data in packet does not match expected
	UNKNOWN                // 2		unknown error occured
)

// ---------------------------------------------------------------------------------------------------

// struct for holding account information of a client
type Account_info struct {
	Username string
	Password string
	Role     int
	Banned   bool
}

// struct for holding client data
type Client struct {
	Account_info    Account_info
	Id              int
	data_sock       net.Conn
	command_sock    net.Conn
	State           int
	Logged_in       bool
	Current_channel int
}

// struc for holding a data packet
type Data_packet struct {
	Type     int
	Username string
	Data     []byte
}

// struct for holding a command packet
type Command_packet struct {
	Type       int
	Username   string
	Arguments  []byte
	Successful bool
	Message    []byte
}

// struct for holding a parsed command
type Parsed_command struct {
	Type       int
	Username   string
	Args       []string
	Successful bool
	Message    []byte
}

type Channel struct {
	Id    int
	Topic []byte
	Users []int
}

// ---------------------------------------------------------------------------------------------------

// global variables
var (
	// creating array of client structs
	active_clients       []*Client
	active_clients_mutex sync.Mutex

	// creating array of chennels structs
	channels       []*Channel
	channels_mutex sync.Mutex

	// counter for tracking the number of clients connected to the server
	num_of_active_clients       int
	num_of_active_clients_mutex sync.Mutex

	// array that holds registered accounts
	registered_accounts       []Account_info
	registered_accounts_mutex sync.Mutex

	// passive socket for accepting clients
	accept_socket net.Listener
)

// ---------------------------------------------------------------------------------------------------

/*
 * This is the main function of the server
 */
func main() {
	// initializing and starting the server
	start_server()

	// closing passive socket once main exits
	defer accept_socket.Close()

	// handling connecting clients
	for {
		handle_incoming_clients(accept_socket)
	}
}

/*
 * This function initializes everything in order to start the server
 */
func start_server() {
	// clearing terminal
	clear_terminal()

	fmt.Println("-------------------------------------------------------------------------")
	fmt.Println("system: Starting server...")

	// initializing the counter for the number of active clients
	init_num_of_active_clients()

	// initializing channels list
	init_channels()

	// initializing the array to hold active clients
	init_active_clients()

	// reading in saved accounts
	load_accounts()

	// creating passive socket
	create_socket()

	// setting up signal catcher for ctrl-c
	setup_signal_handler()

	// displaying server status
	fmt.Println("-------------------------------------------------------------------------")
	fmt.Println("system: Server listening on " + SERVER_HOST + ":" + SERVER_PORT)
	fmt.Println("system: Successfully initialized srever")
	fmt.Println("system: Waiting for client...")
	fmt.Println("-------------------------------------------------------------------------")
}

/*
 * This function initiates num_of_active_clients
 */
func init_num_of_active_clients() {
	num_of_active_clients_mutex.Lock()
	defer num_of_active_clients_mutex.Unlock()

	num_of_active_clients = 0

	msg := GREEN + " - initialized counter for clients\n" + RESET
	fmt.Print(msg)
}

/*
 * This function initializes the list of channels
 */
func init_channels() {
	channels_mutex.Lock()
	defer channels_mutex.Unlock()

	// allocating space for channel array
	channels = make([]*Channel, MAX_CHANNELS)

	// adding default channel to array and setting the rest of the slot to available
	for index := 0; index < MAX_CHANNELS; index++ {
		if index == 0 {
			channels[index] = &Channel{Id: index, Topic: []byte("nonsense"), Users: nil}
		} else {
			channels[index] = &Channel{Id: -1, Topic: []byte(""), Users: nil}
		}
	}
}

/*
 * This function initializes the list of active clients
 */
func init_active_clients() {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	// allocating space for array of clients
	active_clients = make([]*Client, MAX_CLIENTS)

	// initializing each client in the array
	for index := range active_clients {
		active_clients[index] = &Client{
			Account_info:    Account_info{Username: "", Password: "", Role: 0, Banned: false},
			Id:              -1,
			data_sock:       nil,
			command_sock:    nil,
			State:           CHOOSING_SIGN_IN_OPT,
			Logged_in:       false,
			Current_channel: -1,
		}
	}

	msg := GREEN + " - initialized array of clients\n" + RESET
	fmt.Print(msg)
}

/*
 * This function loads all of the accounts stored in the json file into an array
 */
func load_accounts() {
	// reading directory with account files
	json_files := read_accounts_directory()

	// chcking if there are files in the directory
	if json_files == nil {
		return
	}

	// adding the admin account to the registered accounts array
	admin_account := Account_info{Username: "Admin", Password: "gochat", Role: ADMIN, Banned: false}

	registered_accounts_mutex.Lock()
	defer registered_accounts_mutex.Unlock()

	registered_accounts = append(registered_accounts, admin_account)

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
			var account_info Account_info
			err := json.Unmarshal(json_data, &account_info)
			if err != nil {
				error_exit(err)
			}

			// closing file
			close_file(file)

			// adding current account to list of accounts
			registered_accounts = append(registered_accounts, account_info)
		} else {
			custom_error_exit(UNKNOWN)
		}
	}

	msg := GREEN + " - loaded accounts\n" + RESET
	fmt.Print(msg)
}

/*
 * This function creates an array of file names for the files in a given directory
 */
func read_accounts_directory() []fs.DirEntry {
	files, err := os.ReadDir("./users")
	if err != nil {
		error_exit(err)
	}
	return files
}

/*
 * This function opens a file
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
 * This function creates a listening socket
 */
func create_socket() {
	listener, err := net.Listen(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		error_exit(err)
	}

	accept_socket = listener

	msg := GREEN + " - created passive socket\n" + RESET
	fmt.Print(msg)
}

/*
 * This function creates a signal channel
 */
func setup_signal_handler() {
	// Create a channel to receive signals
	signal_channel := make(chan os.Signal, 1)

	// Notify the sigChan whenever a SIGINT signal is received
	signal.Notify(signal_channel, os.Interrupt, syscall.SIGINT)

	// starting routine to close server on siganl
	go handle_ctrl_c(signal_channel)

	msg := GREEN + " - set up signal catcher\n" + RESET
	fmt.Print(msg)
}

/*
 * This function closes the server if ctrl-c is detected
 */
func handle_ctrl_c(signale chan os.Signal) {
	<-signale
	save_accounts()
	accept_socket.Close()

	//TODO

	os.Exit(0)
}

/*
 * This function saves all current accounts to the json files and stores them in the users directory
 */
func save_accounts() {
	// removing preexising files
	remove_existing_json_files()

	// writing account info to json files
	write_accounts_to_json()
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
		error_exit(err)
	}

	fmt.Println("system: client connected")

	return command_socket
}

/*
 * This function writes all of the accounts currently in the accounts array to a json files
 */
func write_accounts_to_json() {
	// looping over registered_accounts
	for index, account := range registered_accounts {
		// skipping first account since this is the admin account
		if index == 0 {
			continue
		}

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

		// writing data to file
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
 * It exits if there is an error
 */
func serve_client_if_space(command_socket net.Conn) {
	// checking if the server is full
	num_of_active_clients_mutex.Lock()
	if num_of_active_clients >= MAX_CLIENTS {
		// server was full

		// disconnecting client
		fmt.Println("system: server is full, disconnecting client")

		// creating the data packet
		packet := Data_packet{Type: DENY, Data: []byte("Server is full. Try again later")}

		// marshaling the data
		json_data := marshal_packet(packet)

		// sending the packet to the client
		write_to_connection(command_socket, json_data)

		// closing the socket
		command_socket.Close()

		return
	}
	num_of_active_clients_mutex.Unlock()

	// incrementing the number of clients online
	increment_active_clients()

	// establishing a connection for data
	data_socket := establish_data_socket(command_socket)

	// finding a free space in the list of clients
	index := find_free_space_for_client()

	// initializing the space
	active_clients_mutex.Lock()
	active_clients[index].Id = index
	active_clients[index].command_sock = command_socket
	active_clients[index].data_sock = data_socket
	active_clients[index].State = CHOOSING_SIGN_IN_OPT
	client := active_clients[index]
	active_clients_mutex.Unlock()

	

	// starting go routine to serve client
	go serve_client(*client)
	fmt.Println("system: serving client")
}

/*
 * This function creates the data socket
 */
func establish_data_socket(command_socket net.Conn) net.Conn {
	// address of data socket from client
	json_data, amount_read := read_from_connection(command_socket)

	// unmarhsal data into packet
	packet := unmarshal_command_packet(json_data[:amount_read])

	// attempt to connect to data_socket
	data_socket, err := net.Dial(SERVER_TYPE, string(packet.Arguments))
	if err != nil {
		error_exit(err)
	}

	fmt.Println("system: Created data socket")

	return data_socket
}

/*
 * This function increments num_of_active_clients
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
func find_free_space_for_client() int {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	// looping over client array to find a slot
	for index, current_client := range active_clients {
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
func serve_client(client Client) {
	// starting a routine to handle inbound commands
	go handle_inbound_commands(client)

	// core loop to handle a clients state
	for {
		// getting latest state if client
		client = update_client(client)

		// switching on the state of the client
		switch client.State {
		case CHOOSING_SIGN_IN_OPT:
			choose_sign_in_opt(client)
		case REGISTERING:
			register_client(client)
		case LOGGING_IN:
			login(client)
		case MESSAGING:
			message(client)
		case IN_MAIN_MENU:
			main_menu(client)
		case QUITTING:
			fmt.Println("system: Closing client's main routine")
			return
		}
	}
}

/*
 * This function handles the sign in screen
 */
func choose_sign_in_opt(client Client) {
	// reading packet from client
	packet := read_data_packet(client)
	fmt.Printf("system: Received menu option \"%s\" from client #%d\n", string(packet.Data), client.Id)

	// checking if the client has changed state and this function needs to return
	if packet.Type == CLOSE {
		return
	}

	// checking if the packet has the expected type
	if packet.Type != MENU_OPTION {
		custom_error_exit(OUT_OF_SYNC)
	}

	// checking if user selected login or register
	if string(packet.Data) == "LOGIN" {
		update_client_state(client, LOGGING_IN)
		client.State = LOGGING_IN
	} else if string(packet.Data) == "REGISTER" {
		update_client_state(client, REGISTERING)
		client.State = REGISTERING
	} else {
		fmt.Println("system: Unexpected menu option in \"choose_sign_in_opt\"")
		custom_error_exit(UNEXPECTED_DATA)
	}
}

/*
 * This function handles registering the client
 */
func register_client(client Client) {
	// getting and validating username
	var username string

	// looping until a valid username is entered, the user hits esc, or the user quits
	for {
		// reading packet from client
		packet := read_data_packet(client)
		fmt.Printf("system: Received data packet from client #%d\n", client.Id)
		print_data_packet(packet)

		// checking if the client has changed state and this function needs to return
		if packet.Type == CLOSE {
			return
		}

		// checking if user pressed the escape key
		if packet.Type == ESC {
			update_client_state(client, CHOOSING_SIGN_IN_OPT)
			return
		}

		// checking if the packet has the expected type
		if packet.Type != REGISTRATION {
			custom_error_exit(OUT_OF_SYNC)
		}

		// saving username temporarely
		username = string(packet.Data)

		// validating username
		is_valid, packet := validate_username(string(packet.Data))

		// sending packet to client
		send_data_packet(packet, client)

		// checking if username was valid
		if is_valid {
			active_clients_mutex.Lock()
			active_clients[client.Id].Account_info.Username = username
			active_clients_mutex.Unlock()
			client.Account_info.Username = username
			break
		}
	}

	// getting and validating password
	var password string

	// looping until a valid password is entered, the user hits esc, or the user quits
	for {
		// reading packet from client
		packet := read_data_packet(client)
		fmt.Printf("system: Received data packet from client #%d\n", client.Id)
		print_data_packet(packet)

		// checking if the client has changed state and this function needs to return
		if packet.Type == CLOSE {
			return
		}

		// checking if user pressed the escape key
		if packet.Type == ESC {
			update_client_state(client, CHOOSING_SIGN_IN_OPT)
			active_clients_mutex.Lock()
			active_clients[client.Id].Account_info.Username = ""
			active_clients_mutex.Unlock()
			client.Account_info.Username = ""
			return
		}

		// checking if the packet has the expected type
		if packet.Type != REGISTRATION {
			custom_error_exit(OUT_OF_SYNC)
		}

		password = string(packet.Data)

		// validating username
		is_valid, packet := validate_password(string(packet.Data))

		send_data_packet(packet, client)

		// checking if username was valid
		if is_valid {
			active_clients_mutex.Lock()
			registered_accounts_mutex.Lock()

			// updating info in active clients list
			active_clients[client.Id].Account_info.Password = password
			active_clients[client.Id].State = IN_MAIN_MENU

			// adding account to list of accounts
			tempAccount := Account_info{Username: username, Password: password}
			registered_accounts = append(registered_accounts, tempAccount)

			registered_accounts_mutex.Unlock()
			active_clients_mutex.Unlock()

			save_accounts()

			break
		}
	}
}

/*
 * This function validates usernames
 */
func validate_username(username string) (bool, Data_packet) {
	// checking if a user already has this name
	if _, exists := name_is_exists(string(username)); exists {
		fmt.Println("server: Username already taken")

		// creating packet
		packet := Data_packet{Type: DENY, Data: []byte("Username already taken")}

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
		packet := Data_packet{Type: DENY, Data: []byte("Username has invalid character or formatting")}

		return false, packet
	}

	// username is valid
	fmt.Println("server: Username is valid")

	// creating response for packet
	msg := "You have been registered with the username \"" + username + "\""

	// creating packet
	packet := Data_packet{Type: ACCEPT, Data: []byte(msg)}

	return true, packet
}

/*
 * This function validates passwords
 */
func validate_password(password string) (bool, Data_packet) {
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
		packet := Data_packet{Type: ACCEPT, Data: []byte("account successfully created")}
		return true, packet
	} else {
		fmt.Println("server: Password has invalid character or formatting")
		packet := Data_packet{Type: DENY, Data: []byte("Password has invalid character or formatting")}
		return false, packet
	}
}

/*
 * This function handles logging in a client
 */
func login(client Client) {
	var index int
	var exists bool

	// looping until a valid username is entered, the user hits esc, or the user quits
	for {
		// read packet from
		packet := read_data_packet(client)
		fmt.Printf("system: Received data packet from client #%d\n", client.Id)
		print_data_packet(packet)

		// checking if the client has changed state and this function needs to return
		if packet.Type == CLOSE {
			return
		}

		// checking if user pressed the escape key
		if packet.Type == ESC {
			update_client_state(client, CHOOSING_SIGN_IN_OPT)
			return
		}

		// checking if the packet has the expected type
		if packet.Type != LOGIN {
			custom_error_exit(OUT_OF_SYNC)
		}

		// checking if an account exists with the given username
		index, exists = name_is_exists(string(packet.Data))

		// checking if an account exists with the given username
		if !exists {
			packet.Type = DENY
			packet.Data = []byte("No account found was found with that username")
			send_data_packet(packet, client)
			continue
		}

		// checking if the account is already logged in
		if exists && is_logged_in(string(packet.Data)) {
			packet.Type = DENY
			packet.Data = []byte("This account is already logged in somewhere")
			send_data_packet(packet, client)
			continue
		} else if exists && is_banned(index) {
			packet.Type = DENY
			packet.Data = []byte("This account is banned from the server")
			send_data_packet(packet, client)
			continue
		}

		// adding client's username to active clients
		active_clients_mutex.Lock()
		active_clients[client.Id].Account_info.Username = string(packet.Data)
		active_clients_mutex.Unlock()

		fmt.Printf("system: Found account for the name given by client #%s\n", string(client.Id))

		// creating and sending accept packet
		packet.Type = ACCEPT
		packet.Data = []byte("Found account with that username")
		send_data_packet(packet, client)
		break
	}

	// looping until a valid password is entered, the user hits esc, or the user quits
	for {
		packet := read_data_packet(client)
		fmt.Printf("system: Received data packet from client #%d\n", client.Id)
		print_data_packet(packet)

		// checking if the client has changed state and this function needs to return
		if packet.Type == CLOSE {
			return
		}

		// checking if user pressed the escape key
		if packet.Type == ESC {
			update_client_state(client, CHOOSING_SIGN_IN_OPT)
			active_clients_mutex.Lock()
			active_clients[client.Id].Account_info.Username = ""
			active_clients_mutex.Unlock()
			return
		}

		// checking if the packet has the expected type
		if packet.Type != LOGIN {
			custom_error_exit(OUT_OF_SYNC)
		}

		// checking if password matches account password
		registered_accounts_mutex.Lock()
		if string(packet.Data) == registered_accounts[index].Password {
			packet.Type = ACCEPT
			packet.Data = []byte("Success!")
			send_data_packet(packet, client)
			active_clients_mutex.Lock()
			active_clients[client.Id].State = IN_MAIN_MENU
			active_clients[client.Id].Account_info.Password = string(packet.Data)
			active_clients[client.Id].Logged_in = true
			active_clients[client.Id].Account_info.Role = registered_accounts[index].Role
			active_clients_mutex.Unlock()
			registered_accounts_mutex.Unlock()
			return
		} else {
			packet.Type = DENY
			packet.Data = []byte("Incorrect password")
			send_data_packet(packet, client)
			registered_accounts_mutex.Unlock()
		}
	}
}

/*
 * This function checks if a user is banned from ther server
 */
func is_banned(user_index int) bool {
	registered_accounts_mutex.Lock()
	defer registered_accounts_mutex.Unlock()

	return registered_accounts[user_index].Banned
}

/*
 * This function handles messaging
 */
func message(client Client) {
	// looping until user goes back to main menu or quits
	for {
		// updating client
		client = update_client(client)

		// reading packet from client
		packet := read_data_packet(client)
		fmt.Printf("system: Received data packet from client #%d\n", client.Id)
		print_data_packet(packet)

		// checking if the client has changed state and this function needs to return
		if packet.Type == CLOSE {
			fmt.Println("closing message()")
			return
		}

		// checking if the packet has the expected type
		if packet.Type != MESSAGE && packet.Type != JOIN_MSG && packet.Type != LEAVE_MSG {
			custom_error_exit(OUT_OF_SYNC)
		}

		// sending message to everyone in the default chat
		active_clients_mutex.Lock()
		channels_mutex.Lock()
		for _, user := range channels[client.Current_channel].Users {
			if user != client.Id {
				send_data_packet(packet, *active_clients[user])
			}
		}
		channels_mutex.Unlock()
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
			return index, true
		}
	}

	// looping over active clients
	for index ,current_user := range active_clients {
		if current_user.Account_info.Username == username {
			return index, true
		}
	}

	return -1, false
}

/*
 * This function handles closing the server is a predefined error occures
 */
func error_exit(err error) {
	fmt.Println("system: ERROR -", err)
	fmt.Println("system: shutting down server...")

	// TODO
	os.Exit(1)
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
	// TODO
	os.Exit(1)
}

/*
 * This function handles removing a client from the active clients list
 */
func sub_client(client Client) {
	fmt.Println("system: Disconnecting client...")

	decrement_num_of_active_clients()

	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()

	active_clients[client.Id].data_sock = nil
	active_clients[client.Id].command_sock = nil
	active_clients[client.Id].Account_info.Username = ""
	active_clients[client.Id].Account_info.Password = ""
	active_clients[client.Id].State = CHOOSING_SIGN_IN_OPT
	active_clients[client.Id].Id = -1
	active_clients[client.Id].Logged_in = false
	active_clients[client.Id].Account_info.Role = 0
	active_clients[client.Id].Current_channel = -1

	client.command_sock.Close()
	client.data_sock.Close()
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
 * This function checks if an account is logged in
 * It returns true if an account is indeed logged in
 */
func is_logged_in(username string) bool {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()
	for _, user := range active_clients {
		if user.Account_info.Username == username {
			return true
		}

	}
	return false
}

/*
 * This function updates the state of a client
 */
func update_client_state(client Client, state int) Client {
	active_clients_mutex.Lock()
	active_clients[client.Id].State = state
	active_clients_mutex.Unlock()
	client.State = state
	return client
}

/*
 * This function sends a packet to a specified client
 */
func send_data_packet(packet Data_packet, client Client) {
	// marshaling data
	json_data := marshal_packet(packet)

	// sending packet
	write_to_connection(client.data_sock, json_data)
}

/*
 * This function reads a packet from a client
 */
func read_data_packet(client Client) Data_packet {
	json_data, amount_read := read_from_connection(client.data_sock)
	var packet Data_packet

	// checking if the packet is empty
	if amount_read > 0 {
		// unmarshaling json packet
		packet = unmarshal_packet(json_data[:amount_read])
	}

	return packet
}

/*
 * This function sends a packet to a specified client
 */
func send_command_packet(packet Command_packet, client Client) {
	// marshaling data
	json_data := marshal_command_packet(packet)

	// sending packet
	write_to_connection(client.command_sock, json_data)
}

/*
 * This function reads a packet from a client
 */
func read_command_packet(client Client) Command_packet {
	json_data, amount_read := read_from_connection(client.command_sock)
	fmt.Printf("server: Recieved packet from client: %d\n", client.Id)
	fmt.Printf("server: \"%s\"\n", string(json_data))

	var packet Command_packet

	if amount_read == -1 {
		packet.Type = CLOSE
		return packet
	}

	// checking if the packet is empty
	if amount_read < 0 {
		return packet
	}

	// unmarshaling json packet
	packet = unmarshal_command_packet(json_data[:amount_read])

	return packet
}

/*
 * This function writes to a specified net.Conn
 */
func write_to_connection(connection net.Conn, data []byte) {
	if connection == nil {
		return
	}

	_, err := connection.Write(data)
	if err != nil {
		fmt.Println("system: Failed to write to socket")
	}
}

/*
 * This function reads from a specified net.Conn
 */
func read_from_connection(connection net.Conn) ([]byte, int) {
	if connection == nil {
		return nil, -1
	}

	// reading packet from user
	data := make([]byte, MAX_PACKET_SIZE)
	amount_read, err := connection.Read(data)
	if err != nil {
		fmt.Println("system: Failed to read from socket")
	}
	return data, amount_read
}

/*
 * This function marshals a packet into a json file
 */
func marshal_packet(packet Data_packet) []byte {
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
func unmarshal_packet(json_data []byte) Data_packet {
	// unmarshaling json packet
	//json_data = trim_null_characters(json_data)
	var packet Data_packet
	err := json.Unmarshal(json_data, &packet)
	if err != nil {
		error_exit(err)
	}
	return packet
}

/*
 * This function marshals a packet into a json file
 */
func marshal_command_packet(packet Command_packet) []byte {
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
func unmarshal_command_packet(json_data []byte) Command_packet {
	var packet Command_packet
	err := json.Unmarshal(json_data, &packet)
	if err != nil {
		error_exit(err)
	}
	return packet
}

/*
 * This function handles incoming commands
 */
func handle_inbound_commands(client Client) {
	for {
		packet := read_command_packet(client)
		if packet.Type == -1 {
			return
		}
		if execute_command(packet, client) {
			fmt.Println("system: Closing command routine")
			return
		}
	}
}

/*
 * Executes a command
 */
func execute_command(command_packet Command_packet, client Client) bool {
	// parsing the command
	command := parse_command(command_packet)

	fmt.Printf("system: Recieved command of type \"%d\" with %d arguments from client #%d\n", command.Type, len(command.Args), client.Id)

	// switching on the command type
	switch command.Type {
	case HELP:
		fmt.Println("system: Running help command")
		help_command(client, command)
	case MAIN:
		fmt.Println("system: Running main command")
		main_command(client, command)
	case EXIT:
		fmt.Println("system: Running exit command")
		exit_command(client)
		return true
	case LOG_OUT:
	case LIST_C:
	case LIST_S:
	case DISCONNECT_C:
	case DISCONNECT_S:
	case BAN_C:
	case BAN_S:
		fmt.Println("system: Running ban-s command")
		ban_s_command(client, command)
	case CREATE:
		fmt.Println("system: Running create command")
		create_command(client, command)
	case DELETE:
	case CHANGE_TOPIC:
		fmt.Println("system: Running change-topic command")
		change_topic_command(client, command)
	case ADD_MOD:
		fmt.Println("system: Running add-mod command")
		add_mod_command(client, command)
	case RM_MOD:
		fmt.Println("system: Running rm-mod command")
		rm_mod_command(client, command)
	default:
		custom_error_exit(UNKNOWN)
		return true
	}
	return false
}

/*
 * This function parses a command packet into a command
 */
func parse_command(command_packet Command_packet) Parsed_command {
	// parsing command into an array of tokens
	args := strings.Split(string(command_packet.Arguments), ":")

	// creating command struct
	var command Parsed_command
	command.Type = command_packet.Type
	command.Username = command_packet.Username
	command.Successful = command_packet.Successful
	command.Message = command_packet.Message

	// checking if the command had arguments
	if len(command_packet.Arguments) > 0 {
		// adding the arguments to the struct
		for i := 0; i < len(args); i++ {
			command.Args = append(command.Args, args[i])
		}
	} else {
		// setting args to nil
		command.Args = nil
	}

	return command
}

/*
 * This function is executes the help command
 */
func help_command(client Client, command Parsed_command) {
	// updating client
	client = update_client(client)

	// creating return command packet
	var cpack Command_packet
	cpack.Type = HELP
	cpack.Username = client.Account_info.Username

	// checking if an arguemtns were passed to help
	if len(command.Args) > 0 {
		cpack.Arguments = []byte("Too many arguments")
		send_command_packet(cpack, client)
		return
	}

	cpack.Arguments = []byte("OK")
	send_command_packet(cpack, client)

	cpack = read_command_packet(client)
	if string(cpack.Arguments) != "READY" {
		custom_error_exit(UNEXPECTED_DATA)
	}

	// saving current state and then setting state to IN_HELP_SCREEN
	client = update_client(client)
	previous_state := client.State
	update_client_state(client, IN_HELP_SCREEN)
	client.State = IN_HELP_SCREEN

	// sending packet with user's role
	cpack = Command_packet{Type: HELP, Username: client.Account_info.Username, Arguments: []byte(strconv.Itoa(client.Account_info.Role))}
	send_command_packet(cpack, client)

	// reading packet from client
	cpack = read_command_packet(client)

	// checking if data is expected keyword
	if string(string(cpack.Arguments)) != "DONE" {
		custom_error_exit(UNEXPECTED_DATA)
	}

	// restoring state prior to command
	update_client_state(client, previous_state)
}

/*
 * This function handles the client exiting
 */
func exit_command(client Client) {
	// udpating client status to quitting
	update_client_state(client, QUITTING)

	// updating client to quitting
	disconnect_client(client)
}

func disconnect_client(client Client) {
	// updating the client's state to quitting
	update_client_state(client, QUITTING)

	// informing client that the state has been changed
	var cpack Command_packet
	cpack.Type = EXIT
	cpack.Username = client.Account_info.Username
	cpack.Arguments = []byte("READY")
	send_command_packet(cpack, client)

	cpack = read_command_packet(client)
	if client.Current_channel > -1 {
		leave_channel(client)
	}
	
	sub_client(client)
}

/*
 * This function handles the creat command
 */
func create_command(client Client, command Parsed_command) {
	// updating client struct
	client = update_client(client)

	// creating command packet to send to client
	var cpack Command_packet
	cpack.Type = CREATE
	cpack.Username = client.Account_info.Username

	// checking if client is in a state to enter this command
	if client.Account_info.Role <= PUBLIC {
		cpack.Message = []byte("You don't have permission to use this command")
		cpack.Arguments = nil
	} else if len(command.Args) < 1 {
		cpack.Message = []byte("Not enough arguments")
		cpack.Arguments = nil
	} else if len(command.Args) > 1 {
		cpack.Message = []byte("Too many arguments")
		cpack.Arguments = nil
	} else {
		// creating channel
		var successful bool
		cpack.Message, successful = create_channel(command)

		// checking if the channel was created successfully
		if successful {
			// building a string from the channel array
			var channel_list strings.Builder
			channels_mutex.Lock()
			for index, channel := range channels {
				if channel.Id != -1 {
					if index != 0 {
						channel_list.WriteString(" ")
					}
					channel_list.WriteString(string(channel.Topic))
				}
			}
			channels_mutex.Unlock()

			cpack.Arguments = []byte(channel_list.String())
			cpack.Successful = true
		} else {
			cpack.Arguments = nil
			cpack.Successful = false
		}
	}

	// sending packet
	send_command_packet(cpack, client)
}

/*
 * This function handles the main command
 */
func main_command(client Client, command Parsed_command) {
	// creating part of the return packet
	var cpack Command_packet
	cpack.Type = MAIN
	cpack.Username = client.Account_info.Username

	// checking if args were passed in with the command
	if len(command.Args) > 0 {
		cpack.Arguments = []byte("Too many arguments")
	} else if leave_channel(client) {
		update_client_state(client, IN_MAIN_MENU)
		dpack := Data_packet{Type: CLOSE, Username: client.Account_info.Username, Data: []byte("Going to main menu")}
		send_data_packet(dpack, client)
		cpack.Arguments = []byte("Success")
	} else {
		cpack.Arguments = []byte("Failed to leave channel")
	}

	// sending resonse to client
	send_command_packet(cpack, client)
}

/*
 *
 */
func change_topic_command(client Client, command Parsed_command) {
	// updating client struct
	client = update_client(client)

	var cpack Command_packet
	cpack.Type = CHANGE_TOPIC
	cpack.Username = client.Account_info.Username

	// ensuring proper arguments, permissions, and other requirements
	if client.Account_info.Role <= PUBLIC {
		cpack.Arguments = []byte("You don't have permission to use this command")
	} else if len(command.Args) < 2 {
		cpack.Arguments = []byte("Not enough arguments")
	} else if len(command.Args) > 2 {
		cpack.Arguments = []byte("Too many arguments")
	} else if command.Args[0] == "nonsense" {
		cpack.Arguments = []byte("Default channel. Cannot change this channel's topic")
	} else {
		index := get_channel_id([]byte(command.Args[0]))

		channels_mutex.Lock()
		defer channels_mutex.Unlock()

		// checking if the channel exists
		if index == -1 {
			cpack.Arguments = []byte("No chat found with the name \"" + string(command.Args[1]) + "\"")
		} else {
			channels[index].Topic = []byte(command.Args[1])
			cpack.Arguments = []byte("Successfully changed channel topic to #" + command.Args[0])
		}
	}

	// sending response to server
	send_command_packet(cpack, client)
}

/*
 * This function gives the moderator role to a user
 */
func add_mod_command(client Client, command Parsed_command) {
	// updating client struct
	client = update_client(client)

	// creating return packet
	var cpack Command_packet
	cpack.Type = ADD_MOD
	cpack.Username = client.Account_info.Username

	// checking requirements
	if client.Account_info.Role < ADMIN {
		cpack.Arguments = []byte("You don't have permission to use this command")
	} else if len(command.Args) < 1 {
		cpack.Arguments = []byte("Not enough arguments")
	} else if len(command.Args) > 1 {
		cpack.Arguments = []byte("Too many arguments")
	} else {

		active_clients_mutex.Lock()
		found_1 := false
		found_2 := false

		// finding account
		for _, user := range active_clients {
			if user.Account_info.Username == command.Args[0] {
				user.Account_info.Role = 1
				found_1 = true

				registered_accounts_mutex.Lock()
				for index, user := range registered_accounts {
					if user.Username == command.Args[0] {
						registered_accounts[index].Role = 1
						found_2 = true
						break
					}
				}
				registered_accounts_mutex.Unlock()
				break
			}
		}
		active_clients_mutex.Unlock()

		// checking if account exists
		if found_1 && found_2 {
			cpack.Arguments = []byte("Successfully gave " + command.Args[0] + " the moderator role")
		} else {
			cpack.Arguments = []byte("Did not find account with that name")
		}
	}

	fmt.Println("system: sending command status")

	// sending response
	send_command_packet(cpack, client)

	// saving changes to account
	save_accounts()
}

/*
 * This function gives the moderator role to a user
 */
func rm_mod_command(client Client, command Parsed_command) {
	// updating client struct
	client = update_client(client)

	var cpack Command_packet
	cpack.Type = ADD_MOD
	cpack.Username = client.Account_info.Username

	if client.Account_info.Role < ADMIN {
		cpack.Arguments = []byte("You don't have permission to use this command")
	} else if len(command.Args) < 1 {
		cpack.Arguments = []byte("Not enough arguments")
	} else if len(command.Args) > 1 {
		cpack.Arguments = []byte("Too many arguments")
	} else {

		active_clients_mutex.Lock()
		for _, user := range active_clients {
			if user.Account_info.Username == command.Args[0] {
				user.Account_info.Role = 0

				registered_accounts_mutex.Lock()
				for _, user := range registered_accounts {
					user.Role = 0
				}
				registered_accounts_mutex.Unlock()
			}
		}
		active_clients_mutex.Unlock()

		cpack.Arguments = []byte("Successfully removed the moderator role from " + command.Args[0])
	}

	// sending response
	send_command_packet(cpack, client)

	// saving account changes
	save_accounts()
}

/*
 * This function checks permmisions and requriemtns and then attmp to ban a user from 
 */
func ban_s_command(client Client, command Parsed_command) {
	// updating client struct
	client = update_client(client)

	// creating return packet
	var cpack Command_packet
	cpack.Type = BAN_S
	cpack.Username = client.Account_info.Username

	if client.Account_info.Role < MODERATOR { // checking if you have permission
		cpack.Arguments = []byte("You don't have permission to use this command")
	} else if len(command.Args) < 1 { // checking if there are enough arguments
		cpack.Arguments = []byte("Not enough arguments")
	} else if len(command.Args) > 1 { // checking if there are too many arguments
		cpack.Arguments = []byte("Too many arguments")
	} else if client.Account_info.Username == command.Args[0] { // checking if you are trying to ban yourself
		cpack.Arguments = []byte("Cannot ban yourself")
	} else {
		// getting the index of the user given their name
		user_index := get_user_index(command.Args[0])

		if user_index == -1 {
			cpack.Arguments = []byte("Could not find a user with that name")
		} else if client.Account_info.Role == ADMIN {
			// banning user from server
			ban_from_server(user_index)
			cpack.Arguments = []byte("Banned " + command.Args[0] + " from the server")
		} else {
			if is_public(user_index) {
				ban_from_server(user_index)
				cpack.Arguments = []byte("Banned " + command.Args[0] + " from the server")
			} else if command.Args[0] == "Admin" {
				cpack.Arguments = []byte("Cannot ban the Admin")
			} else {
				cpack.Arguments = []byte("Must be Admin to ban a moderator")
			}
		}
	}

	send_command_packet(cpack, client)
}

/*
 * This function handles udpating the account to be banned 
 */
func ban_from_server(user_index int) {
	registered_accounts_mutex.Lock()
	defer registered_accounts_mutex.Unlock()

	registered_accounts[user_index].Banned = true

	save_accounts()

	// TODO: Disconnect the user from the server
}

/*
 * This function checks if an account has the public role
 */
func is_public(user_index int) bool {
	registered_accounts_mutex.Lock()
	defer registered_accounts_mutex.Unlock()

	return registered_accounts[user_index].Role == PUBLIC
}

/*
 * This function gets the index of an account given the accounts username
 */
func get_user_index(username string) int {
	registered_accounts_mutex.Lock()
	defer registered_accounts_mutex.Unlock()

	// looping over registered accounts
	for index, user := range registered_accounts {
		if user.Username == username {
			return index
		}
	}

	return -1
}

/*
 * This function prints data packets
 */
func print_data_packet(packet Data_packet) {
	fmt.Println("---------------------------------------------------")
	fmt.Printf(" - TYPE\t\t%d\n", packet.Type)
	fmt.Printf(" - Username\t\t%s\n", packet.Username)
	fmt.Printf(" - DATA\t\t%s\n", string(packet.Data))
	fmt.Println("---------------------------------------------------")
}

/*
 * This function clears the terminal
 */
func clear_terminal() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

/*
 * This function creates a channel
 */
func create_channel(command Parsed_command) ([]byte, bool) {
	// creating channel struct
	channel := Channel{Topic: []byte(command.Args[0]), Users: nil}

	// finding free slot for channel
	free_slot_index := find_free_channel_slot()
	if free_slot_index == -1 {
		return []byte("Maximum number of channels already exist"), false
	}

	// adding channel to array
	channels_mutex.Lock()
	defer channels_mutex.Unlock()
	channels[free_slot_index] = &channel

	// returning success message
	return []byte("Successfull added a channel with topic #" + command.Args[0]), true
}

/*
 * This function find a free channel slot to store the new channel
 */
func find_free_channel_slot() int {
	channels_mutex.Lock()
	defer channels_mutex.Unlock()

	// looping through channels array to find slot
	for index, channel := range channels {
		if channel.Id == -1 {
			return index
		}
	}
	return -1
}

/*
 * This funtion handles the functionality of the main menu
 */
func main_menu(client Client) {
	fmt.Println("Made it to main menu")
	// reading packet from client
	data_packet := read_data_packet(client)

	// checking if the client has changed state and this function needs to return
	if data_packet.Type == CLOSE {
		return
	}

	// validating packet adn confirming client is ready
	if data_packet.Type != MAIN || string(data_packet.Data) != "READY" {
		custom_error_exit(OUT_OF_SYNC)
	}

	// sending channels to client
	var channel_list strings.Builder
	channels_mutex.Lock()
	for index, channel := range channels {
		if channel.Id != -1 {
			if index != 0 {
				channel_list.WriteString(" ")
			}
			channel_list.WriteString(string(channel.Topic))
		}
	}
	channels_mutex.Unlock()

	// preparing packet
	data_packet = Data_packet{Type: MAIN, Username: client.Account_info.Username, Data: []byte(channel_list.String())}
	send_data_packet(data_packet, client)

	// reading packet from client
	packet := read_data_packet(client)

	// checking if the client has changed state and this function needs to return
	if packet.Type == CLOSE {
		return
	}

	// checking if the packet has the expected type
	if packet.Type != MENU_OPTION {
		custom_error_exit(OUT_OF_SYNC)
	}

	// converting string to int
	user_choice, err := strconv.Atoi(string(packet.Data))
	if err != nil {
		error_exit(err)
	}

	// joining a channel
	join_channel(client, user_choice)

	// updating client status
	update_client_state(client, MESSAGING)
}

/*
 * This function joins a specific channel
 */
func join_channel(client Client, channel_id int) {
	channels_mutex.Lock()
	defer channels_mutex.Unlock()

	// adding user id to list of users in channel
	channels[channel_id].Users = append(channels[channel_id].Users, client.Id)

	active_clients_mutex.Lock()
	active_clients[client.Id].Current_channel = channel_id
	client.Current_channel = channel_id
	active_clients_mutex.Unlock()
	msg := "\n" + client.Account_info.Username + " has joined the chat\n" + time.Now().Format("3:04 PM") + "\n"
	send_message(client, JOIN_MSG, msg)
}

/*
 * This function udpates a client struct
 */
func update_client(client Client) Client {
	active_clients_mutex.Lock()
	defer active_clients_mutex.Unlock()
	return *active_clients[client.Id]
}

/*
 * This function handles leaving a channel
 */
func leave_channel(client Client) bool {
	// updating client
	client = update_client(client)

	channels_mutex.Lock()
	defer channels_mutex.Unlock()

	// looping over users in a channel
	for index, user := range channels[client.Current_channel].Users {
		// checking if we found the user
		if user == client.Id {

			// sending leaving message
			msg := "\n" + client.Account_info.Username + " has left the chat\n" + time.Now().Format("3:04 PM") + "\n"
			send_message(client, LEAVE_MSG, msg)

			// removing user from channel
			if index+1 == len(channels[client.Current_channel].Users) {
				if len(channels[client.Current_channel].Users) == 0 {
					channels[client.Current_channel].Users = nil
				} else {
					channels[client.Current_channel].Users = channels[client.Current_channel].Users[:len(channels[client.Current_channel].Users)-1]
				}
				return true
			} else {
				channels[client.Current_channel].Users = append(channels[client.Current_channel].Users[:index], channels[client.Current_channel].Users[index+1:]...)
				return true
			}

		}
	}

	return false
}

/*
 * This function sends a message to the channel that a client is currently in
 */
func send_message(client Client, msg_type int, msg string) {
	var packet Data_packet

	// checking if user left or joined
	if msg_type == JOIN_MSG {
		packet = Data_packet{Type: JOIN_MSG, Data: []byte(msg)}
	} else {
		packet = Data_packet{Type: LEAVE_MSG, Data: []byte(msg)}
	}

	// sending message to channel
	active_clients_mutex.Lock()
	for _, user := range channels[client.Current_channel].Users {
		if user != client.Id {
			send_data_packet(packet, *active_clients[user])
		}
	}
	active_clients_mutex.Unlock()
}

/*
 * This function gets a channel ID given a name
 */
func get_channel_id(topic []byte) int {
	channels_mutex.Lock()
	defer channels_mutex.Unlock()

	// looping over channels
	for index, channel := range channels {
		if string(channel.Topic) == string(topic) {
			return index
		}
	}
	return -1
}