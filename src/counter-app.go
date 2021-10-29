package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coreos/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
)

// etcdCli is a client used to interact with Etcd
var etcdCli *clientv3.Client
var podName, nodeName string
var globalCounter uint64
var reg *regexp.Regexp
var version string = "0.1"

var gracePeriod time.Duration = 1 * time.Second

func init() {
	var err error

	podName = os.Getenv("MY_POD_NAME")
	if podName == "" {
		podName = "unknown"
	}

	nodeName = os.Getenv("MY_NODE_NAME")
	if nodeName == "" {
		nodeName = "unknown"
	}

	// Create a etcd client
	etcdCli, err = clientv3.New(getEtcdConfig())
	if err != nil {
		log.Fatal(err)
	}

	//Regex to make sure the key is purely numerical
	reg, err = regexp.Compile("[^0-9]+")
	if err != nil {
		log.Fatal(err)
	}
}

//	setEtcdConfig() reads the env var ETCD_ENDPOINTS
//	and accordingly, sets the Etcd endpoints. It defaults to 0.0.0.0:2379
func getEtcdConfig() clientv3.Config {
	var etcdEndpoints string = os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpoints == "" {
		etcdEndpoints = "http://0.0.0.0:2379"
	} else if strings.Contains(etcdEndpoints, ",") {
		var etcdEndpointsArr []string = strings.Split(etcdEndpoints, ",")
		log.Printf("configuring etcd array with %v\n", etcdEndpointsArr)
		return clientv3.Config{Endpoints: etcdEndpointsArr}
	}
	return clientv3.Config{Endpoints: []string{etcdEndpoints}}
}

func fetchKey() (uint64, error) {
	// use keyvalue api
	kv := clientv3.NewKV(etcdCli)
	// fetch existing key
	gr, err := kv.Get(context.Background(), "key")
	if err != nil || len(gr.Kvs) == 0 {
		return globalCounter, err
	}
	//filtering out non-numeric values
	counter := 0
	if len(gr.Kvs) > 0 {
		processedString := reg.ReplaceAllString(string(gr.Kvs[0].Value), "")
		if processedString != "" {
			counter, err = strconv.Atoi(processedString)
			if err != nil {
				return globalCounter, err
			}
		}
	}
	log.Println("Value: ", string(gr.Kvs[0].Value), "Revision: ", gr.Header.Revision)
	return uint64(counter), nil
}

// UpdateCount() uses a shared
func updateCount(w http.ResponseWriter, r *http.Request) {
	// create a sessions to aqcuire a lock
	s, _ := concurrency.NewSession(etcdCli)
	defer s.Close()
	myLock := concurrency.NewMutex(s, "/distributed-lock/")
	ctx := context.Background()
	log.Println("trying to aquire lock for ", podName)
	if err := myLock.Lock(ctx); err != nil {
		log.Fatal(err)
	}

	log.Println("acquired lock for: ", podName)
	log.Println("incrementing global counter for: ", podName)
	defer myLock.Unlock(ctx)
	if err := myLock.Unlock(ctx); err != nil {
		status := fmt.Sprintf(
			"Warning: cannot aquire lock in Etcd. My pod name is: `%s`, my node name is: `%s`, global counter value is unkown, local count value is: %v\n",
			podName, nodeName, atomic.LoadUint64(&globalCounter))
		log.Println(status)
		http.Error(w, status, http.StatusInternalServerError)
		return
	}
	// fetch the latest key value first
	kv := clientv3.NewKV(etcdCli)
	key, err := fetchKey()
	if err != nil {
		http.Error(w, "Error happened fetching key", http.StatusInternalServerError)
		log.Printf("Error happened fetching key. Err: %s", err)
		return
	}
	// increment the latest value
	atomic.StoreUint64(&globalCounter, key)
	atomic.AddUint64(&globalCounter, 1)

	// Insert a key value
	pr, _ := kv.Put(ctx, "key", strconv.Itoa(int(globalCounter)))
	log.Println("Modified to Value: ", globalCounter, "Revision: ", pr.Header.Revision)
	log.Println("released lock for ", podName)

	w.Header().Set("Content-Type", "application/json")
	resp := make(map[string]string)
	resp["message"] = "Status OK"
	resp["content"] = fmt.Sprintf("count value = %d", globalCounter)
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Error happened in JSON marshal", http.StatusInternalServerError)
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
	}
	w.Write(jsonResp)
}

