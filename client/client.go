// socket-client project main.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
	"syscall"
	"os/signal"
	"sync"
)
const (
	SERVER_HOST = "localhost"
	SERVER_PORT = "7777"
	SERVER_TYPE = "tcp"
    MAX_PACKET_SIZE  = 1024
)

const (
	REGISTERING	= iota 	// 0
	MESSAGING        	// 1
)

// packet types
const (
	ACCEPT	= iota	// 0
	DENY			// 1
	MESSAGE 		// 2
	REGISTRATION	// 3
	QUIT			// 4
)

// struct to hold packet information
type packet struct {
	Type				int
	Sender_username 	string
	Sender_id			int
	Data				[]byte
}

var status int
var username string
var(
	chat_strand	[]packet
	mutex_chat	sync.Mutex
) 

func main() {
	// establish connection
	connection, err := net.Dial(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
			panic(err)
	}
	fmt.Println("-----------------------------------------------------------------------")
	fmt.Println("system: Successfully connected to server")
	fmt.Println("\t- address:\t ", SERVER_HOST)
	fmt.Println("\t- port:\t\t ", SERVER_PORT)

	// Create a channel to receive signals
    sigChan := make(chan os.Signal, 1)

    // Notify the sigChan whenever a SIGINT signal is received
    signal.Notify(sigChan, os.Interrupt, syscall.SIGINT)

    // Start a goroutine to handle the SIGINT signal asynchronously
    go handleSigInt(sigChan, connection)

	status = REGISTERING

	for {
		switch(status){
		case REGISTERING:
			// registering the client
			if register_user(connection){
				status = MESSAGING
			}
		case MESSAGING:
			time.Sleep(1 * time.Second)
			message(connection)
			fmt.Println("Exiting CHAT 429")
			connection.Close()
			return
		}			
	}
}

func register_user(connection net.Conn)(is_registered bool) {
	// creating scanner
	scanner := bufio.NewScanner(os.Stdin)

	for {
		// prompting user
		fmt.Println("-----------------------------------------------------------------------")
		fmt.Println("Please enter a username. (NOTE: This will be visible to all other users)")
		fmt.Println("Username requirements:")
		fmt.Println("- Must start with: \"A-Z\" or \"a-z\"")
		fmt.Println("- Must end with: \"A-Z\", \"a-z\", or \"0-9\"")
		fmt.Println("- Mmay contain: \"A-Z\", \"a-z\", \"0-9\", \"-\", \"_\"")
		fmt.Println("- Must be 5-20 characters long")
		fmt.Println("-----------------------------------------------------------------------")
		fmt.Print("-> ")

	    // getting input from user
		if scanner.Scan() {

			// storing scanned text in variable
			username = scanner.Text()			
		}

		// declaring registration packet
		var packet packet

		// initializing packet
		packet.Type = REGISTRATION
		packet.Data = []byte(username)

		// marshaling data
		jsonData, err := json.Marshal(packet)
		if err != nil {
			fmt.Println("system: Error marshaling data-", err.Error())
		}

		// sending json data
		_, err = connection.Write(jsonData)
		if err != nil {
			fmt.Println("system: Error writing to server: ", err.Error())
		}

		// reading packet from server
		json_packet := make([]byte, MAX_PACKET_SIZE)
		bytes_read, err := connection.Read(json_packet)
	    if err != nil {
	            fmt.Println("Error reading:", err.Error())
	    }

		err = json.Unmarshal(json_packet[:bytes_read], &packet)
		if err != nil {
			fmt.Println("system: Error unmarshaling JSON:", err)
		}

		switch(packet.Type) {
		case DENY:
			fmt.Printf("system: %s\n", string(packet.Data))

		case ACCEPT:
			fmt.Printf("system: %s\n", string(packet.Data))
			return true
		}
	}
}

func message(connection net.Conn)(){

	go handle_inbound_msg(connection)
	scanner := bufio.NewScanner(os.Stdin)
	var msg string
	var packet packet 

	print_chat_strand()

	for {
		if scanner.Scan() {
			

			// storing scanned text in variable
			msg = scanner.Text()			
		}
		fmt.Println("-----------------------------------------------------------------------")

		if msg == "/exit" {
			packet.Type = QUIT
			
			json_packet, err := json.Marshal(packet)
			if err != nil {
				fmt.Println("system: Error marshaling data-", err.Error())
			}

			_, err = connection.Write(json_packet)
			if err != nil {
				fmt.Println("system: Error writing to server: ", err.Error())
			}

			return
		}

		
		packet.Type = MESSAGE
		packet.Data = []byte(msg)
		packet.Sender_username = "You"

		chat_strand = append(chat_strand, packet)

		packet.Sender_username = username

		jsonData, err := json.Marshal(packet)
		if err != nil {
			fmt.Println("system: Error marshaling data-", err.Error())
		}

		_, err = connection.Write(jsonData)
		if err != nil {
			fmt.Println("system: Error writing to server: ", err.Error())
		}

		print_chat_strand()
	}
}

