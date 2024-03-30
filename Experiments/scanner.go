package main

import (
	"os"
	"os/exec"
	"fmt"
	"github.com/eiannone/keyboard"
)

func main() {
	fmt.Println(string(scanner()))
}

/*
 * This function scans the input of a user until they enter the return character
 */
func scanner()([]byte){
	// creating keyboard object
	if err := keyboard.Open(); err != nil {
		panic(err)
	}

	// creating byte array to hold input
	var input []byte

	// looping until the user enter the return key
	for {
		// getting key press
		char, key, err := keyboard.GetSingleKey()
		if err != nil {
			panic(err)
		}

		// checking if enter key was pressed
		if key == keyboard.KeyEnter {
			break
		}

		input = append(input, byte(char))
	}
	return input
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