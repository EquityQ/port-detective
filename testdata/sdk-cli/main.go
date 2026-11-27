// A controllable executable fixture exercises real SDK subprocess lifecycles.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

func recordArgs() {
	if file := os.Getenv("SDK_TEST_ARGUMENTS"); file != "" {
		encoded, _ := json.Marshal(os.Args[1:])
		_ = os.WriteFile(file, encoded, 0o600)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		if name := os.Getenv("SDK_TEST_VERSION_LOG"); name != "" {
			file, _ := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
		recordArgs()
		if delay, _ := strconv.Atoi(os.Getenv("SDK_TEST_COMMAND_DELAY_MS")); delay > 0 {
			time.Sleep(time.Duration(delay) * time.Millisecond)
		}
		fmt.Println(`{"port":3000,"is_open":true,"protocol":"tcp","status":"occupied","bind_addresses":["127.0.0.1"]}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "wait" {
		recordArgs()
		fmt.Println(`{"port":3000,"is_open":true,"protocol":"tcp","status":"occupied"}`)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "find-free" {
		recordArgs()
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
	if len(os.Args) > 1 && os.Args[1] == "kill" {
		recordArgs()
		proto := "tcp"
		dryRun := false
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "--protocol" && i+1 < len(os.Args) {
				proto = os.Args[i+1]
			}
			if os.Args[i] == "--dry-run" {
				dryRun = true
			}
		}
		fmt.Printf(`{"port":3000,"protocol":%q,"dry_run":%t,"preview_id":"tok.kill","preview_expires_at_unix":1900000000,"post_action_status":"not_attempted","message":"dry run"}`+"\n", proto, dryRun)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "clean" {
		recordArgs()
		envelope := false
		dryRun := false
		for _, arg := range os.Args[2:] {
			if arg == "--envelope" {
				envelope = true
			}
			if arg == "--dry-run" {
				dryRun = true
			}
		}
		if envelope {
			fmt.Printf(`{"cwd":"/tmp/app","scope":"all local TCP listeners","dry_run":%t,"count":1,"cleaned":[{"port":3000,"is_open":true,"protocol":"tcp","pid":123,"status":"occupied","post_action_status":"not_attempted"}],"preview_id":"tok.clean","preview_expires_at_unix":1900000000,"message":"dry run"}`+"\n", dryRun)
			return
		}
		fmt.Println("[]")
		return
	}
	recordArgs()
	fmt.Println("[]")
}
