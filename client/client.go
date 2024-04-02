package main

// imported packages
import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eiannone/keyboard"
	"golang.org/x/crypto/ssh/terminal" // Import the terminal package
)

// server information
const (
	SERVER_HOST = "localhost"
	CLIENT_HOST = "localhost"
	COMMAND_PORT = "7777"
	DATA_PORT = "7778"
	CONNECTION_TYPE = "tcp"
)

// general constants
const (
	MAX_PACKET_SIZE = 1024
)

// ansi colors constants
const (
	Reset 	= "\033[0m"
	Green 	= "\033[32m"
	Yellow 	= "\033[33m"
)

// client states
const (
	REGISTERING          = iota // registing account
	MESSAGING                   // messaging group chat
	QUITTING                    // quitting application
	LOGGING_IN                  // logging in to existing account
	CHOOSING_SIGN_IN_OPT        // selecting to log in register or exit
)

// packet types
const (
	ACCEPT = iota
	DENY
	MESSAGE
	REGISTRATION
	QUIT
	LOGIN
	CHOOSE_SIGN_IN_OPT
	CHAT_STATUS_MSG
	COMMAND
	CONNECT
)

// commands type
const (
	DNE = iota // This an invalid command
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

// roles for the client
const (
	PLEB      = iota // 0
	MODERATOR        // 1
	ADMIN            // 2
)

// struct to hold packet information
type packet struct {
	Type     int
	Username string
	Data     []byte
}

type command_packet struct {
	Type 		int
	Username 	string
	Arguments 	[]byte
}

var client_status int
var username string
var terminal_width int
var terminal_height int
var horizontal_line []byte
var vertical_space []byte
var command_socket net.Conn
var data_socket net.Conn

var (
	chat_strand []packet
	mutex_chat  sync.Mutex
)

// this creates a mapping of the strings on the
// left to the integers on the right
var command_to_int = map[string]int{
	"/help":         1,
	"/main":         2,
	"/log-out":      3,
	"/list-c":       4,
	"/list-s":       5,
	"/disconnect-c": 6,
	"/disconnect-s": 7,
	"/ban-c":        8,
	"/ban-s":        9,
	"/create":       10,
	"/delete":       11,
	"/change-topic": 12,
	"/add-mod":      13,
	"/rm-mod":       14,
}

/*
 * This is the main function of the server
 */
func main() {
	// initialize client
	initialize_client()

	// main loop for handling states of the client
	for {
		switch client_status {
		case CHOOSING_SIGN_IN_OPT:
			choose_sign_in_opt()
		case LOGGING_IN:
			login()
		case REGISTERING:
			register_user()
		case MESSAGING:
			message()
		case QUITTING:
			shutdown()
		}
	}
}

/*
 * This function initializes the client
 */
func initialize_client() {
	clear_terminal()
	client_status = CHOOSING_SIGN_IN_OPT
	get_terminal_dimensions()
	create_horizantal_line()
	create_vertical_space()
	connect_to_server()
	establish_data_connection()
	setup_signal_handler()
	print_client_status()
	print_splash_screen()
}

/*
 * This function attempts to connect to the
 * server and closes the client if it fails
 */
func connect_to_server() {
	var err error
	command_socket, err = net.Dial(CONNECTION_TYPE, SERVER_HOST+":"+COMMAND_PORT)
	if err != nil {
		error_exit(err)
	}
}

/*
 * This function sends a packet to the server to inform
 * it that the client is disconnecting
 */
func disconnect_from_server() {
	packet := packet{Type: QUIT, Data: []byte("Error with client")}
	send_data_packet(packet)
	command_socket.Close()
	data_socket.Close()
}

/*
 * This function creates a signal catcher for
 * when the user enter ctrl-c
 */
func setup_signal_handler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT)
	go handleSigInt(sigChan)
}

/*
 * This function prints the client status after successfully launching
 */
func print_client_status() {
	// printing connection status
	fmt.Println(string(horizontal_line))
	fmt.Println("system: Successfully connected to server")
	fmt.Println("\t- address:\t ", SERVER_HOST)
	fmt.Println("\t- port:\t\t ", COMMAND_PORT)
	fmt.Println(string(horizontal_line))
	time.Sleep(1 * time.Second)
}

/*
 * This function handles the client choosing to either sign in or register
 */
