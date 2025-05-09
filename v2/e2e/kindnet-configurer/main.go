package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	containerName      = "coil"
	ipv4               = "v4"
	ipv6               = "v6"
	getAction          = "get"
	setAction          = "set"
	defaultConflist    = "10-kindnet.conflist"
	numOfNodes         = 4
	defaultTmpFilename = "networks"
)

func main() {
	action := flag.String("action", getAction, "Action to perform (get/set)")
	container := flag.String("container", containerName, "Base name of the container to use")
	protocol := flag.String("protocol", ipv4, "Version of IP protocol to use")
	cniConfig := flag.String("cni-config", defaultConflist, "CNI config file to edit")
	file := flag.String("file", defaultTmpFilename, "Base name for temporary file")

	flag.Parse()

	if *protocol != ipv4 && *protocol != ipv6 {
		log.Fatalf("invalid protocol [%s]", *protocol)
	}

	containers := []string{}
	for i := 0; i < numOfNodes; i++ {
		c := fmt.Sprintf("%s-worker", *container)
		if i == 0 {
			c = fmt.Sprintf("%s-control-plane", *container)
		}
		if i > 1 {
			c = fmt.Sprintf("%s%d", c, i)
		}
		containers = append(containers, c)
	}

	switch *action {
	case getAction:
		if err := get(*cniConfig, *protocol, *file, containers); err != nil {
			log.Fatal(err)
		}
	case setAction:
		if err := set(*cniConfig, *protocol, *file, containers); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("command [%s] not supported", *action)
	}
}

func get(conflistName, protoVer, tmpFilename string, containers []string) error {
	path := filepath.Join("tmp", fmt.Sprintf("%s-%s", tmpFilename, protoVer))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	address := "10.244."
	if protoVer == ipv6 {
		address = "fd00:10:244:"
	}

	for _, container := range containers {
		var err error
		var output string
		var errOutput string
		for i := 0; i < 120; i++ {
			cmd := exec.Command("docker", "exec", container, "cat", "/etc/cni/net.d/"+conflistName)
			var buffer bytes.Buffer
			cmd.Stdout = &buffer
			var bufferErr bytes.Buffer
			cmd.Stderr = &bufferErr
			if err = cmd.Run(); err != nil {
				errOutput = bufferErr.String()
				fmt.Printf("Error: %s: %s\n", err.Error(), errOutput)
				fmt.Println("Retrying...")
				time.Sleep(time.Second)
			} else {
				output = buffer.String()
				break
			}
		}

		if err != nil {
			return fmt.Errorf("error: %w: %s", err, errOutput)
		}

		start := strings.Index(output, address)
		end := start + 10
		if protoVer == ipv6 {
			end = strings.Index(output, "/64")
		}

		network := output[start:end]
		if _, err := fmt.Fprintln(f, network); err != nil {
			return fmt.Errorf("failed to write temporary file %s: %w", path, err)
		}
	}

	return nil
}

func set(conflistName, protoVer, tmpFilename string, contianers []string) error {
	f, err := os.Open(filepath.Join("tmp", fmt.Sprintf("%s-%s", tmpFilename, protoVer)))
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for _, container := range contianers {
		scanner.Scan()
		network := scanner.Text()
		reg := fmt.Sprintf("s/10\\.244\\.0\\.0/%s/", network)
		if protoVer == ipv6 {
			reg = fmt.Sprintf("s/fd00\\:10\\:244\\:\\:/%s/", network)
		}
		cmd := exec.Command("docker", "exec", container, "sed", "-i", reg, "/etc/cni/net.d/"+conflistName)
		var bufferErr bytes.Buffer
		cmd.Stderr = &bufferErr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error: %w: %s", err, bufferErr.String())
		}
	}
	return nil
}
