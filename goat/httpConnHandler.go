package goat

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

// HTTPConnHandler handles incoming HTTP (TCP) network connections
type HTTPConnHandler struct {
}

// Handle incoming HTTP connections and serve
func (h HTTPConnHandler) Handle(l net.Listener, httpDoneChan chan bool) {
	// Create shutdown function
	go func(l net.Listener, httpDoneChan chan bool) {
		// Wait for done signal
		Static.ShutdownChan <- <-Static.ShutdownChan

		// Close listener
		l.Close()
		httpDoneChan <- true
	}(l, httpDoneChan)

	// Log API configuration
	if Static.Config.API {
		log.Println("API functionality enabled")
	}

	// Set up HTTP routes for handling functions
	http.HandleFunc("/", parseHTTP)

	// Serve HTTP requests
	http.Serve(l, nil)
}

// Parse incoming HTTP connections to API, before making API calls
func parseAPI(w http.ResponseWriter, r *http.Request) {
	// Count incoming connections
	atomic.AddInt64(&Static.HTTP.Current, 1)
	atomic.AddInt64(&Static.HTTP.Total, 1)

	// Add header to identify goat and JSON response
	w.Header().Add("Server", fmt.Sprintf("%s/%s", App, Version))
}

// Parse incoming HTTP connections before making tracker calls
func parseHTTP(w http.ResponseWriter, r *http.Request) {
	// Count incoming connections
	atomic.AddInt64(&Static.HTTP.Current, 1)
	atomic.AddInt64(&Static.HTTP.Total, 1)

	// Parse querystring
	querystring := r.URL.Query()

	// Flatten arrays into single values
	query := map[string]string{}
	for k, v := range querystring {
		query[k] = v[0]
	}

	// Check if IP was previously set
	if _, ok := query["ip"]; !ok {
		// If no IP set, detect and store it in query map
		query["ip"] = strings.Split(r.RemoteAddr, ":")[0]
	}

	// Add header to identify goat
	w.Header().Add("Server", fmt.Sprintf("%s/%s", App, Version))

	// Create channel to return response to client
	resChan := make(chan []byte)

	// Store current URL path
	url := r.URL.Path

	// Split URL into segments
	urlArr := strings.Split(url, "/")

	// If configured, Detect if client is making an API call
	url = urlArr[1]
	if Static.Config.API && url == "api" {
		// Log API calls
		log.Printf("API: %s\n", r.URL.Path)

		// Handle API calls
		go APIRouter(r, resChan)

		// Wait for response, and send it when ready
		w.Write(<-resChan)
		close(resChan)
		return
	}

	// Detect if passkey present in URL
	var passkey string
	if len(urlArr) == 3 {
		passkey = urlArr[1]
		url = urlArr[2]
	}

	// Verify that torrent client is advertising its User-Agent, so we can use a whitelist
	if _, ok := r.Header["User-Agent"]; !ok {
		w.Write(HTTPTrackerError("Your client is not identifying itself"))
		return
	}

	client := r.Header["User-Agent"][0]

	// If configured, verify that torrent client is on whitelist
	if Static.Config.Whitelist {
		whitelist := new(WhitelistRecord).Load(client, "client")
		if whitelist == (WhitelistRecord{}) || !whitelist.Approved {
			w.Write(HTTPTrackerError("Your client is not whitelisted"))

			// Block things like browsers and web crawlers, because they will just clutter up the table
			if strings.Contains(client, "Mozilla") || strings.Contains(client, "Opera") {
				return
			}

			// Insert unknown clients into list for later approval
			if whitelist == (WhitelistRecord{}) {
				whitelist.Client = client
				whitelist.Approved = false

				log.Printf("whitelist: detected new client '%s', awaiting manual approval", client)

				go whitelist.Save()
			}

			return
		}
	}

	// Put client in query map
	query["client"] = client

	// Check if server is configured for passkey announce
	if Static.Config.Passkey && passkey == "" {
		w.Write(HTTPTrackerError("No passkey found in announce URL"))
		return
	}

	// Validate passkey if needed
	user := new(UserRecord).Load(passkey, "passkey")
	if Static.Config.Passkey && user == (UserRecord{}) {
		w.Write(HTTPTrackerError("Invalid passkey"))
		return
	}

	// Put passkey in query map
	query["passkey"] = user.Passkey

	// Mark client as HTTP
	query["udp"] = "0"

	// Handle tracker functions via different URLs
	switch url {
	// Tracker announce
	case "announce":
		// Validate required parameter input
		required := []string{"info_hash", "ip", "port", "uploaded", "downloaded", "left"}
		// Validate required integer input
		reqInt := []string{"port", "uploaded", "downloaded", "left"}

		// Check for required parameters
		for _, r := range required {
			if _, ok := query[r]; !ok {
				w.Write(HTTPTrackerError("Missing required parameter: " + r))
				close(resChan)
				return
			}
		}

		// Check for all valid integers
		for _, r := range reqInt {
			if _, ok := query[r]; ok {
				_, err := strconv.Atoi(query[r])
				if err != nil {
					w.Write(HTTPTrackerError("Invalid integer parameter: " + r))
					close(resChan)
					return
				}
			}
		}

		// Only allow compact announce
		if _, ok := query["compact"]; !ok || query["compact"] != "1" {
			w.Write(HTTPTrackerError("Your client does not support compact announce"))
			close(resChan)
			return
		}

		// Perform tracker announce
		go TrackerAnnounce(user, query, nil, resChan)
	// Tracker scrape
	case "scrape":
		go TrackerScrape(user, query, resChan)
	// Any undefined handlers
	default:
		w.Write(HTTPTrackerError("Malformed announce"))
		close(resChan)
		return
	}

	// Wait for response, and send it when ready
	w.Write(<-resChan)
	close(resChan)
	return
}
