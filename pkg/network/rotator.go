package network

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// IPRotator manages a pool of local network interfaces / IPs to rotate outbound egress traffic.
type IPRotator struct {
	ips []net.IP
	idx uint64
}

var globalRotator *IPRotator

// InitRotator initializes the global IP rotator.
func InitRotator() *IPRotator {
	var parsedIPs []net.IP

	// 1. From environment variable ROTATING_IPS (comma-separated list)
	if envList := os.Getenv("ROTATING_IPS"); envList != "" {
		for _, part := range strings.Split(envList, ",") {
			part = strings.TrimSpace(part)
			if ip := net.ParseIP(part); ip != nil {
				parsedIPs = append(parsedIPs, ip)
			}
		}
	}

	// 2. Auto-discovery from local network interfaces if not explicitly set
	if len(parsedIPs) == 0 {
		if ifaces, err := net.Interfaces(); err == nil {
			for _, iface := range ifaces {
				if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
					continue
				}
				addrs, err := iface.Addrs()
				if err != nil {
					continue
				}
				for _, addr := range addrs {
					var ip net.IP
					switch v := addr.(type) {
					case *net.IPNet:
						ip = v.IP
					case *net.IPAddr:
						ip = v.IP
					}
					if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
						parsedIPs = append(parsedIPs, ip)
					}
				}
			}
		}
	}

	if len(parsedIPs) > 1 {
		log.Printf("[ip-rotator] 🌐 Pool de rotação ativado com %d IPs de egress: %v", len(parsedIPs), parsedIPs)
	} else if len(parsedIPs) == 1 {
		log.Printf("[ip-rotator] IP local único detectado: %v", parsedIPs[0])
	} else {
		log.Printf("[ip-rotator] Nenhum IP secundário configurado, usando rotação padrão do SO.")
	}

	globalRotator = &IPRotator{
		ips: parsedIPs,
	}
	return globalRotator
}

func (r *IPRotator) NextIP() net.IP {
	if r == nil || len(r.ips) == 0 {
		return nil
	}
	if len(r.ips) == 1 {
		return r.ips[0]
	}
	i := atomic.AddUint64(&r.idx, 1) % uint64(len(r.ips))
	return r.ips[i]
}

// NewRotatingHTTPClient returns an http.Client whose dialer binds each connection
// to the next IP in the rotation pool.
func NewRotatingHTTPClient(timeout time.Duration) *http.Client {
	if globalRotator == nil {
		InitRotator()
	}

	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := *dialer
			if nextIP := globalRotator.NextIP(); nextIP != nil {
				d.LocalAddr = &net.TCPAddr{IP: nextIP}
				log.Printf("[ip-rotator] 🔄 Requisição para %s saindo pelo IP local %s", addr, nextIP)
			}
			return d.DialContext(ctx, network, addr)
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     10 * time.Second,
		DisableKeepAlives:   true, // Força nova conexão TCP para rotacionar o IP a cada chamada
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// TestRotation runs N requests to an external IP echo service and returns the list of observed public IPs.
func TestRotation(attempts int) []string {
	client := NewRotatingHTTPClient(10 * time.Second)
	var results []string

	for i := 1; i <= attempts; i++ {
		resp, err := client.Get("https://api.ipify.org")
		if err != nil {
			results = append(results, fmt.Sprintf("Tentativa %d: Erro: %v", i, err))
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		results = append(results, fmt.Sprintf("Tentativa %d: Public IP = %s", i, strings.TrimSpace(string(body))))
	}
	return results
}
