// A controllable executable fixture exercises real SDK subprocess lifecycles.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		if name := os.Getenv("SDK_TEST_VERSION_LOG"); name != "" {
			file, _ := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if file != nil {
				fmt.Fprintln(file, os.Getpid())
				file.Close()
			}
		}
		if delay, _ := strconv.Atoi(os.Getenv("SDK_TEST_VERSION_DELAY_MS")); delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		if os.Getenv("SDK_TEST_VERSION_FAIL") != "" {
			fmt.Fprintln(os.Stderr, "version unavailable")
			os.Exit(7)
		}
		version := os.Getenv("SDK_TEST_VERSION")
		if version == "" {
			version = "1.2.4"
		}
		fmt.Println("port-detective version " + version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "check" {
		if delay, _ := strconv.Atoi(os.Getenv("SDK_TEST_COMMAND_DELAY_MS")); delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		fmt.Println(`{"port":3000,"is_open":true,"status":"occupied"}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		fmt.Println(`{"healthy":true}`)
		return
	}
	if file := os.Getenv("SDK_TEST_ARGUMENTS"); file != "" {
		encoded, _ := json.Marshal(os.Args[1:])
		os.WriteFile(file, encoded, 0600)
	}
	fmt.Println("[]")
}