func handle_inbound_msg(connection net.Conn)(){
	var packet packet
	for {
		// reading packet from server
		json_packet := make([]byte, MAX_PACKET_SIZE)
		bytes_read, err := connection.Read(json_packet)
		if err != nil {
				fmt.Println("Error reading:", err.Error())
		}

		// Remove null characters from the JSON packet
		cleaned_packet := make([]byte, 0, len(json_packet))
		for _, b := range json_packet[:bytes_read] {
			if b != 0 {
				cleaned_packet = append(cleaned_packet, b)
			}
		}

		err = json.Unmarshal(cleaned_packet, &packet)
		if err != nil {
			fmt.Println("system: Error unmarshaling JSON:", err)
		}

		if packet.Type == MESSAGE {
			if status == MESSAGING {

				mutex_chat.Lock()
				chat_strand = append(chat_strand, packet)
				mutex_chat.Unlock()

				print_chat_strand()
			}
		}
	}
}

func handleSigInt(sigChan chan os.Signal, connection net.Conn) {
    // Wait for a SIGINT signal
    <-sigChan

    // Handle the SIGINT signal
	fmt.Println("\nExiting CHAT 429")

	var packet packet
	packet.Type = QUIT
			
	json_packet, err := json.Marshal(packet)
	if err != nil {
		fmt.Println("system: Error marshaling data-", err.Error())
	}

	_, err = connection.Write(json_packet)
	if err != nil {
		fmt.Println("system: Error writing to server: ", err.Error())
	}

	connection.Close()
    os.Exit(0)
}

func print_chat_strand(){

	// Clear the screen
	cmd := exec.Command("clear") // For Unix-like systems
	// cmd := exec.Command("cmd", "/c", "cls") // For Windows
	cmd.Stdout = os.Stdout
	cmd.Run()
	fmt.Print("\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n")
	fmt.Println("-----------------------------------------------------------------------")
	fmt.Println("\tThis is the beggining of the COMP 429 group chat")
	fmt.Print("\n\n")

	for _, packet := range chat_strand  {
		if chat_strand != nil {
			if packet.Sender_username == "You" {
				username := packet.Sender_username + ": "
	
				username_buffer := make([]byte, 27)
				for i := range username_buffer {
					username_buffer[i] = ' '
				}
				copy(username_buffer, []byte(username))
				fmt.Print("\n")
				fmt.Printf("%*s\n", 71, string(username_buffer))

				index := 0
				closest_space := 0
				var last_space int
				formatted_string := make([]byte, len(packet.Data) - 1)
				formatted_string = []byte(packet.Data)
				
				fmt.Printf("%*s\n", 71, " __________________________")
				fmt.Printf("%*s\n", 71, "|                          ")
				
				var temp_string string
				output := make([]byte, 27)
				for _, character := range packet.Data {

					if character == ' ' {
						closest_space = index
					}

					if index%24 == 0 && index != 0 {
						for i := range output {
							output[i] = ' '
						}

						if last_space > 0 {
							temp_string = "| " + string(formatted_string[last_space + 1:closest_space])
						} else {
							temp_string = "| " + string(formatted_string[last_space:closest_space])
						}
						copy(output, []byte(temp_string))
						fmt.Printf("%*s\n", 71, string(output))
						last_space = closest_space
					} 
					index++
				}

				for i := range output {
					output[i] = ' '
				}

				if last_space > 0 {
					temp_string = "| " + string(formatted_string[last_space + 1:])
				} else {
					temp_string = "| " + string(formatted_string[last_space:])
				}
				copy(output, []byte(temp_string))
				fmt.Printf("%*s\n", 71, string(output))
				fmt.Printf("%*s\n", 71, "|__________________________")
			} else {
				username := "\n" + packet.Sender_username + ": "
				fmt.Println(username)
				index := 0
				closest_space := 0
				var last_space int
				formatted_string := make([]byte, len(packet.Data) - 1)
				formatted_string = []byte(packet.Data)
				
				fmt.Println("__________________________ ")
				fmt.Println("                          |")
				
				var temp_string string
				output := make([]byte, 27)
				for _, character := range packet.Data {

					if character == ' ' {
						closest_space = index
					}

					if index%24 == 0 && index != 0 {
						for i := range output {
							output[i] = ' '
						}

						if last_space > 0 {
							temp_string = string(formatted_string[last_space + 1:closest_space])
						} else {
							temp_string = string(formatted_string[last_space:closest_space])
						}
						copy(output, []byte(temp_string))
						output[26] = '|'
						fmt.Println(string(output))
						last_space = closest_space
					} 
					index++
				}

				for i := range output {
					output[i] = ' '
				}
				
				if last_space > 0 {
					temp_string = string(formatted_string[last_space + 1:])
				} else {
					temp_string = string(formatted_string[last_space:])
				}
				copy(output, []byte(temp_string))
				output[26] = '|'
				fmt.Println(string(output))
				fmt.Println("__________________________|")
			}
		}
	}

	if chat_strand == nil {
		fmt.Println("\n\n\n-----------------------------------------------------------------------")
		fmt.Print("-> ")
	} else {
		fmt.Println("\n\n\n-----------------------------------------------------------------------")
		fmt.Print("-> ")
	}
}