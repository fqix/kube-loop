package forwardruntime

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fqix/kube-loop/internal/client/socksbridge"
	"github.com/fqix/kube-loop/internal/client/traffic"
	"github.com/fqix/kube-loop/internal/singbox"
	"github.com/fqix/kube-loop/internal/utils"
)

// TestDirectSOCKSReplacement exercises real client and Gateway sing-box processes.
// It bypasses the outer Gateway authorization adapter and uses loopback targets;
// this is a transport/lifecycle probe, not a Kubernetes end-to-end test.
func TestDirectSOCKSReplacement(t *testing.T) {
	binary := os.Getenv("KUBELOOP_SINGBOX_PATH")
	if binary == "" {
		t.Skip("KUBELOOP_SINGBOX_PATH is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	tcpTarget := directProbeTCP(t)
	udpTarget := directProbeUDP(t)
	password, err := trojanPassword("11111111-1111-4111-8111-111111111111", 1)
	if err != nil {
		t.Fatal(err)
	}
	port, err := utils.FreeTCPPort()
	if err != nil {
		t.Fatal(err)
	}
	config, err := singbox.GenerateGatewaySessionConfig(singbox.GatewaySessionOptions{
		SessionID: "11111111-1111-4111-8111-111111111111", ListenPort: port, TrojanPassword: password,
		Network: singbox.NetworkSpec{PodCIDRs: []string{"127.0.0.0/8"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	gatewayAddress := fmt.Sprintf("127.0.0.1:%d", port)
	directProbeGateway(ctx, t, binary, config, gatewayAddress)
	starter := Starter{BinaryPath: binary, ReadyTimeout: 3 * time.Second}
	options := Options{SessionID: "11111111-1111-4111-8111-111111111111", Generation: 1,
		Endpoint: "ws://" + gatewayAddress + singbox.GatewayWebSocketPath, RelayTicket: "local-probe-ticket"}
	first, err := starter.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	direct := traffic.Dialer{Endpoint: traffic.Endpoint{Address: first.Address()}}
	t.Run("native_tcp", func(t *testing.T) { directProbeEcho(ctx, t, direct, "tcp", tcpTarget) })
	t.Run("native_udp", func(t *testing.T) { directProbeEcho(ctx, t, direct, "udp", udpTarget) })

	bridge, err := socksbridge.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })
	bridge.SetForwardDialer(direct)
	bridged := traffic.Dialer{Endpoint: traffic.Endpoint{Address: bridge.Addr().String()}}
	t.Run("host_interception_is_bypassed", func(t *testing.T) {
		bridge.SetHostTCPHandler(func(host string, port uint16) (func(net.Conn), bool) {
			if net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10)) != tcpTarget {
				return nil, false
			}
			return func(conn net.Conn) { defer conn.Close(); _, _ = conn.Write([]byte("HOST")) }, true
		})
		conn, err := bridged.DialContext(ctx, "tcp", tcpTarget)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		marker := make([]byte, 4)
		if _, err := io.ReadFull(conn, marker); err != nil {
			t.Fatal(err)
		}
		if string(marker) != "HOST" {
			t.Fatalf("host marker = %q", marker)
		}
		// The same destination through the native endpoint reaches the echo server.
		directProbeEcho(ctx, t, direct, "tcp", tcpTarget)
		bridge.SetHostTCPHandler(nil)
	})
	t.Run("replacement_changes_native_address_but_bridge_stays", func(t *testing.T) {
		second, err := starter.Start(ctx, options)
		if err != nil {
			t.Fatal(err)
		}
		defer second.Close()
		if first.Address() == second.Address() {
			t.Fatal("concurrent native processes unexpectedly share an address")
		}
		replacement := traffic.Dialer{Endpoint: traffic.Endpoint{Address: second.Address()}}
		bridge.SetForwardDialer(replacement)
		if err := first.Close(); err != nil {
			t.Fatal(err)
		}
		stale, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", first.Address())
		if err == nil {
			_ = stale.Close()
			t.Fatal("old native endpoint still accepts connections")
		}
		for _, dialer := range []traffic.Dialer{replacement, bridged} {
			directProbeEcho(ctx, t, dialer, "tcp", tcpTarget)
			directProbeEcho(ctx, t, dialer, "udp", udpTarget)
		}
		t.Logf("native endpoint changed %s -> %s; bridge retained %s", first.Address(), second.Address(), bridge.Addr())
	})
}

func directProbeEcho(ctx context.Context, t *testing.T, dialer traffic.Dialer, network, target string) {
	t.Helper()
	conn, err := dialer.DialContext(ctx, network, target)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "ping" {
		t.Fatalf("%s response = %q", network, response)
	}
}

func directProbeTCP(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String()
}

func directProbeUDP(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buffer := make([]byte, 65535)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			_, _ = conn.WriteTo(buffer[:n], addr)
		}
	}()
	return conn.LocalAddr().String()
}

func directProbeGateway(ctx context.Context, t *testing.T, binary string, config []byte, address string) {
	t.Helper()
	directory := t.TempDir()
	path := filepath.Join(directory, "gateway.json")
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(directory, "gateway.log"))
	if err != nil {
		t.Fatal(err)
	}
	processCtx, cancel := context.WithCancel(ctx)
	command := exec.CommandContext(processCtx, binary, "run", "-c", path)
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		cancel()
		_ = log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait(); _ = log.Close() })
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{Timeout: 100 * time.Millisecond}).DialContext(ctx, "tcp", address)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(log.Name())
	t.Fatalf("Gateway did not start: %s", data)
}