func choose_sign_in_opt() {
	var packet packet
	current_choice := 0

	// creating channel to send client choice
	choice_channel := make(chan int)

	// creating routine to handle printing the sign in menu
	go display_sign_in_menu(choice_channel)

	// sending default choice to display_sign_in_menu
	choice_channel <- 0

	// creating a keyboard reader
	if err := keyboard.Open(); err != nil {
		panic(err)
	}
	defer keyboard.Close()

	// looping until enter is pressed
	for {
		// getting key press
		_, key, err := keyboard.GetSingleKey()
		if err != nil {
			panic(err)
		}

		// Check if the pressed key is the up or down arrow
		if key == keyboard.KeyArrowUp {
			current_choice += 2
			current_choice = current_choice % 3
			choice_channel <- current_choice
		} else if key == keyboard.KeyArrowDown {
			current_choice++
			current_choice = current_choice % 3
			choice_channel <- current_choice
		}

		// Break the loop if Enter key is pressed
		if key == keyboard.KeyEnter {
			// send signal to display_sign_in_menu
			choice_channel <- 3

			// setting state based on choice and preparing
			// packet to be sent to the server with the choice
			if current_choice == 0 {
				packet.Type = CHOOSE_SIGN_IN_OPT
				packet.Data = []byte("login")
				client_status = LOGGING_IN
			} else if current_choice == 1 {
				packet.Type = CHOOSE_SIGN_IN_OPT
				packet.Data = []byte("register")
				client_status = REGISTERING
			} else if current_choice == 2 {
				packet.Type = QUIT
				packet.Data = []byte("client disconnecting")
				client_status = QUITTING
			}
			break
		}
	}

	// sending choice to server
	send_data_packet(packet)
}

/*
 * This function is responcible for registering the client with the server.
 * This includes getting a username and password to create an account
 */
 func register_user() {
	get_username_for_registration()
	get_password_for_registration()
}

/*
 * This function loops until it gets a valid username from the user
 */
 func get_username_for_registration()() {
	// clearing terminal
	clear_terminal()

	// creating scanner
	scanner := bufio.NewScanner(os.Stdin)

	// prompting user
	fmt.Println(string(horizontal_line))
	fmt.Println("Please enter a username. (NOTE: This will be visible to all other users)")
	fmt.Println("Username requirements:")
	fmt.Println("- Must start with: \"A-Z\" or \"a-z\"")
	fmt.Println("- Must end with: \"A-Z\", \"a-z\", or \"0-9\"")
	fmt.Println("- May contain: \"A-Z\", \"a-z\", \"0-9\", \"-\", \"_\"")
	fmt.Println("- Must be 5-20 characters long")
	fmt.Println(string(horizontal_line))

	// looping until user either picks a valid username or exits
	var input string
	for {
		arrow := Green + "-> " + Reset
		fmt.Print(arrow)

		// getting input from user
		if scanner.Scan() {
			// storing scanned text in variable
			input = scanner.Text()
		}
		fmt.Println(string(horizontal_line))

		// checking if user typed /exit
		if check_and_quit(input) {
			return
		}

		if is_comand(input) {
			handle_command(input)
			clear_terminal()
			fmt.Println(string(horizontal_line))
			fmt.Println("Please enter a username. (NOTE: This will be visible to all other users)")
			fmt.Println("Username requirements:")
			fmt.Println("- Must start with: \"A-Z\" or \"a-z\"")
			fmt.Println("- Must end with: \"A-Z\", \"a-z\", or \"0-9\"")
			fmt.Println("- May contain: \"A-Z\", \"a-z\", \"0-9\", \"-\", \"_\"")
			fmt.Println("- Must be 5-20 characters long")
			fmt.Println(string(horizontal_line))
			continue
		}

		// declaring and initializing packet
		packet := packet{Type: REGISTRATION, Data: []byte(input)}

		// sending packet containging username
		send_data_packet(packet)

		// waiting for response
		packet = read_data_packet()

		// checking response from server
		if packet.Type == DENY {
			// printing response
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
		} else if packet.Type == ACCEPT {
			// printing response
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))

			// setting global value for username
			username = input

			break
		}
	}
 }

 /*
 * This function loops until it gets a valid password from the user
 */
 func get_password_for_registration()() {
	// clearing terminal
	clear_terminal()

	// creating scanner
	scanner := bufio.NewScanner(os.Stdin)

	// prompting user
	fmt.Println(string(horizontal_line))
	fmt.Println("Please enter a passowrd")
	fmt.Println("Password requirements:")
	fmt.Println("- Must contain at least one captial letter")
	fmt.Println("- Must contain at least one number")
	fmt.Println("- Must contain at least one special character (!, @, #, $, %, ?)")
	fmt.Println("- Must be at least 7 characters long")
	fmt.Println(string(horizontal_line))

	// looping until user enters a valid password or exits
	var input string
	for {
		arrow := Green + "-> " + Reset
		fmt.Print(arrow)

		// getting input from user
		if scanner.Scan() {

			// storing scanned text in variable
			input = scanner.Text()
		}
		fmt.Println(string(horizontal_line))

		// checking if user typed /exit
		if check_and_quit(input) {
			return
		}

		if is_comand(input) {
			handle_command(input)
			clear_terminal()
			fmt.Println(string(horizontal_line))
			fmt.Println("Please enter a passowrd")
			fmt.Println("Password requirements:")
			fmt.Println("- Must contain at least one captial letter")
			fmt.Println("- Must contain at least one number")
			fmt.Println("- Must contain at least one special character (!, @, #, $, %, ?)")
			fmt.Println("- Must be at least 7 characters long")
			fmt.Println(string(horizontal_line))
			continue
		}

		// declaring and initializing packet
		packet := packet{Type: REGISTRATION, Data: []byte(input)}

		// sending password packet
		send_data_packet(packet)

		// waiting on response
		packet = read_data_packet()

		// checking response from server
		if packet.Type == DENY {
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
		} else if packet.Type == ACCEPT {
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
			client_status = MESSAGING
			break
		}
	}
}