// readynessCheck() returns a 200 status if the etcd cluster is reachable
func readynessCheck(w http.ResponseWriter, r *http.Request) {
	resp, err := etcdCli.MemberList(context.Background())
	if err != nil {
		http.Error(w, "Error, Etcd is not ready or not accessible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	jsonResp, err := createJsonResponse("etcd members", uint64(len(resp.Members)))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		http.Error(w, "Error happened in JSON marshal", http.StatusInternalServerError)
	}
	w.Write(jsonResp)
	log.Println("Etcd number of members:", len(resp.Members))
}

// getCount() returns the count value from Etcd.
// it does not use a shared lock while reading the value
func getCount(w http.ResponseWriter, r *http.Request) {
	key, err := fetchKey()
	if err != nil {
		http.Error(w, "Error happened fetching key", http.StatusInternalServerError)
		log.Printf("Error happened fetching key. Err: %s", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	jsonResp, err := createJsonResponse("count value", key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		http.Error(w, "Error happened in JSON marshal", http.StatusInternalServerError)
	}
	w.Write(jsonResp)
}

// writes some static content to the http response
func getStaticContent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jsonResp, err := createJsonResponse("meaning of life", 42)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		http.Error(w, "Error happened in JSON marshal", http.StatusInternalServerError)
	}
	w.Write(jsonResp)
}

// createJsonResponse() creates a []byte with a json format to be return the http client
func createJsonResponse(message string, value uint64) ([]byte, error) {
	resp := make(map[string]string)
	resp["pod"] = podName
	resp["node"] = nodeName
	resp["message"] = "Status OK"
	resp["content"] = fmt.Sprintf("%s = %d", message, value)
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Fatalf("Error happened in JSON marshal. Err: %s", err)
		return nil, err
	}
	return jsonResp, nil
}

// cleanup() makes sure the connections are closed
func cleanup() {
	if etcdCli != nil {
		log.Print("closing etcd client\n")
		etcdCli.Close()
	}
}

////////////////////// main() ///////////////////////////
func main() {
	log.Printf("counter running version %s", version)
	// make sure we close the client
	defer cleanup()

	var serverAddr string
	flag.StringVar(&serverAddr, "server-addr", ":8080", "The port the server binds to.")
	flag.Parse()

	// increments the local and global counters every time is called
	http.HandleFunc("/increment", updateCount)
	// returns the current value of the counter
	http.HandleFunc("/fetch", getCount)
	// returns 200 status if Etcd is healthy
	http.HandleFunc("/healthz", readynessCheck)
	// returns some static content
	http.HandleFunc("/static", getStaticContent)

	httpServer := &http.Server{
		Addr: serverAddr,
	}
	// SIGTERM handling logic, to shutdown HTTP server
	// and close open sessions. We could do more
	// by making sure all requests complete before exiting
	terminateChan := make(chan os.Signal)
	signal.Notify(terminateChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-terminateChan
		log.Print("SIGTERM received. Shutting down the process gracefully\n")
		// sleeping gracePeriod to allow in-flight requests to complete
		time.Sleep(gracePeriod)
		httpServer.Shutdown(context.Background())
		cleanup()
		os.Exit(0)
	}()

	log.Printf("starting http server on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, nil); err != nil {
		if err.Error() != "http: Server closed" {
			log.Printf("HTTP server closed with an error: %v\n", err)
			httpServer.Shutdown(context.Background())
		}
		log.Printf("HTTP server shut down")
	}
}
