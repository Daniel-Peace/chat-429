package main

// imported packages
import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
	"golang.org/x/crypto/ssh/terminal"
)

// constants
const (
	// client and server information
	// SERVER_HOST = "localhost"
	SERVER_HOST     = "localhost"
	CLIENT_HOST     = "localhost"
	COMMAND_PORT    = "7777"
	DATA_PORT       = "7778"
	CONNECTION_TYPE = "tcp"

	// other
	MAX_PACKET_SIZE = 1024
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

// maps commands to integers
var command_to_int = map[string]int{
	"/help":         1,
	"/exit":         2,
	"/main":         3,
	"/log_out":      4,
	"/list-c":       5,
	"/list-s":       6,
	"/disconnect-c": 7,
	"/disconnect-s": 8,
	"/ban-c":        9,
	"/ban-s":        10,
	"/create":       11,
	"/delete":       12,
	"/change-topic": 13,
	"/add-mod":      14,
	"/rm-mod":       15,
}

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
	REFRESH             // 10 used to refresh certain screens
)

// roles for the client
const (
	PUBLIC    = iota // 0
	MODERATOR        // 1
	ADMIN            // 2
)

// custom errors
const (
	OUT_OF_SYNC     = iota // 0
	UNEXPECTED_DATA        // 1
	UNKNOWN                // 2
)

// ---------------------------------------------------------------------------------------------------

// struct to hold packet information
type Data_packet struct {
	Type     int
	Username string
	Data     []byte
}

type Command_packet struct {
	Type       int
	Username   string
	Arguments  []byte
	Successful bool
	Message    []byte
}

var (
	client_status   int
	terminal_width  int
	terminal_height int
	username        string
	horizontal_line []byte
	vertical_space  []byte
	current_channel []byte
	command_socket  net.Conn
	data_socket     net.Conn

	channels []string

	chat_strand []Data_packet
	mutex_chat  sync.Mutex
)

// --------------------------------------------------------------------------------------------------------

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
		case IN_MAIN_MENU:
			main_menu()
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
 * This function clears the terminal
 */
func clear_terminal() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
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
 * This function creates a second connection with the server for data
 */
func establish_data_connection() {
	// creating a tempary passive socket to listen for the server connecting
	tmp_passive_socket := create_socket()

	// creating connection packet to be sent
	packet := Command_packet{Type: CONNECT, Username: username, Arguments: []byte(CLIENT_HOST + ":" + DATA_PORT)}

	// sending connection packet
	send_command_packet(packet)

	// waiting for the server to accept
	data_socket = accept_connection(tmp_passive_socket)

	// closing the passive socket
	tmp_passive_socket.Close()
}

/*
 * This function creates a signal catcher for
 * when the user enter ctrl-c
 */
func setup_signal_handler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGINT)
	go handle_ctrl_c(sigChan)
}

/*
 * This function prints the client status after successfully launching
 */
func print_client_status() {
	fmt.Println(string(horizontal_line))
	fmt.Println("system: Command socket connected on:")
	fmt.Println("\t- address:\t ", SERVER_HOST)
	fmt.Println("\t- port:\t\t ", COMMAND_PORT)
	fmt.Println("system: Data socket connected on:")
	fmt.Println("\t- address:\t ", SERVER_HOST)
	fmt.Println("\t- port:\t\t ", COMMAND_PORT)
	fmt.Println(string(horizontal_line))
	time.Sleep(2 * time.Second)
}

/*
 * This function prints the splash screen for the program
 */
func print_splash_screen() {
	clear_terminal()
	loading_bar := make([]byte, terminal_width)
	for i := 0; i < terminal_width; i++ {
		fmt.Print(string(vertical_space[0 : (terminal_height-15)/2]))
		banner_line_1 := "__        __   _                         "
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_1))/2)+len(banner_line_1), banner_line_1)
		banner_line_2 := "\\ \\      / /__| | ___ ___  _ __ ___   ___"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_2))/2)+len(banner_line_2), banner_line_2)
		banner_line_3 := "  \\ \\ /\\ / / _ \\ |/ __/ _ \\| '_ ` _ \\ / _ \\"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_3))/2)+len(banner_line_3), banner_line_3)
		banner_line_4 := "   \\ V  V /  __/ | (_| (_) | | | | | |  __/"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_4))/2)+len(banner_line_4), banner_line_4)
		banner_line_5 := "    \\_/\\_/ \\___|_|\\___\\___/|_| |_| |_|\\___|"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_5))/2)+len(banner_line_5), banner_line_5)
		banner_line_6 := "_____  "
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_6))/2)+len(banner_line_6), banner_line_6)
		banner_line_7 := "|_   _|__"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_7))/2)+len(banner_line_7), banner_line_7)
		banner_line_8 := "   | |/ _ \\"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_8))/2)+len(banner_line_8), banner_line_8)
		banner_line_9 := "    | | (_) |"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_9))/2)+len(banner_line_9), banner_line_9)
		banner_line_10 := "   |_|\\___/"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_10))/2)+len(banner_line_10), banner_line_10)
		banner_line_11 := "  ____ _   _    _  _____ _  _  ____   ___"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_11))/2)+len(banner_line_11), banner_line_11)
		banner_line_12 := "  / ___| | | |  / \\|_   _| || ||___ \\ / _ \\"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_12))/2)+len(banner_line_12), banner_line_12)
		banner_line_13 := "  | |   | |_| | / _ \\ | | | || |_ __) | (_) |"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_13))/2)+len(banner_line_13), banner_line_13)
		banner_line_14 := "  | |___|  _  |/ ___ \\| | |__   _/ __/ \\__, |"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_14))/2)+len(banner_line_14), banner_line_14)
		banner_line_15 := "  \\____|_| |_/_/   \\_\\_|    |_||_____|  /_/"
		fmt.Printf("%*s\n", ((terminal_width-len(banner_line_15))/2)+len(banner_line_15), banner_line_15)
		fmt.Print(string(vertical_space[0 : (terminal_height-(terminal_height-15)/2)-15-3]))
		fmt.Println(string(horizontal_line))
		fmt.Println(string(loading_bar))
		fmt.Println(string(horizontal_line))

		// increasing loading bar by one each iteration
		loading_bar = append(loading_bar, '=')
		time.Sleep(20 * time.Millisecond)
	}
}