/*
 * This function handles logging in the client
 */
 func login() {
	var packet packet
	var input string

	// creatin scanner
	scanner := bufio.NewScanner(os.Stdin)

	// clearing terminal
	clear_terminal()

	// prompting user
	fmt.Println("Enter your username below:")
	fmt.Println(string(horizontal_line))

	// looping until user enters a valid username or exits
	for {
		arrow := Green + "-> " + Reset
		fmt.Print(arrow)

		// getting user input
		if scanner.Scan() {
			input = scanner.Text()
		}
		fmt.Println(string(horizontal_line))

		// checking if the user entered /exit
		if check_and_quit(input) {
			return
		}

		if is_comand(input) {
			fmt.Println("It is a command")
			handle_command(input)
			clear_terminal()
			fmt.Println(string(horizontal_line))
			fmt.Println("Enter your username below:")
			fmt.Println(string(horizontal_line))
			continue
		}

		// setting this clients username
		username = input

		// initializing packet
		packet.Type = LOGIN
		packet.Data = []byte(username)
		send_data_packet(packet)

		// reading packet from server
		packet = read_data_packet()

		// checking if username was accepted
		if packet.Type == ACCEPT {
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
			break
		} else {
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
		}
	}

	// sleeping to give user time to see message
	time.Sleep(1500 * time.Microsecond)

	// clearing terminal
	clear_terminal()

	// prompting user
	fmt.Println("Enter your password below:")
	fmt.Println(string(horizontal_line))

	// looping until user enters valid password or exits
	for {
		arrow := Green + "-> " + Reset
		fmt.Print(arrow)
		var password string

		// getting user input
		if scanner.Scan() {
			fmt.Println("scanning in password loop")
			password = scanner.Text()
		}
		fmt.Println(string(horizontal_line))

		// checking if the user entered /exit
		if check_and_quit(password) {
			return
		}

		// checks if a command was entered and executes it if it was
		if is_comand(password) {
			handle_command(password)
			clear_terminal()
			fmt.Println(string(horizontal_line))
			fmt.Println("Enter your password below:")
			fmt.Println(string(horizontal_line))
			continue
		}

		// initializing packet
		packet.Type = LOGIN
		packet.Data = []byte(password)
		send_data_packet(packet)

		// reading packet from server
		packet = read_data_packet()

		// determining if password was accepted
		if packet.Type == ACCEPT {
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
			client_status = MESSAGING
			break
		} else {
			fmt.Printf("system: %s\n", string(packet.Data))
			fmt.Println(string(horizontal_line))
		}
	}

	// sleeping to give user time to see message
	time.Sleep(1500 * time.Microsecond)
}

