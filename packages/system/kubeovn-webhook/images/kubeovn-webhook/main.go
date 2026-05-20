package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	PortSecurityGlobal bool
	RoutesGlobal       string
)

// seenClientIssuers records which client-certificate issuers have already been
// logged, so the observation below costs one line per distinct issuer rather
// than one per admitted pod.
var seenClientIssuers sync.Map

// logClientCert reports, once per distinct issuer, whether the caller presented
// a client certificate and who signed it.
//
// This is the evidence needed to decide whether client-certificate enforcement
// can be turned on. The handler serves namespace-derived values to whoever asks
// (see GHSA-g883-q79m-8225), and the fix is to accept only the API server — but
// the API server presents a certificate only when the cluster is configured for
// it (an AdmissionConfiguration with a kubeConfigFile), which is not universal
// across the variants this ships on. Switching straight to
// RequireAndVerifyClientCert on a cluster that sends nothing would fail every
// admission call, and this webhook is registered failurePolicy: Fail — so pod
// creation would stop cluster-wide. Observe first, enforce second.
func logClientCert(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := "<none>"
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			key = r.TLS.PeerCertificates[0].Issuer.String()
		}
		if _, loaded := seenClientIssuers.LoadOrStore(key, struct{}{}); !loaded {
			if key == "<none>" {
				log.Printf("client certificate: none presented — client-ca-file enforcement is NOT yet safe on this cluster")
			} else {
				log.Printf("client certificate: presented, issuer %q — enforcement can be enabled with --client-ca-file for this CA", key)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	var (
		tlsCertFile  string
		tlsKeyFile   string
		clientCAFile string
	)

	flag.StringVar(&tlsCertFile, "tls-cert-file", "/etc/webhook/certs/tls.crt", "TLS certificate file.")
	flag.StringVar(&tlsKeyFile, "tls-key-file", "/etc/webhook/certs/tls.key", "TLS key file.")
	flag.StringVar(&clientCAFile, "client-ca-file", "", "CA bundle for verifying client certificates. When set, callers MUST present a certificate signed by it; when empty, certificates are requested and logged but not required.")
	flag.BoolVar(&PortSecurityGlobal, "port-security", true, "If false, skip adding port_security unless specified by the Namespace.")
	flag.StringVar(&RoutesGlobal, "routes", "", "Default ovn.kubernetes.io/routes if not in Namespace.")

	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate-pods", HandleMutatePods)

	tlsConfig, err := newReloadingTLSConfig(tlsCertFile, tlsKeyFile)
	if err != nil {
		log.Fatalf("Failed to load key pair: %v", err)
	}

	tlsConfig.MinVersion = tls.VersionTLS12

	// Default: ask for a client certificate, accept the connection either way.
	// This never rejects a caller, so it cannot break admission; it only makes
	// the certificate visible to logClientCert.
	tlsConfig.ClientAuth = tls.RequestClientCert

	if clientCAFile != "" {
		clientCAPool := x509.NewCertPool()
		clientCAData, err := os.ReadFile(clientCAFile)
		if err != nil {
			log.Fatalf("Failed to read client CA file: %v", err)
		}
		if !clientCAPool.AppendCertsFromPEM(clientCAData) {
			log.Fatalf("Failed to parse client CA certificate")
		}
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = clientCAPool
		log.Printf("mTLS enabled: requiring client certificates signed by %s", clientCAFile)
	} else {
		log.Printf("mTLS not enforced (--client-ca-file empty): client certificates are requested and logged only")
	}

	server := &http.Server{
		Addr:              ":8443",
		TLSConfig:         tlsConfig,
		Handler:           logClientCert(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Starting webhook server on %s", server.Addr)
	if err := server.ListenAndServeTLS("", ""); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
