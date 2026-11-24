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
			version = "1.4.1"
		}
		fmt.Println("port-detective version " + version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "check" {
		if file := os.Getenv("SDK_TEST_ARGUMENTS"); file != "" {
			encoded, _ := json.Marshal(os.Args[1:])
			os.WriteFile(file, encoded, 0600)
		}
		if delay, _ := strconv.Atoi(os.Getenv("SDK_TEST_COMMAND_DELAY_MS")); delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		fmt.Println(`{"port":3000,"is_open":true,"protocol":"tcp","status":"occupied","bind_addresses":["127.0.0.1"]}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "wait" {
		if file := os.Getenv("SDK_TEST_ARGUMENTS"); file != "" {
			encoded, _ := json.Marshal(os.Args[1:])
			os.WriteFile(file, encoded, 0600)
		}
		fmt.Println(`{"port":3000,"is_open":true,"protocol":"tcp","status":"occupied"}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "find-free" {
		if file := os.Getenv("SDK_TEST_ARGUMENTS"); file != "" {
			encoded, _ := json.Marshal(os.Args[1:])
			os.WriteFile(file, encoded, 0600)
		}
		fmt.Println(`{"port":3005,"protocol":"tcp","ipv4":true,"ipv6":false,"reserved":false,"message":"available"}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		if os.Getenv("SDK_TEST_DOCTOR_DEGRADED") != "" {
			fmt.Println(`{"version":"1.4.1","os":"linux","arch":"amd64","binary_path":"/bin/pd","cwd":"/tmp","status":"degraded","metadata_accessible":false,"hints":["restricted"]}`)
			os.Exit(5)
		}
		fmt.Println(`{"healthy":true,"version":"1.4.1","os":"linux","arch":"amd64","binary_path":"/bin/pd","cwd":"/tmp","status":"ok","metadata_accessible":true,"hints":[]}`)
		return
	}
	if file := os.Getenv("SDK_TEST_ARGUMENTS"); file != "" {
		encoded, _ := json.Marshal(os.Args[1:])
		os.WriteFile(file, encoded, 0600)
	}
	fmt.Println("[]")
}