/*
 * This function is responcible for handling messaging
 */
 func message() {
	var input string

	// starting a go routine to handle inbound messages
	go handle_inbound_msg()

	// send join message
	msg := "\n" + username + " has joined the chat\n" + time.Now().Format("3:04 PM") + "\n"
	packet := packet{Type: CHAT_STATUS_MSG, Data: []byte(msg)}
	send_data_packet(packet)

	// printing the chat strand
	print_chat_strand()

	// creating a scanner to get user input
	scanner := bufio.NewScanner(os.Stdin)

	// scanning user inputs and sending messages
	for {
		if scanner.Scan() {
			// storing scanned text in variable
			input = scanner.Text()
		}
		fmt.Println(string(horizontal_line))

		// checking if user typed /exit
		if check_and_quit(input) {
			return
		}

		// checks if a command was entered and executes it if it was
		if is_comand(input) {
			handle_command(input)
			clear_terminal()
			print_chat_strand()
			continue
		}

		// declaring and initializing packet
		packet.Type = MESSAGE
		packet.Data = []byte(input)
		packet.Username = username

		// adding new message to chat strand
		chat_strand = append(chat_strand, packet)

		// sending message to server
		send_data_packet(packet)

		// reprinting chat strand
		if client_status == MESSAGING {
			print_chat_strand()
		}
	}
}

/*
 * This function is responcible for handling inbound messages
 */
 func handle_inbound_msg() {
	// reading inbound messages
	for {
		// reading packet
		packet := read_data_packet()

		// checking packet type
		if packet.Type == MESSAGE || packet.Type == CHAT_STATUS_MSG {
			// appending new message to chat strand
			mutex_chat.Lock()
			chat_strand = append(chat_strand, packet)
			mutex_chat.Unlock()

			if client_status == MESSAGING {
				// reprinting updated chat strand
				print_chat_strand()
			}
		}
	}
}

/*
 * This function handles when the user presses ctrl-c
 */
 func handleSigInt(sigChan chan os.Signal) {
	// Wait for a SIGINT signal
	<-sigChan

	// informing client that the signal was recieved
	fmt.Println("\nExiting CHAT 429")

	// declaring and initializing packet
	var packet packet
	packet.Type = QUIT

	// sending quit packet to server
	send_data_packet(packet)

	// closing connection
	data_socket.Close()
	command_socket.Close()

	// exiting program
	os.Exit(0)
}

