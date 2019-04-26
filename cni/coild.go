package cni

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
)

func getIPFromCoild(coild *url.URL, podNS, podName, containerId string) (ip net.IP, err error) {
	u := *coild
	u.Path = path.Join(u.Path, "/ip")
	var data struct {
		PodNS       string `json:"pod-namespace"`
		PodName     string `json:"pod-name"`
		AddressType string `json:"address-type"`
		ContainerID string `json:"container-id"`
	}
	data.PodNS = podNS
	data.PodName = podName
	data.AddressType = "ipv4"
	data.ContainerID = containerId

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

func returnIPToCoild(coild *url.URL, podNS, podName, containerId string) error {
	u := *coild
	u.Path = path.Join(u.Path, "/ip", podNS, podName, containerId)

	req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("coild returns %d", resp.StatusCode)
	}

	return nil
}