/*
 * This function creates a listening socket and handles the possible errors
 */
func create_socket() net.Listener {
	listener, err := net.Listen(CONNECTION_TYPE, CLIENT_HOST+":"+DATA_PORT)
	if err != nil {
		error_exit(err)
	}
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
	return connection
}

/*
 * This function sends a packet to the command socket
 */
func send_command_packet(packet Command_packet) {
	json_data := marshal_command_packet(packet)
	write_to_connection(json_data, command_socket)
}

/*
 * This function reads a packet from the command socket
 */
func read_command_packet() Command_packet {
	json_data, amount_read := read_from_connection(command_socket)
	packet := unmarshal_command_packet(json_data[:amount_read])
	return packet
}

/*
 * This function sends a packet to the data socket
 */
func send_data_packet(packet Data_packet) {
	json_data := marshal_data_packet(packet)
	write_to_connection(json_data, data_socket)
}

/*
 * This function reads a packet from the data socket
 */
func read_data_packet() Data_packet {
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
		fmt.Println("system: Failed to write to socket")
	}
}

/*
 * This function reads from a specified socket
 */
func read_from_connection(connection net.Conn) ([]byte, int) {
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
func marshal_data_packet(packet Data_packet) []byte {
	json_data, err := json.Marshal(packet)
	if err != nil {
		fmt.Println("server: Error marshaling data-", err.Error())
	}
	return json_data
}

/*
 * This function unmarshals json data into a packetand handles the possible errors
 */
func unmarshal_data_packet(json_data []byte) Data_packet {
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
 * this function closes the sockets and exits the program
 */
func shutdown() {
	fmt.Println("system: Shutting down...")
	command_socket.Close()
	data_socket.Close()
	os.Exit(0)
}

/*
 * This function handles the client choosing to either sign in or register
 */
func choose_sign_in_opt() {
	var packet Data_packet
	current_choice := 0

	// creating channel to send client choice
	choice_channel := make(chan int)

	// creating routine to handle printing the sign in menu
	go display_sign_in_menu(choice_channel)

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
				packet.Type = MENU_OPTION
				packet.Data = []byte("LOGIN")
				client_status = LOGGING_IN
				send_data_packet(packet)
			} else if current_choice == 1 {
				packet.Type = MENU_OPTION
				packet.Data = []byte("REGISTER")
				client_status = REGISTERING
				send_data_packet(packet)
			} else if current_choice == 2 {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			}
			break
		}
	}
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
					display_opt_0_selected()
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
					display_opt_1_selected()
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
					display_opt_2_selected()
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
func display_opt_0_selected() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print(string(vertical_space[4 : (terminal_height)/2-4]))
	line_3 := "--> LOGIN <--"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "CREATE ACCOUNT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "QUIT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
	fmt.Print(string(vertical_space[(terminal_height)/2+3:]))
}

/*
 * This function prints a screen with the register option selected
 */
func display_opt_1_selected() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print(string(vertical_space[4 : (terminal_height)/2-4]))
	line_3 := "LOGIN"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "--> CREATE ACCOUNT <--"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "QUIT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
	fmt.Print(string(vertical_space[(terminal_height)/2+3:]))
}

/*
 * This function prints a screen with the quit option selected
 */
func display_opt_2_selected() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print(string(vertical_space[4 : (terminal_height)/2-4]))
	line_3 := "LOGIN"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "CREATE ACCOUNT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "--> QUIT <--"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
	fmt.Print(string(vertical_space[(terminal_height)/2+3:]))
}

/*
 * This function displays all options of the sign in menu with none selected
 */
func display_opt() {
	clear_terminal()
	fmt.Println(string(horizontal_line))
	line_1 := "Select an option below"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print(string(vertical_space[4 : (terminal_height)/2-4]))
	line_3 := "LOGIN"
	fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	fmt.Print("\n")
	line_4 := "CREATE ACCOUNT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
	fmt.Print("\n")
	line_5 := "QUIT"
	fmt.Printf("%*s\n", ((terminal_width-len(line_5))/2)+len(line_5), line_5)
	fmt.Print(string(vertical_space[(terminal_height)/2+3:]))
}

/*
 * This function is responcible for registering the client with the server.
 * This includes getting a username and password to create an account
 */
func register_user() {
	if !get_username_for_registration() {
		return
	}

	if !get_password_for_registration() {
		return
	}
}

/*
 * This function loops until it gets a valid username from the user
 */
func get_username_for_registration() bool {
	var packet Data_packet
	var input []byte

	// clearing terminal
	clear_terminal()

	print_registration_username(string(input), nil)

	for {
		if err := keyboard.Open(); err != nil {
			panic(err)
		}

		// getting user input
		for {
			// getting key press
			char, key, err := keyboard.GetSingleKey()
			if err != nil {
				panic(err)
			}

			// checking if enter key was pressed
			if key == keyboard.KeyEnter {
				break
			} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
			} else if key == keyboard.KeyCtrlC {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			} else if key == keyboard.KeySpace {
				input = append(input, ' ')
			} else if key == keyboard.KeyTab || key == keyboard.KeyArrowLeft || key == keyboard.KeyArrowRight || key == keyboard.KeyArrowDown || key == keyboard.KeyArrowUp {
				continue
			} else if key == keyboard.KeyEsc {
				client_status = CHOOSING_SIGN_IN_OPT
				username = ""
				var dpack Data_packet
				dpack.Type = ESC
				dpack.Data = []byte("User hit ESC")
				send_data_packet(dpack)
				return false
			} else {
				input = append(input, byte(char))
			}
			// clearing the terminal
			clear_terminal()

			// printing login screen
			print_registration_username(string(input), nil)
		}

		// checking if the user entered a command
		if is_comand(string(input)) {
			msg := handle_command(string(input))
			input = nil
			clear_terminal()
			print_registration_username(string(input), msg)
			continue
		}

		// declaring and initializing packet
		packet = Data_packet{Type: REGISTRATION, Data: []byte(input)}

		// sending packet containging username
		send_data_packet(packet)

		// waiting for response
		packet = read_data_packet()

		// checking response from server
		// checking if username was accepted
		if packet.Type == ACCEPT {
			break
		} else {
			go play_sound("error.mp3")
			print_registration_username(string(input), packet.Data)
		}
	}
	return true
}