/*
 * This function clears the terminal
 */
 func clear_terminal() {
	// Clear the screen
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

/*
 * This function creates a horizantal line based on terminal size
 */
 func create_horizantal_line() {
	for i := 0; i < terminal_width; i++ {
		horizontal_line = append(horizontal_line, '-')
	}
}

/*
 * This function creates a vertical space the same hight as the terminal
 */
func create_vertical_space() {
	for i := 0; i < terminal_height; i++ {
		vertical_space = append(vertical_space, '\n')
	}
}

/*
 * This function creates a second connection with the server for data
 */
func establish_data_connection() {
	// creating a tempary passive socket to listen for the server connecting
	tmp_passive_socket := create_socket()

	// creating connection packet to be sent
	packet := command_packet{Type: CONNECT, Username: username, Arguments: []byte(CLIENT_HOST + ":" + DATA_PORT)}

	// sending connection packet
	send_command_packet(packet)

	// waiting for the server to accept
	data_socket = accept_connection(tmp_passive_socket)

	// closing the passive socket
	tmp_passive_socket.Close()
}

/*
 * This function creates a listening socket and handles the possible errors
 */
 func create_socket() net.Listener {
	fmt.Println("system: Creating socket...")

	// creating socket
	listener, err := net.Listen(CONNECTION_TYPE, CLIENT_HOST+":"+DATA_PORT)

	// handling erros
	if err != nil {
		error_exit(err)
	}

	fmt.Println("system: Successfully created socket")

	return listener
}

/*
 * This function accepts a client's connection
 * It exits if there is an error and returns the net.Conn upon success
 */
 func accept_connection(passive_socket net.Listener) net.Conn {
	connection, err := passive_socket.Accept()
	if err != nil {
		fmt.Println("Error accepting: ", err.Error())
		os.Exit(1)
	}
	fmt.Println("system: server connected")
	return connection
}

/*
 * This function gets passed a type of error and closes the
 * client while informing the server that the client is disconnecting
 */
 func error_exit(err error) {
	clear_terminal()
	fmt.Println("system: ERROR -", err)
	disconnect_from_server()
	os.Exit(1)
}

// -------------------------------------------------------------------------------------------------------

/*
 * This function formats and prints the chat strand
 */
func print_chat_strand() {
	// clearing terminal
	clear_terminal()

	// forcing text box to bottom of screen
	fmt.Print("\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n")
	fmt.Println(string(horizontal_line))
	start_of_chat_banner := "This is the beggining of the COMP 429 group chat"
	fmt.Printf("%*s\n", ((terminal_width-len(start_of_chat_banner))/2)+len(start_of_chat_banner), start_of_chat_banner)
	fmt.Print("\n\n")

	// looping over the chat strand to print all messages
	for _, packet := range chat_strand {
		// checking if the chat strand is empty
		if chat_strand != nil {

			if packet.Type == CHAT_STATUS_MSG {
				status_message := Yellow + string(packet.Data) + Reset
				fmt.Println(status_message)
				continue
			}

			// checking if its a message the client sent
			if packet.Username == username {
				// creating header for message
				username := Green + "You" + Reset + ": "

				// creating buffer with padding
				username_buffer := make([]byte, 27)
				for i := range username_buffer {
					username_buffer[i] = ' '
				}
				copy(username_buffer, []byte(username))
				fmt.Print("\n")
				fmt.Printf("%*s\n", terminal_width, string(username_buffer))

				// printing top of message bubble
				fmt.Printf("%*s\n", terminal_width, " __________________________")
				fmt.Printf("%*s\n", terminal_width, "|                          ")

				// formatting messages to fit in bubble
				var last_space int
				var temp_string string
				index := 0
				closest_space := 0
				message := []byte(packet.Data)
				output := make([]byte, 27)

				// looping over characters in message
				for _, character := range packet.Data {
					// marking space as the closest space to the current character
					if character == ' ' {
						closest_space = index
					}

					// finding the closest space and print up to that point when reaching the end of the line
					if index%24 == 0 && index != 0 {
						// adding spaces to create pading
						for i := range output {
							output[i] = ' '
						}

						// differentiating between single word and multi-word messages
						if last_space > 0 {
							temp_string = "| " + string(message[last_space+1:closest_space])
						} else {
							temp_string = "| " + string(message[last_space:closest_space])
						}

						// copying current line to output buffer
						copy(output, []byte(temp_string))

						// printing output buffer
						fmt.Printf("%*s\n", terminal_width, string(output))

						// updating last space
						last_space = closest_space
					}
					index++
				}

				// creating buffer with padding
				for i := range output {
					output[i] = ' '
				}

				// creating last line of message
				if last_space > 0 {
					temp_string = "| " + string(message[last_space+1:])
				} else {
					temp_string = "| " + string(message[last_space:])
				}

				// coping last line to output buffer
				copy(output, []byte(temp_string))

				// printing output buffer
				fmt.Printf("%*s\n", terminal_width, string(output))

				// printing bottom of bubble
				fmt.Printf("%*s\n", terminal_width, "|__________________________")
			} else {
				// creating header for message
				username_header := Green + "\n" + packet.Username + Reset + ": "

				// printing header
				fmt.Println(username_header)

				// printing top half of bubble
				fmt.Println("__________________________ ")
				fmt.Println("                          |")

				// formatting messages to fit in bubble
				var last_space int
				var temp_string string
				index := 0
				closest_space := 0
				message := []byte(packet.Data)
				output := make([]byte, 27)

				// looping over characters in message
				for _, character := range packet.Data {
					// marking space as the closest space to the current character
					if character == ' ' {
						closest_space = index
					}

					// finding the closest space and print up to that point when reaching the end of the line
					if index%24 == 0 && index != 0 {
						for i := range output {
							output[i] = ' '
						}

						// differentiating between single word and multi-word messages
						if last_space > 0 {
							temp_string = string(message[last_space+1 : closest_space])
						} else {
							temp_string = string(message[last_space:closest_space])
						}

						// coping last line to output buffer
						copy(output, []byte(temp_string))
						output[26] = '|'
						fmt.Println(string(output))
						last_space = closest_space
					}
					index++
				}

				// creating buffer with padding
				for i := range output {
					output[i] = ' '
				}

				// differentiating between single word and multi-word messages
				if last_space > 0 {
					temp_string = string(message[last_space+1:])
				} else {
					temp_string = string(message[last_space:])
				}

				// coping last line to output buffer
				copy(output, []byte(temp_string))
				output[26] = '|'
				fmt.Println(string(output))
				fmt.Println("__________________________|")
			}
		}
	}

	// checking if there are messages in the strand
	fmt.Print("\n\n\n")
	fmt.Println(string(horizontal_line))
	arrow := Green + "-> " + Reset
	fmt.Print(arrow)
}

/*
 * This function prints the splash screen for the program
 */
func print_splash_screen() {
	clear_terminal()
	loading_bar := make([]byte, terminal_width)
	for i := 0; i < terminal_width; i++ {
		fmt.Print(string(vertical_space[0:(terminal_height-15)/2]))
		banner_line_1 := "__        __   _                         "
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_1))/2)+len(banner_line_1), banner_line_1)
		banner_line_2 := "\\ \\      / /__| | ___ ___  _ __ ___   ___"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_2))/2)+len(banner_line_2), banner_line_2)
		banner_line_3 := " \\ \\ /\\ / / _ \\ |/ __/ _ \\| '_ ` _ \\ / _ \\"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_3))/2)+len(banner_line_3), banner_line_3)
		banner_line_4 := "  \\ V  V /  __/ | (_| (_) | | | | | |  __/"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_4))/2)+len(banner_line_4), banner_line_4)
		banner_line_5 := "   \\_/\\_/ \\___|_|\\___\\___/|_| |_| |_|\\___|"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_5))/2)+len(banner_line_5), banner_line_5)
		banner_line_6 := "_____  "
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_6))/2)+len(banner_line_6), banner_line_6)
		banner_line_7 := "|_   _|__"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_7))/2)+len(banner_line_7), banner_line_7)
		banner_line_8 := "  | |/ _ \\"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_8))/2)+len(banner_line_8), banner_line_8)
		banner_line_9 := "   | | (_) |"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_9))/2)+len(banner_line_9), banner_line_9)
		banner_line_10 := "  |_|\\___/"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_10))/2)+len(banner_line_10), banner_line_10)
		banner_line_11 := "  ____ _   _    _  _____ _  _  ____   ___"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_11))/2)+len(banner_line_11), banner_line_11)
		banner_line_12 := " / ___| | | |  / \\|_   _| || ||___ \\ / _ \\"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_12))/2)+len(banner_line_12), banner_line_12)
		banner_line_13 := "  | |   | |_| | / _ \\ | | | || |_ __) | (_) |"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_13))/2)+len(banner_line_13), banner_line_13)
		banner_line_14 := "  | |___|  _  |/ ___ \\| | |__   _/ __/ \\__, |"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_14))/2)+len(banner_line_14), banner_line_14)
		banner_line_15 := " \\____|_| |_/_/   \\_\\_|    |_||_____|  /_/"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_15))/2)+len(banner_line_15), banner_line_15)
		fmt.Print(string(vertical_space[0:(terminal_height - (terminal_height-15)/2) - 15 - 3]))
		fmt.Println(string(horizontal_line))
		fmt.Println(string(loading_bar))
		fmt.Println(string(horizontal_line))

		// increasing loading bar by one each iteration
		loading_bar = append(loading_bar, '=')
		time.Sleep(30 * time.Millisecond)
	}
}

