package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildWebDaemonArgs(t *testing.T) {
	got := buildWebDaemonArgs(
		"data/custom.db",
		"127.0.0.1:8788",
		"http://127.0.0.1:11434",
		"qwen3:latest",
		4*time.Minute,
		true,
	)
	want := []string{
		"web",
		"--addr", "127.0.0.1:8788",
		"--ollama-url", "http://127.0.0.1:11434",
		"--model", "qwen3:latest",
		"--ollama-timeout", "4m0s",
		"--db", "data/custom.db",
		"--restart",
	}
	if !equalStringSlices(got, want) {
		t.Fatalf("buildWebDaemonArgs() = %#v, want %#v", got, want)
	}
}

func TestOpenDaemonLogCreatesParentDirectory(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "logs", "web.log")
	logFile, err := openDaemonLog(logPath)
	if err != nil {
		t.Fatalf("openDaemonLog() error = %v", err)
	}
	if _, err := logFile.WriteString("ready\n"); err != nil {
		t.Fatalf("write daemon log: %v", err)
	}
	if err := logFile.Close(); err != nil {
		t.Fatalf("close daemon log: %v", err)
	}

	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read daemon log: %v", err)
	}
	if string(contents) != "ready\n" {
		t.Fatalf("daemon log contents = %q, want ready newline", contents)
	}
}

func TestTCPListenPort(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "ipv4", addr: "127.0.0.1:8787", want: "8787"},
		{name: "empty host", addr: ":8787", want: "8787"},
		{name: "ipv6", addr: "[::1]:8787", want: "8787"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := tcpListenPort(test.addr)
			if err != nil {
				t.Fatalf("tcpListenPort(%q) error = %v", test.addr, err)
			}
			if got != test.want {
				t.Fatalf("tcpListenPort(%q) = %q, want %q", test.addr, got, test.want)
			}
		})
	}
}

func TestTCPListenPortRejectsInvalidAddress(t *testing.T) {
	if _, err := tcpListenPort("127.0.0.1"); err == nil {
		t.Fatal("tcpListenPort() error = nil, want invalid address error")
	}
}

func TestRestartWebListenerSignalsExistingListener(t *testing.T) {
	originalFind := findListeningPIDsForPort
	originalSignal := signalProcessByPID
	originalAvailable := tcpListenAvailable
	originalSleep := restartSleep
	defer func() {
		findListeningPIDsForPort = originalFind
		signalProcessByPID = originalSignal
		tcpListenAvailable = originalAvailable
		restartSleep = originalSleep
	}()

	findListeningPIDsForPort = func(port string) ([]int, error) {
		if port != "8787" {
			t.Fatalf("findListeningPIDsForPort port = %q, want 8787", port)
		}
		return []int{4242, 4242, 0}, nil
	}

	var signaled []int
	signalProcessByPID = func(pid int) error {
		signaled = append(signaled, pid)
		return nil
	}

	checks := 0
	tcpListenAvailable = func(addr string) (bool, error) {
		if addr != "127.0.0.1:8787" {
			t.Fatalf("tcpListenAvailable addr = %q, want 127.0.0.1:8787", addr)
		}
		checks++
		return checks > 1, nil
	}
	restartSleep = func(time.Duration) {}

	if err := restartWebListener("127.0.0.1:8787", time.Second); err != nil {
		t.Fatalf("restartWebListener() error = %v", err)
	}
	if len(signaled) != 1 || signaled[0] != 4242 {
		t.Fatalf("signaled pids = %#v, want [4242]", signaled)
	}
	if checks != 2 {
		t.Fatalf("availability checks = %d, want 2", checks)
	}
}

func TestRestartWebListenerTimesOutWhenPortStaysBusy(t *testing.T) {
	originalFind := findListeningPIDsForPort
	originalSignal := signalProcessByPID
	originalAvailable := tcpListenAvailable
	originalSleep := restartSleep
	defer func() {
		findListeningPIDsForPort = originalFind
		signalProcessByPID = originalSignal
		tcpListenAvailable = originalAvailable
		restartSleep = originalSleep
	}()

	findListeningPIDsForPort = func(string) ([]int, error) {
		return []int{4242}, nil
	}
	signalProcessByPID = func(int) error {
		return nil
	}
	tcpListenAvailable = func(string) (bool, error) {
		return false, nil
	}
	restartSleep = func(time.Duration) {}

	err := restartWebListener("127.0.0.1:8787", 0)
	if err == nil {
		t.Fatal("restartWebListener() error = nil, want timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("restartWebListener() error = %v, want timeout", err)
	}
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestServeHTTPServerGracefullyShutsDownInFlightRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	requestRelease := make(chan struct{})

	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		<-requestRelease
		_, _ = writer.Write([]byte("completed"))
	})
	server := &http.Server{Handler: handler}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}

	shutdown := make(chan struct{})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveHTTPServer(server, listener, shutdown)
	}()

	requestDone := make(chan struct{})
	var responseBody []byte
	var responseErr error
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			responseErr = err
			close(requestDone)
			return
		}
		defer response.Body.Close()
		responseBody, responseErr = io.ReadAll(response.Body)
		close(requestDone)
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request did not start")
	}

	close(shutdown)
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown returned before in-flight request was released")
	default:
	}
	close(requestRelease)

	select {
	case <-requestDone:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request did not complete")
	}
	if responseErr != nil {
		t.Fatalf("in-flight request error = %v", responseErr)
	}
	if string(responseBody) != "completed" {
		t.Fatalf("response body = %q, want completed", responseBody)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serveHTTPServer() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serveHTTPServer() did not return after shutdown")
	}
}
