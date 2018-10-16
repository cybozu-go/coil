package cni

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
)

func getIPFromCoild(coild *url.URL, podNS, podName string) (net.IP, error) {
	u := *coild
	u.Path = "/ip"
	var data struct {
		PodNS       string `json:"pod-namespace"`
		PodName     string `json:"pod-name"`
		AddressType string `json:"address-type"`
	}
	data.PodNS = podNS
	data.PodName = podName
	data.AddressType = "ipv4"

	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(u.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("coild returns %d", resp.StatusCode)
	}

	var result struct {
		Address string `json:"address"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return net.ParseIP(result.Address), nil
}

func returnIPToCoild(coild *url.URL, podNS, podName string) error {
	return nil
}