/*
 * This function gets the terminal dimensions
 */
func get_terminal_dimensions() {
	// Get the file descriptor for standard output
	fd := int(syscall.Stdout)

	// Get the terminal size
	width, height, err := terminal.GetSize(fd)
	if err != nil {
		fmt.Println("Error getting terminal size:", err)
		return
	}

	// setting global vars
	terminal_width = width
	terminal_height = height
}

/*
 * This function is called as a go routine to display the sign in options
 */
func display_sign_in_menu(choice chan int) {
	currently_selected := 0
	opt_1 := 0
	opt_2 := 0
	opt_3 := 0

	for {
		select {
		case c := <-choice:
			switch c {
			case 3:
				return
			case 0:
				currently_selected = 0
			case 1:
				currently_selected = 1
			case 2:
				currently_selected = 2
			}
		default:
			switch currently_selected {
			case 0:
				if opt_1 == 16 {
					opt_1 = 0
				} else if opt_1 < 8 {
					display_opt_1_a()
					opt_1++
				} else if opt_1 < 16 {
					display_opt()
					opt_1++
				}
				time.Sleep(30 * time.Millisecond)

			case 1:
				if opt_2 == 16 {
					opt_2 = 0
				} else if opt_2 < 8 {
					display_opt_2_a()
					opt_2++
				} else if opt_2 < 16 {
					display_opt()
					opt_2++
				}
				time.Sleep(30 * time.Millisecond)
			case 2:
				if opt_3 == 16 {
					opt_3 = 0
				} else if opt_3 < 8 {
					display_opt_3_a()
					opt_3++
				} else if opt_3 < 16 {
					display_opt()
					opt_3++
				}
				time.Sleep(30 * time.Millisecond)
			}
		}
	}
}