/*
 * This function loops until it gets a valid password from the user
 */
func get_password_for_registration() bool {
	var packet Data_packet
	var input []byte

	// clearing terminal
	clear_terminal()

	print_registration_password(string(input), nil)

	for {
		if err := keyboard.Open(); err != nil {
			panic(err)
		}

		// getting user input
		for {
			// getting key press
			char, key, err := keyboard.GetSingleKey()
			if err != nil {
				panic(err)
			}

			// checking if enter key was pressed
			if key == keyboard.KeyEnter {
				break
			} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
			} else if key == keyboard.KeyCtrlC {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			} else if key == keyboard.KeySpace {
				input = append(input, ' ')
			} else if key == keyboard.KeyTab || key == keyboard.KeyArrowLeft || key == keyboard.KeyArrowRight || key == keyboard.KeyArrowDown || key == keyboard.KeyArrowUp {
				continue
			} else if key == keyboard.KeyEsc {
				client_status = CHOOSING_SIGN_IN_OPT
				username = ""
				var dpack Data_packet
				dpack.Type = ESC
				dpack.Data = []byte("User hit ESC")
				send_data_packet(dpack)
				return false
			} else {
				input = append(input, byte(char))
			}
			// clearing the terminal
			clear_terminal()

			// printing login screen
			print_registration_password(string(input), nil)
		}

		// checking if the user entered a command
		if is_comand(string(input)) {
			msg := handle_command(string(input))
			input = nil
			clear_terminal()
			print_registration_password(string(input), msg)
			continue
		}

		// declaring and initializing packet
		packet = Data_packet{Type: REGISTRATION, Data: []byte(input)}

		// sending packet containging username
		send_data_packet(packet)

		// waiting for response
		packet = read_data_packet()

		// checking response from server
		// checking if username was accepted
		if packet.Type == ACCEPT {
			client_status = IN_MAIN_MENU
			break
		} else {
			go play_sound("error.mp3")
			print_registration_password(string(input), packet.Data)
		}
	}

	return true
}

/*
 * prints login screen with username prompt
 */
func print_registration_username(input string, error []byte) {
	var bar []byte
	var padding []byte

	for i := 0; i < terminal_width; i++ {
		bar = append(horizontal_line, '-')
	}

	for i := 0; i < terminal_width; i++ {
		padding = append(padding, ' ')
	}

	if error == nil {
		// printing prompt
		fmt.Println(string(horizontal_line))
		fmt.Println("Please enter a username. (NOTE: This will be visible to all other users)")
		fmt.Println("Username requirements:")
		fmt.Println("- Must start with: \"A-Z\" or \"a-z\"")
		fmt.Println("- Must end with: \"A-Z\", \"a-z\", or \"0-9\"")
		fmt.Println("- May contain: \"A-Z\", \"a-z\", \"0-9\", \"-\", \"_\"")
		fmt.Println("- Must be 5-20 characters long")
		fmt.Println(string(horizontal_line))

		// printing top space
		fmt.Print(string(vertical_space[:terminal_height/2-10]))

		// printing text box
		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-1]))
		}
	} else {
		fmt.Println(string(horizontal_line))
		fmt.Println("Please enter a username. (NOTE: This will be visible to all other users)")
		fmt.Println("Username requirements:")
		fmt.Println("- Must start with: \"A-Z\" or \"a-z\"")
		fmt.Println("- Must end with: \"A-Z\", \"a-z\", or \"0-9\"")
		fmt.Println("- May contain: \"A-Z\", \"a-z\", \"0-9\", \"-\", \"_\"")
		fmt.Println("- Must be 5-20 characters long")
		fmt.Println(string(horizontal_line))
		fmt.Print(string(vertical_space[:terminal_height/2-10]))
		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		line_4 := "         " + RED + string(error) + RESET
		fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-3]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		}
	}
}

/*
 * prints login screen with password prompt
 */
func print_registration_password(input string, error []byte) {
	var bar []byte
	var padding []byte

	for i := 0; i < terminal_width; i++ {
		bar = append(horizontal_line, '-')
	}

	for i := 0; i < terminal_width; i++ {
		padding = append(padding, ' ')
	}

	if error == nil {
		// prompting user
		fmt.Println(string(horizontal_line))
		fmt.Println("Please enter a passowrd")
		fmt.Println("Password requirements:")
		fmt.Println("- Must contain at least one captial letter")
		fmt.Println("- Must contain at least one number")
		fmt.Println("- Must contain at least one special character (!, @, #, $, %, ?)")
		fmt.Println("- Must be at least 7 characters long")
		fmt.Println(string(horizontal_line))
		fmt.Print(string(vertical_space[:terminal_height/2-10]))

		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-1]))
		}
	} else {
		fmt.Println(string(horizontal_line))
		fmt.Println("Please enter a passowrd")
		fmt.Println("Password requirements:")
		fmt.Println("- Must contain at least one captial letter")
		fmt.Println("- Must contain at least one number")
		fmt.Println("- Must contain at least one special character (!, @, #, $, %, ?)")
		fmt.Println("- Must be at least 7 characters long")
		fmt.Println(string(horizontal_line))
		fmt.Print(string(vertical_space[:terminal_height/2-10]))

		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		line_4 := "         " + RED + string(error) + RESET
		fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-3]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		}
	}
}

