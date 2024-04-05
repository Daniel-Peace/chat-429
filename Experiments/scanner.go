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
func scanner_looped()([]byte){
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

		// clearing the terminal
		clear_terminal()

		// checking if enter key was pressed
		if key == keyboard.KeyEnter {
			break
		} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
			if len(input) <= 0 {
				continue
			}
			input = input[:len(input) - 1]
		} else if key == keyboard.KeyCtrlC{
			os.Exit(1)
		} else if key == keyboard.KeySpace {
			input = append(input, ' ')
		} else {
			input = append(input, byte(char))
		}
		fmt.Print(string(input))
	}
	return input
}

/*
 * This function scans the input of a user until they enter the return character
 */
 func scanner()([]byte) {
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

		// clearing the terminal
		clear_terminal()

		// checking if enter key was pressed
		if key == keyboard.KeyEnter {
			break
		} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
			if len(input) <= 0 {
				continue
			}
			input = input[:len(input) - 1]
		} else if key == keyboard.KeyCtrlC{
			os.Exit(1)
		} else if key == keyboard.KeySpace {
			input = append(input, ' ')
		} else {
			input = append(input, byte(char))
		}
		fmt.Print(string(input))
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