/*
 * This function prints a screen with the login option selected
 */
func display_opt_1_a() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print("\n\n\n\n\n\n")
	line_3 := "--> LOGIN <--"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "CREATE ACCOUNT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "QUIT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
}

/*
 * This function prints a screen with the register option selected
 */
func display_opt_2_a() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print("\n\n\n\n\n\n")
	line_3 := "LOGIN"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "--> CREATE ACCOUNT <--"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "QUIT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
}

/*
 * This function prints a screen with the quit option selected
 */
func display_opt_3_a() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print("\n\n\n\n\n\n")
	line_3 := "LOGIN"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "CREATE ACCOUNT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "--> QUIT <--"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
}

func display_opt() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print("\n\n\n\n\n\n")
	line_3 := "LOGIN"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "CREATE ACCOUNT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "QUIT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
}

// sends quit message to server
func send_quit_packet() {
	var packet packet
	packet.Type = QUIT

	send_data_packet(packet)
}

// shutsdown client
func shutdown() {
	fmt.Println("system: Shutting down...")
	command_socket.Close()
	data_socket.Close()
	os.Exit(0)
}

// checks if user entered /exit
func check_and_quit(input string) bool {
	if input == "/exit" {
		send_quit_packet()
		client_status = QUITTING
		return true
	}
	return false
}

func handle_command(input string) {
	// sending the command to the server
	packet := parse_command(input)

	// send command to server
	send_command_packet(packet)

	switch packet.Type {
	case HELP:
		help_command()
	}
}

func help_command() {
	packet := read_command_packet()
	fmt.Println("Recieved command packet from server")
	quit_channel := make(chan int)

	if string(packet.Arguments) == "0" {
		go display_help_screen(quit_channel, 0)
	} else if string(packet.Arguments) == "1" {
		go display_help_screen(quit_channel, 1)
	} else if string(packet.Arguments) == "2" {
		go display_help_screen(quit_channel, 2)
	} else {
		os.Exit(1)
	}

	quit_channel <- 1

	if err := keyboard.Open(); err != nil {
		panic(err)
	}
	defer keyboard.Close()

	for {
		// getting key press
		char, _, err := keyboard.GetSingleKey()
		if err != nil {
			panic(err)
		}

		if char == 'q' || char == 'Q' {
			quit_channel <- 0
			break
		}
	}

	packet.Type = HELP
	packet.Username = username
	packet.Arguments = []byte("DONE")

	send_command_packet(packet)
}