/*
 * This function handles logging in the client
 */
func login() {
	var packet Data_packet
	// creating byte array to hold input
	var input []byte

	// clearing terminal
	clear_terminal()

	// prompting user
	print_login_username(string(input), nil)

	// looping until user enters a valid username or exits
	for {
		if err := keyboard.Open(); err != nil {
			panic(err)
		}

		// getting user input
		for {
			// getting key press
			char, key, err := keyboard.GetSingleKey()
			if err != nil {
				panic(err)
			}

			// checking if enter key was pressed
			if key == keyboard.KeyEnter {
				break
			} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
			} else if key == keyboard.KeyCtrlC {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			} else if key == keyboard.KeySpace {
				input = append(input, ' ')
			} else if key == keyboard.KeyTab || key == keyboard.KeyArrowLeft || key == keyboard.KeyArrowRight || key == keyboard.KeyArrowDown || key == keyboard.KeyArrowUp {
				continue
			} else if key == keyboard.KeyEsc {
				client_status = CHOOSING_SIGN_IN_OPT
				username = ""
				var dpack Data_packet
				dpack.Type = ESC
				dpack.Data = []byte("User hit ESC")
				send_data_packet(dpack)
				return
			} else {
				input = append(input, byte(char))
			}
			// clearing the terminal
			clear_terminal()

			// printing login screen
			print_login_username(string(input), nil)
		}

		// checking if the user entered a command
		if is_comand(string(input)) {
			msg := handle_command(string(input))
			input = nil
			clear_terminal()
			print_login_username(string(input), msg)
			continue
		}

		// setting this clients username
		username = string(input)

		// initializing packet
		packet.Type = LOGIN
		packet.Data = []byte(username)
		send_data_packet(packet)

		// reading packet from server
		packet = read_data_packet()

		// checking if username was accepted
		if packet.Type == ACCEPT {
			break
		} else {
			go play_sound("error.mp3")
			print_login_username(string(input), packet.Data)
		}
	}

	// clearing terminal
	clear_terminal()

	// prompting user
	fmt.Println("Enter your password below:")
	fmt.Println(string(horizontal_line))

	// resetting input
	input = nil

	var password_mask []byte

	// clearing terminal
	clear_terminal()

	// printing prompt
	print_login_password(string(password_mask), nil)

	// looping until user enters valid password or exits
	for {
		if err := keyboard.Open(); err != nil {
			panic(err)
		}

		// getting user input
		for {
			// getting key press
			char, key, err := keyboard.GetSingleKey()
			if err != nil {
				panic(err)
			}

			// checking if enter key was pressed
			if key == keyboard.KeyEnter {
				break
			} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
				if len(input) > 0 {
					input = input[:len(input)-1]
					password_mask = password_mask[:len(password_mask)-1]
				}
			} else if key == keyboard.KeyCtrlC {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			} else if key == keyboard.KeyTab || key == keyboard.KeyArrowLeft || key == keyboard.KeyArrowRight || key == keyboard.KeyArrowDown || key == keyboard.KeyArrowUp {
				continue
			} else if key == keyboard.KeySpace {
				input = append(input, ' ')
				password_mask = append(password_mask, '*')
			} else if key == keyboard.KeyEsc {
				client_status = CHOOSING_SIGN_IN_OPT
				username = ""
				var dpack Data_packet
				dpack.Type = ESC
				dpack.Data = []byte("User hit ESC")
				send_data_packet(dpack)
				return
			} else {
				input = append(input, byte(char))
				password_mask = append(password_mask, '*')
			}

			// clearing terminal
			clear_terminal()

			// print login screen
			print_login_password(string(password_mask), nil)
		}

		// checking if command was entered
		if is_comand(string(input)) {
			msg := handle_command(string(input))
			input = nil
			clear_terminal()
			print_login_username(string(input), msg)
			continue
		}

		// initializing packet
		packet.Type = LOGIN
		packet.Data = input
		send_data_packet(packet)

		// reading packet from server
		fmt.Println("reading")
		packet = read_data_packet()

		// determining if password was accepted
		if packet.Type == ACCEPT {
			fmt.Print(string(vertical_space[:terminal_height/2-1]))
			msg := "         " + GREEN + string(packet.Data) + RESET
			fmt.Printf("%*s\n", ((terminal_width-len(msg))/2)+len(msg), msg)
			fmt.Print(string(vertical_space[:terminal_height/2]))
			client_status = IN_MAIN_MENU
			break
		} else {
			go play_sound("error.mp3")
			print_login_password(string(password_mask), packet.Data)
		}
	}

	// sleeping to give user time to see message
	time.Sleep(2 * time.Second)
}

/*
 * prints login screen with username prompt
 */
func print_login_username(input string, error []byte) {
	var bar []byte
	var padding []byte

	for i := 0; i < terminal_width; i++ {
		bar = append(horizontal_line, '-')
	}

	for i := 0; i < terminal_width; i++ {
		padding = append(padding, ' ')
	}

	if error == nil {
		// printing space above text box
		fmt.Print(string(vertical_space[:terminal_height/2-2]))

		// printing prompt
		line_1 := "Enter your username below:"
		fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)

		// printing text box
		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-1]))
		}
	} else {
		fmt.Print(string(vertical_space[:terminal_height/2-2]))
		line_1 := "Enter your username below:"
		fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		line_4 := "         " + RED + string(error) + RESET
		fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)

		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-3]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		}
	}
}

/*
 * prints login screen with password prompt
 */
