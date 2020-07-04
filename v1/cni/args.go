package cni

import (
	"errors"
	"strings"
)

func parseArgs(args string) map[string]string {
	result := make(map[string]string)
	kvs := strings.Split(args, ";")
	for _, kv := range kvs {
		v := strings.SplitN(kv, "=", 2)
		if len(v) != 2 {
			continue
		}
		result[v[0]] = v[1]
	}
	return result
}

func getPodInfo(kv map[string]string) (podNS string, podName string, err error) {
	podNS, ok := kv["K8S_POD_NAMESPACE"]
	if !ok {
		return "", "", errors.New("no K8S_POD_NAMESPACE")
	}

	podName, ok = kv["K8S_POD_NAME"]
	if !ok {
		return "", "", errors.New("no K8S_POD_NAME")
	}

	return podNS, podName, nil
}