func display_help_screen(quit chan int, role int) {
	for {
		select {
		case q := <-quit:
			switch q {
			case 0:
				return
			case 1:
				clear_terminal()
				fmt.Println(string(horizontal_line))
				fmt.Println("Below is a list of command and descriptions of what they do.  Press 'q' to quit")
				fmt.Println(string(horizontal_line))
				display_public_commands()
				if role == 1 {
					display_moderator_commands()
				} 

				if role > 2 {
					display_admin_commands()
				}
			default:
				return
			}
		default:
			clear_terminal()
			fmt.Println(string(horizontal_line))
			fmt.Println("Below is a list of command and descriptions of what they do. Press 'q' to quit")
			fmt.Println(string(horizontal_line))
			display_public_commands()
			if role > 1 {
				display_moderator_commands()
			}

			if role > 2 {
				display_admin_commands()
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
}

func display_public_commands() {
	fmt.Println("Public commands:")
	fmt.Print("\n")
	fmt.Println(" - /help\t\t\t\tBrings up the help screen which lists all commands")
	fmt.Print("\n")
	fmt.Println(" - /main\t\t\t\tDisconnects you from the current channel and takes you to the main menu")
	fmt.Print("\n")
	fmt.Println(" - /log-out\t\t\t\tLogs out of the current account and takes you to the sign in screen")
	fmt.Print("\n")
	fmt.Println(" - /list-c\t\t\t\tLists all users in the current channel")
	fmt.Print("\n")
	fmt.Println(" - /list-s\t\t\t\tLists all users on the server")
}

func display_moderator_commands() {
	fmt.Print("\n")
	fmt.Println("Moderator commands:")
	fmt.Print("\n")
	fmt.Println(" - /disconnect-c <channel> <username>\tDisconnects a user from a specific channel")
	fmt.Print("\n")
	fmt.Println(" - /disconnect-s <username>\t\tDisconnects a user from the server")
	fmt.Print("\n")
	fmt.Println(" - /ban-c <channel> <username>\t\tBans a user from a specific channel")
	fmt.Print("\n")
	fmt.Println(" - /ban-s <username>\t\t\tBans a user from the server")
	fmt.Print("\n")
	fmt.Println(" - /create <channel name> <topic>\tCreates a new channel with a given name and topic")
	fmt.Print("\n")
	fmt.Println(" - /delete <channel>\t\t\tDeletes a specific channel")
	fmt.Print("\n")
	fmt.Println(" - /change-topic <channel> <topic>\tchanges the topic of a specific channel")
}

func display_admin_commands() {
	fmt.Print("\n")
	fmt.Println("Moderator commands:")
	fmt.Print("\n")
	fmt.Println(" - /add-mod <username>\t\t\tGives a user the role moderator")
	fmt.Print("\n")
	fmt.Println(" - /rm-mod <username>\t\t\tRemoves the moderator role from a user")
}

// --------------------------------------------------------------------------------------------------------------

/*
 * This function sends a packet to the command socket
 */
func send_command_packet(packet command_packet)() {
	json_data := marshal_command_packet(packet)
	write_to_connection(json_data, command_socket)
}

/*
 * This function reads a packet from the command socket
 */
func read_command_packet()(command_packet) {
	json_data, amount_read := read_from_connection(command_socket)
	packet := unmarshal_command_packet(json_data[:amount_read])
	return packet
}

/*
 * This function sends a packet to the data socket
 */
func send_data_packet(packet packet) {
	json_data := marshal_data_packet(packet)
	write_to_connection(json_data, data_socket) 
}

/*
 * This function reads a packet from the data socket
 */
func read_data_packet() packet {
	json_data, amount_read := read_from_connection(data_socket)
	packet := unmarshal_data_packet(json_data[:amount_read])
	return packet
}

/*
 * This function writes to a specified socket
 */
func write_to_connection(data []byte, connection net.Conn) {
	_, err := connection.Write(data)
	if err != nil {
		error_exit(err)
	}
}

/*
 * This function reads from a specified socket
 */
func read_from_connection(connection net.Conn) ([]byte, int) {
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
func marshal_data_packet(packet packet) []byte {
	json_data, err := json.Marshal(packet)
	if err != nil {
		fmt.Println("server: Error marshaling data-", err.Error())
	}
	return json_data
}

/*
 * This function unmarshals json data into a packetand handles the possible errors
 */
func unmarshal_data_packet(json_data []byte) packet {
	var packet packet
	err := json.Unmarshal(json_data, &packet)
	if err != nil {
		error_exit(err)
	}
	return packet
}

/*
 * This function marshals a packet into a json file
 */
func marshal_command_packet(packet command_packet) []byte {
	json_data, err := json.Marshal(packet)
	if err != nil {
		fmt.Println("server: Error marshaling data-", err.Error())
	}
	return json_data
}

/*
 * This function unmarshals json data into a packetand handles the possible errors
 */
func unmarshal_command_packet(json_data []byte) command_packet {
	var packet command_packet
	err := json.Unmarshal(json_data, &packet)
	if err != nil {
		error_exit(err)
	}
	return packet
}

/*
 * This function checks if a string is a command
 */
func is_comand(input string) bool {
	tokens := strings.Split(input, " ")
	return command_to_int[tokens[0]] > 0
}

/*
 * This function parses commands
 */
func parse_command(input string) command_packet{
	var packet command_packet
	var args strings.Builder

	// splitting input string into tokens
	tokens := strings.Split(input, " ")

	// looping over tokens to build agument string
	for i := 1; i < len(tokens); i++ {
		if i != 1 {
			args.WriteString(":")
		}

		args.WriteString(tokens[i])
	}

	// initializing packet
	packet.Type = command_to_int[tokens[0]]
	packet.Username = username
	packet.Arguments = []byte(args.String())

	return packet
}