package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
)

var (
	leaderElectionDuration = "2m"
	leading                = false
)

type leaderElectionResponse struct {
	Name string
}

func isLeader() bool {
	leadershipResponse, err := http.Get("http://localhost:4040")
	if err != nil {
		panic(err.Error())
	}
	defer leadershipResponse.Body.Close()
	contents, err := ioutil.ReadAll(leadershipResponse.Body)
	if err != nil {
		panic(err.Error())
	}
	var leaderElection leaderElectionResponse
	err = json.Unmarshal(contents, &leaderElection)
	if err != nil {
		panic(err.Error())
	}
	ourHostname := hostname()
	return leaderElection.Name == ourHostname
}

func hostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return hostname
}