func print_login_password(input string, error []byte) {
	var bar []byte
	var padding []byte

	for i := 0; i < terminal_width; i++ {
		bar = append(horizontal_line, '-')
	}

	for i := 0; i < terminal_width; i++ {
		padding = append(padding, ' ')
	}

	if error == nil {
		fmt.Print(string(vertical_space[:terminal_height/2-2]))
		line_1 := "Enter your password below:"
		fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-1]))
		}
	} else {
		// printing top space
		fmt.Print(string(vertical_space[:terminal_height/2-2]))

		// printing prompt
		line_1 := "Enter your password below:"
		fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)

		// printing text box
		if len(input) > 20 {
			line_1 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			line_2 := "| " + input + " |"
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:len(input)+4])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		} else {
			line_1 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
			var line_2 string
			if len(input)%2 == 0 {
				line_2 = "|" + string(padding[:(22-len(input))/2]) + input + string(padding[:(22-len(input))/2]) + "|"
			} else {
				line_2 = "|" + string(padding[:(22-len(input))/2+1]) + input + string(padding[:(22-len(input))/2]) + "|"
			}
			fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
			line_3 := string(bar[:24])
			fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
		}
		line_4 := "         " + RED + string(error) + RESET
		fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)

		// printing bottom space
		if terminal_height%2 == 0 {
			fmt.Print(string(vertical_space[:terminal_height/2-3]))
		} else {
			fmt.Print(string(vertical_space[:terminal_height/2-2]))
		}
	}
}

/*
 * This function is responcible for handling messaging
 */
func message() {
	// var input string
	var input []byte
	var err_msg []byte
	var packet Data_packet

	// starting a go routine to handle inbound messages
	go handle_inbound_msg(&input)

	// scanning user inputs and sending messages
	if err := keyboard.Open(); err != nil {
		panic(err)
	}

	for {

		input = nil
		// clearing the terminal
		clear_terminal()

		// printing login screen
		print_chat_strand(input, err_msg)

		// getting user input
		for {
			// getting key press
			char, key, err := keyboard.GetSingleKey()
			if err != nil {
				panic(err)
			}

			// checking if enter key was pressed
			if key == keyboard.KeyEnter {
				err_msg = nil
				break
			} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
				if len(input) > 0 {
					input = input[:len(input)-1]
				}
			} else if key == keyboard.KeyCtrlC {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			} else if key == keyboard.KeyTab || key == keyboard.KeyArrowLeft || key == keyboard.KeyArrowRight || key == keyboard.KeyArrowDown || key == keyboard.KeyArrowUp {
				continue
			} else if key == keyboard.KeyEsc {

				var cpack Command_packet
				cpack.Type = MAIN
				cpack.Username = username
				main_command(cpack)
				return
			} else if key == keyboard.KeySpace {
				input = append(input, ' ')
			} else {
				input = append(input, byte(char))
			}

			// clearing the terminal
			clear_terminal()

			// printing login screen
			print_chat_strand(input, err_msg)
		}

		if input == nil {
			continue
		}

		// checks if a command was entered and executes it if it was
		if is_comand(string(input)) {
			err_msg = handle_command(string(input))
			if client_status != MESSAGING {
				fmt.Println("client state changed")
				return
			}
			clear_terminal()
			print_chat_strand(input, err_msg)
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
			print_chat_strand(input, err_msg)
		}
	}
}

/*
 * This function is responcible for handling inbound messages
 */
func handle_inbound_msg(input *[]byte) {

	// reading inbound messages
	for {

		// reading packet
		packet := read_data_packet()

		if packet.Type == CLOSE {
			return
		}

		if packet.Type == REFRESH {
			current_channel = packet.Data
			// reprinting updated chat strand
			print_chat_strand(*input, nil)
		}

		// checking packet type
		if packet.Type == MESSAGE || packet.Type == JOIN_MSG || packet.Type == LEAVE_MSG {
			// appending new message to chat strand
			mutex_chat.Lock()
			chat_strand = append(chat_strand, packet)
			mutex_chat.Unlock()

			if client_status == MESSAGING {

				if packet.Type == JOIN_MSG {
					go play_sound("joining.mp3")
				} else if packet.Type == LEAVE_MSG {
					go play_sound("leaving.mp3")
				} else {
					go play_sound("receive.mp3")
				}

				// reprinting updated chat strand
				print_chat_strand(*input, nil)
			}
		}
	}
}

/*
 * This function handles when the user presses ctrl-c
 */
func handle_ctrl_c(sigChan chan os.Signal) {
	// Wait for a SIGINT signal
	<-sigChan

	// informing client that the signal was recieved
	fmt.Println("\nExiting CHAT 429")

	var cpack Command_packet
	cpack.Type = EXIT
	cpack.Username = ""
	cpack.Arguments = []byte("client disconnecting")
	client_status = QUITTING
	exit_command(cpack)
}

/*
 * This function gets passed a type of error and closes the
 * client while informing the server that the client is disconnecting
 */
func error_exit(err error) {
	clear_terminal()
	fmt.Println("system: ERROR -", err)
	var cpack Command_packet
	cpack.Type = EXIT
	cpack.Username = ""
	cpack.Arguments = []byte("Error, disconnecting")
	client_status = QUITTING
	exit_command(cpack)
}

/*
 * This function formats and prints the chat strand
 */
