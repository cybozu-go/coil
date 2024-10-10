package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	containerName = "coil-worker"
	ipv4          = "v4"
	ipv6          = "v6"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("too few arguments")
	}

	cmd := os.Args[1]
	conflistName := os.Args[2]
	protoVer := os.Args[3]

	if protoVer != ipv4 && protoVer != ipv6 {
		log.Fatalf("invalid protocol")
	}

	switch cmd {
	case "get":
		if err := get(conflistName, protoVer); err != nil {
			log.Fatal(err)
		}
	case "set":
		if err := set(conflistName, protoVer); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("%s: command not supported", cmd)
	}
}

func get(conflistName string, protoVer string) error {
	f, err := os.Create(filepath.Join("tmp", "networks"))
	if err != nil {
		return err
	}
	defer f.Close()

	address := "10.244."
	if protoVer == ipv6 {
		address = "fd00:10:244:"
	}

	for i := 1; i < 4; i++ {
		container := containerName
		if i > 1 {
			container += strconv.Itoa(i)
		}
		cmd := exec.Command("docker", "exec", container, "cat", "/etc/cni/net.d/"+conflistName)
		var buffer bytes.Buffer
		cmd.Stdout = &buffer
		if err := cmd.Run(); err != nil {
			return err
		}
		output := buffer.String()
		start := strings.Index(output, address)
		end := start + 10
		if protoVer == ipv6 {
			end = strings.Index(output, "/64")
		}

		network := output[start:end]
		if err := os.Setenv(strings.ToUpper(container)+"_NETWORK", network); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(f, network); err != nil {
			return err
		}
	}

	return nil
}

func set(conflistName string, protoVer string) error {
	f, err := os.Open(filepath.Join("tmp", "networks"))
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for i := 1; i < 4; i++ {
		container := containerName
		if i > 1 {
			container += strconv.Itoa(i)
		}
		scanner.Scan()
		network := scanner.Text()
		fmt.Printf("%s: %s\n", strings.ToUpper(container)+"_NETWORK", network)
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