func print_chat_strand(input []byte, err_msg []byte) {
	var padding []byte

	for i := 0; i < terminal_width; i++ {
		padding = append(padding, ' ')
	}

	// clearing terminal
	clear_terminal()

	// forcing text box to bottom of screen
	fmt.Print(string(vertical_space))
	fmt.Println(string(horizontal_line))
	start_of_chat_banner := "This is the beginning of the #" + string(current_channel) + " group chat"
	fmt.Printf("%*s\n", ((terminal_width-len(start_of_chat_banner))/2)+len(start_of_chat_banner), start_of_chat_banner)
	fmt.Print("\n\n")

	// looping over the chat strand to print all messages
	for _, packet := range chat_strand {
		// checking if the chat strand is empty
		if chat_strand != nil {

			if packet.Type == JOIN_MSG || packet.Type == LEAVE_MSG {
				status_message := YELLOW + string(packet.Data) + RESET
				fmt.Println(status_message)
				continue
			}

			// checking if its a message the client sent
			if packet.Username == username {
				// creating header for message
				username := GREEN + "You" + RESET + ": "

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
				username_header := GREEN + "\n" + packet.Username + RESET + ": "

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

	if err_msg != nil {
		fmt.Print("\n\n")
		msg := YELLOW + "         " + string(err_msg) + RESET
		fmt.Printf("%*s\n", ((terminal_width-len(msg))/2)+len(msg), msg)
	} else {
		fmt.Print("\n\n\n")
	}

	fmt.Println(string(horizontal_line))
	arrow := GREEN + "-> " + RESET + string(input)
	fmt.Print(arrow)
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

	var cpack Command_packet
	cpack.Type = EXIT
	cpack.Username = ""
	cpack.Arguments = []byte("Error, disconnecting")
	client_status = QUITTING
	exit_command(cpack)
}

/*
 * This function handles main menu functionality
 */
func main_menu() {
	// clearing terminal
	clear_terminal()

	// informting server that the client is ready
	data_packet := Data_packet{Type: MAIN, Username: username, Data: []byte("READY")}
	send_data_packet(data_packet)

	// getting list of channels from the server
	data_packet = read_data_packet()

	// varifying packet
	if data_packet.Type != MAIN {
		custom_error_exit(OUT_OF_SYNC)
	}

	// parsing the choice into an array of strings
	channels = strings.Split(string(data_packet.Data), " ")

	// appending QUIT option to menu
	channels = append(channels, "QUIT")

	// creating channel to send client choice
	choice_channel := make(chan int)

	// creating go routine to handle displaying the menu
	go display_main_menu(choice_channel, channels)

	// creating a keyboard reader
	if err := keyboard.Open(); err != nil {
		panic(err)
	}
	defer keyboard.Close()

	// holds the users current choice
	current_choice := 0

	// looping until a user makes a selection
	for {
		var input []byte
		var error []byte
		// getting key press
		char, key, err := keyboard.GetSingleKey()
		if err != nil {
			panic(err)
		}

		// Check if the pressed key is the up or down arrow
		if key == keyboard.KeyArrowUp {
			current_choice += len(channels) - 1
			current_choice = current_choice % len(channels)
			choice_channel <- current_choice
		} else if key == keyboard.KeyArrowDown {
			current_choice++
			current_choice = current_choice % len(channels)
			choice_channel <- current_choice
		} else if char == '/' {
			choice_channel <- -1
			input = append(input, byte(char))

			clear_terminal()
			display_main_menu_with_command(channels, input, error)

			for {
				// getting key press
				char, key, err := keyboard.GetSingleKey()
				if err != nil {
					panic(err)
				}

				error = nil

				// checking if enter key was pressed
				if key == keyboard.KeyEnter {
					if is_comand(string(input)) {
						error = handle_command(string(input))
						input = nil
					} else {
						error = []byte("Not a valid command")
					}
				} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
					if len(input) > 0 {
						input = input[:len(input)-1]
					} else {
						// creating go routine to handle displaying the menu
						go display_main_menu(choice_channel, channels)
						break
					}
				} else if key == keyboard.KeyCtrlC {
					var cpack Command_packet
					cpack.Type = EXIT
					cpack.Username = ""
					cpack.Arguments = []byte("client disconnecting")
					client_status = QUITTING
					exit_command(cpack)
				} else if key == keyboard.KeySpace {
					input = append(input, ' ')
				} else if key == keyboard.KeyTab || key == keyboard.KeyArrowLeft || key == keyboard.KeyArrowRight {
					continue
				} else if key == keyboard.KeyEsc {
					// creating go routine to handle displaying the menu
					go display_main_menu(choice_channel, channels)
					break
				} else if key == keyboard.KeyArrowUp || key == keyboard.KeyArrowDown {
					// creating go routine to handle displaying the menu
					go display_main_menu(choice_channel, channels)
					break
				} else {
					input = append(input, byte(char))
				}

				clear_terminal()
				display_main_menu_with_command(channels, input, error)
			}
		}

		// Break the loop if Enter key is pressed
		if key == keyboard.KeyEnter {
			// send signal to display_sign_in_menu
			choice_channel <- -1
			if current_choice != len(channels)-1 {
				var packet Data_packet
				packet.Type = MENU_OPTION
				packet.Data = []byte(strconv.Itoa(current_choice))
				current_channel = []byte(channels[current_choice])
				client_status = MESSAGING
				send_data_packet(packet)
			} else {
				var cpack Command_packet
				cpack.Type = EXIT
				cpack.Username = ""
				cpack.Arguments = []byte("client disconnecting")
				client_status = QUITTING
				exit_command(cpack)
			}
			break
		}
	}
}

/*
 * This function is called as a go routine to display the sign in options
 */
func display_main_menu(choice chan int, channels []string) {
	currently_selected := 0
	loop_iteration := 0
	for {
		select {
		// getting users input from channel
		case c := <-choice:
			currently_selected = c

			// checking if user has pressed enter
			if currently_selected == -1 {
				return
			}

			// alternating between printing a selected option and no selected options
			if loop_iteration == 16 {
				loop_iteration = 0
			} else if loop_iteration < 8 {
				print_main_menu_with_choice(currently_selected, channels)
				loop_iteration++
			} else if loop_iteration < 16 {
				print_main_menu_without_choice(channels)
				loop_iteration++
			}
			time.Sleep(30 * time.Millisecond)
		default:
			// checking if user has pressed enter
			if currently_selected == -1 {
				return
			}

			// alternating between printing a selected option and no selected options
			if loop_iteration == 16 {
				loop_iteration = 0
			} else if loop_iteration < 8 {
				print_main_menu_with_choice(currently_selected, channels)
				loop_iteration++
			} else if loop_iteration < 16 {
				print_main_menu_without_choice(channels)
				loop_iteration++
			}
			time.Sleep(30 * time.Millisecond)
		}
	}
}

func display_main_menu_with_command(channels []string, input []byte, err []byte) {
	var bar []byte
	var padding []byte

	for i := 0; i < terminal_width; i++ {
		bar = append(horizontal_line, '-')
	}

	for i := 0; i < terminal_width; i++ {
		padding = append(padding, ' ')
	}

	print_main_menu_without_choice(channels)
	if len(input) > 20 {
		line_1 := string(bar[:len(input)+4])
		fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
		line_2 := "| " + string(input) + " |"
		fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
		line_3 := string(bar[:len(input)+4])
		fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	} else {
		line_1 := string(bar[:24])
		fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
		var line_2 string
		if len(input)%2 == 0 {
			line_2 = "|" + string(padding[:(22-len(input))/2]) + string(input) + string(padding[:(22-len(input))/2]) + "|"
		} else {
			line_2 = "|" + string(padding[:(22-len(input))/2+1]) + string(input) + string(padding[:(22-len(input))/2]) + "|"
		}
		fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
		line_3 := string(bar[:24])
		fmt.Printf("%*s\n", ((terminal_width-len(line_3))/2)+len(line_3), line_3)
	}

	line_4 := YELLOW + "         " + string(err) + RESET
	fmt.Printf("%*s\n", ((terminal_width-len(line_4))/2)+len(line_4), line_4)
}

/*
 * This function prints a screen with the login option selected
 */
func print_main_menu_with_choice(choice int, channels []string) {
	// clearing terminal
	clear_terminal()

	// printing prompt
	fmt.Println(string(horizontal_line))
	line_1 := "Select a channel from the list below to join"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print("\n\n\n\n\n\n")

	// printing options
	for i := 0; i < len(channels); i++ {
		var msg string
		if i == choice {
			if i != len(channels)-1 {
				msg = "--> #" + channels[i] + " <--"
			} else {
				msg = "--> " + channels[i] + " <--"
			}

		} else if i != len(channels)-1 {
			msg = "#" + channels[i]
		} else {
			msg = channels[i]
		}
		fmt.Printf("%*s\n", ((terminal_width-len(msg))/2)+len(msg), msg)
		fmt.Print("\n")
	}
}

/*
 * This function prints a screen with the login option selected
 */
func print_main_menu_without_choice(channels []string) {
	// clearing terminal
	clear_terminal()

	// printing prompt
	fmt.Println(string(horizontal_line))
	line_1 := "Select a channel from the list below to join"
	fmt.Printf("%*s\n", ((terminal_width-len(line_1))/2)+len(line_1), line_1)
	line_2 := "Use the up and down arrows to change selection"
	fmt.Printf("%*s\n", ((terminal_width-len(line_2))/2)+len(line_2), line_2)
	fmt.Println(string(horizontal_line))
	fmt.Print("\n\n\n\n\n\n")

	// printing prompt
	for i := 0; i < len(channels); i++ {
		var msg string
		if i != len(channels)-1 {
			msg = "#" + channels[i]
		} else {
			msg = channels[i]
		}
		fmt.Printf("%*s\n", ((terminal_width-len(msg))/2)+len(msg), msg)
		fmt.Print("\n")
	}
}

// -----------------------------------------------------------------------------------------------------------------------
// COMMANDS
// -----------------------------------------------------------------------------------------------------------------------

/*
 * This function checks if a string is a command
 */
func is_comand(input string) bool {
	tokens := strings.Split(input, " ")
	return command_to_int[tokens[0]] > 0
}

/*
 * This function executes the command entered
 */
func handle_command(input string) []byte {
	// sending the command to the server
	packet := parse_command(input)

	switch packet.Type {
	case HELP:
		return help_command(packet)
	case EXIT:
		return exit_command(packet)
	case CREATE:
		return create_command(packet)
	case MAIN:
		return main_command(packet)
	case CHANGE_TOPIC:
		return change_topic_command(packet)
	case ADD_MOD:
		return add_mod_command(packet)
	case RM_MOD:
		return rm_mod_command(packet)
	case BAN_S:
		return ban_s_command(packet)
	default:
		return nil
	}
}

/*
 * This function parses commands
 */
func parse_command(input string) Command_packet {
	var packet Command_packet
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

/*
 * This function handles the client exiting the application
 */
func exit_command(cpack Command_packet) []byte {
	// send command to server
	send_command_packet(cpack)

	// checking if the server changed states
	cpack = read_command_packet()
	if cpack.Type != EXIT || string(cpack.Arguments) != "READY" {
		custom_error_exit(UNKNOWN)
	}

	// closing function on server side that is using data_socket
	dpack := Data_packet{Type: CLOSE, Username: "", Data: []byte("Client disconnecting")}
	send_data_packet(dpack)

	// creating exit packet
	cpack.Type = EXIT
	cpack.Username = username
	cpack.Arguments = []byte("CLOSE_SENT")

	// sending packet to server
	send_command_packet(cpack)

	// shutting down client
	shutdown()
	return nil
}

/*
 * This function handles the client accessing the help menu
 */
func help_command(cpack Command_packet) []byte {
	// send command to server
	send_command_packet(cpack)

	packet := read_command_packet()
	if string(packet.Arguments) != "OK" {
		return packet.Arguments
	}

	packet.Type = HELP
	packet.Username = username
	packet.Arguments = []byte("READY")
	send_command_packet(packet)

	packet = read_command_packet()

	quit_channel := make(chan int)

	if string(packet.Arguments) == "0" {
		go display_help_screen(quit_channel, PUBLIC)
	} else if string(packet.Arguments) == "1" {
		go display_help_screen(quit_channel, MODERATOR)
	} else if string(packet.Arguments) == "2" {
		go display_help_screen(quit_channel, ADMIN)
	} else {
		custom_error_exit(UNEXPECTED_DATA)
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
	return nil
}

/*
 * This function handles the main command
 */
func main_command(cpack Command_packet) []byte {
	if client_status == CHOOSING_SIGN_IN_OPT || client_status == REGISTERING || client_status == LOGGING_IN {
		return []byte("Command not availbale. Must sign in first.")
	} else if client_status == IN_MAIN_MENU {
		return []byte("You are already in the main menu")
	}

	// send server the command
	send_command_packet(cpack)

	// getting response from server
	cpack = read_command_packet()
	if cpack.Type != MAIN {
		custom_error_exit(OUT_OF_SYNC)
	}

	// checking if command was successful
	if string(cpack.Arguments) != "Success" {
		return cpack.Arguments
	} else {
		fmt.Println("updating status")
		client_status = IN_MAIN_MENU
		dpack := Data_packet{Type: CLOSE, Username: username, Data: []byte("State changed")}
		send_data_packet(dpack)
		chat_strand = nil
	}

	return cpack.Arguments
}

/*
 * This function handles the create command
 */
func create_command(cpack Command_packet) []byte {

	if client_status == CHOOSING_SIGN_IN_OPT || client_status == REGISTERING || client_status == LOGGING_IN {
		return []byte("Command not availbale. Must sign in first.")
	}
	// sending command
	send_command_packet(cpack)

	// getting repsonse from server
	cpack = read_command_packet()
	if cpack.Type != CREATE {
		custom_error_exit(OUT_OF_SYNC)
	}

	if cpack.Successful {
		// parsing the choice into an array of strings
		channels = strings.Split(string(cpack.Arguments), " ")

		// appending QUIT option to menu
		channels = append(channels, "QUIT")
	}

	return cpack.Message
}

/*
 * This function handles the change_topic command
 */
func change_topic_command(cpack Command_packet) []byte {
	if client_status == CHOOSING_SIGN_IN_OPT || client_status == REGISTERING || client_status == LOGGING_IN {
		return []byte("Command not availbale. Must sign in first.")
	}

	// sending command to server
	send_command_packet(cpack)

	// getting response from server
	cpack = read_command_packet()
	if cpack.Type != CHANGE_TOPIC {
		custom_error_exit(OUT_OF_SYNC)
	}

	return cpack.Arguments
}

/*
 * This function handles giving the moderator role from a user
 */
func add_mod_command(cpack Command_packet) []byte {
	// checking if user is signed in
	if client_status == CHOOSING_SIGN_IN_OPT || client_status == REGISTERING || client_status == LOGGING_IN {
		return []byte("Command not availbale. Must sign in first.")
	}

	// send command to server
	send_command_packet(cpack)

	// getting response from server
	cpack = read_command_packet()
	if cpack.Type != ADD_MOD {
		custom_error_exit(OUT_OF_SYNC)
	}

	return cpack.Arguments
}

/*
 * This function handles removing the moderator role from a user
 */
func rm_mod_command(cpack Command_packet) []byte {
	// checking if user is signed in
	if client_status == CHOOSING_SIGN_IN_OPT || client_status == REGISTERING || client_status == LOGGING_IN {
		return []byte("Command not availbale. Must sign in first.")
	}

	// send command to server
	send_command_packet(cpack)

	// getting response from server
	cpack = read_command_packet()
	if cpack.Type != ADD_MOD {
		custom_error_exit(OUT_OF_SYNC)
	}

	return cpack.Arguments
}

/*
 * This function hanels the ban-s command
 */
func ban_s_command(cpack Command_packet) []byte {
	// checking if user is signed in
	if client_status == CHOOSING_SIGN_IN_OPT || client_status == REGISTERING || client_status == LOGGING_IN {
		return []byte("Command not availbale. Must sign in first.")
	}

	// send command to server
	send_command_packet(cpack)

	// getting response from server
	cpack = read_command_packet()
	if cpack.Type != BAN_S {
		custom_error_exit(OUT_OF_SYNC)
	}

	return cpack.Arguments
}

/*
 * This function handles the displaying the help screen
 */
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
				if role > 0 {
					display_moderator_commands()
				}

				if role > 1 {
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
			if role > 0 {
				display_moderator_commands()
			}

			if role > 1 {
				display_admin_commands()
			}
		}
		time.Sleep(30 * time.Millisecond)
	}
}

/*
 * This function prints the commands for the public role
 */
func display_public_commands() {
	fmt.Println("Public commands:")
	fmt.Print("\n")
	fmt.Println(" - /help\t\t\t\tBrings up the help screen which lists all commands")
	fmt.Print("\n")
	fmt.Println(" - /main\t\t\t\tDisconnects you from the current channel and takes you to the main menu")
	fmt.Print("\n")
}

/*
 * This function prints the commands for the moderator role
 */
func display_moderator_commands() {
	fmt.Print("\n")
	fmt.Println("Moderator commands:")
	fmt.Print("\n")
	fmt.Println(" - /ban-s <username>\t\t\tBans a user from the server")
	fmt.Print("\n")
	fmt.Println(" - /create <topic>\t\t\tCreates a new channel with a given topic")
	fmt.Print("\n")
	fmt.Println(" - /change-topic <channel> <topic>\tchanges the topic of a specific channel")
}

/*
 * This function prints the commands for the admin role
 */
func display_admin_commands() {
	fmt.Print("\n")
	fmt.Println("Moderator commands:")
	fmt.Print("\n")
	fmt.Println(" - /add-mod <username>\t\t\tGives a user the role moderator")
	fmt.Print("\n")
	fmt.Println(" - /rm-mod <username>\t\t\tRemoves the moderator role from a user")
}

func play_sound(file_path string) {
	f, err := os.Open(file_path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Decode the MP3 data
	streamer, format, err := mp3.Decode(f)
	if err != nil {
		panic(err)
	}

	// Initialize the speaker with the format of the audio
	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/8))

	// Play the audio stream
	speaker.Play(streamer)

	time.Sleep(2 * time.Second)
}
