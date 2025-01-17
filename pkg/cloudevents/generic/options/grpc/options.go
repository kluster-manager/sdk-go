package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v2"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/grpc/protocol"
)

const (
	// SpecTopic is a pubsub topic for resource spec.
	SpecTopic = "sources/+/clusters/+/spec"

	// StatusTopic is a pubsub topic for resource status.
	StatusTopic = "sources/+/clusters/+/status"

	// SpecResyncTopic is a pubsub topic for resource spec resync.
	SpecResyncTopic = "sources/clusters/+/specresync"

	// StatusResyncTopic is a pubsub topic for resource status resync.
	StatusResyncTopic = "sources/+/clusters/statusresync"
)

// GRPCOptions holds the options that are used to build gRPC client.
type GRPCOptions struct {
	URL            string
	CAFile         string
	ClientCertFile string
	ClientKeyFile  string
}

// GRPCConfig holds the information needed to build connect to gRPC server as a given user.
type GRPCConfig struct {
	// URL is the address of the gRPC server (host:port).
	URL string `json:"url" yaml:"url"`
	// CAFile is the file path to a cert file for the gRPC server certificate authority.
	CAFile string `json:"caFile,omitempty" yaml:"caFile,omitempty"`
	// ClientCertFile is the file path to a client cert file for TLS.
	ClientCertFile string `json:"clientCertFile,omitempty" yaml:"clientCertFile,omitempty"`
	// ClientKeyFile is the file path to a client key file for TLS.
	ClientKeyFile string `json:"clientKeyFile,omitempty" yaml:"clientKeyFile,omitempty"`
}

// BuildGRPCOptionsFromFlags builds configs from a config filepath.
func BuildGRPCOptionsFromFlags(configPath string) (*GRPCOptions, error) {
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	config := &GRPCConfig{}
	if err := yaml.Unmarshal(configData, config); err != nil {
		return nil, err
	}

	if config.URL == "" {
		return nil, fmt.Errorf("url is required")
	}

	if (config.ClientCertFile == "" && config.ClientKeyFile != "") ||
		(config.ClientCertFile != "" && config.ClientKeyFile == "") {
		return nil, fmt.Errorf("either both or none of clientCertFile and clientKeyFile must be set")
	}
	if config.ClientCertFile != "" && config.ClientKeyFile != "" && config.CAFile == "" {
		return nil, fmt.Errorf("setting clientCertFile and clientKeyFile requires caFile")
	}

	return &GRPCOptions{
		URL:            config.URL,
		CAFile:         config.CAFile,
		ClientCertFile: config.ClientCertFile,
		ClientKeyFile:  config.ClientKeyFile,
	}, nil
}

func NewGRPCOptions() *GRPCOptions {
	return &GRPCOptions{}
}

func (o *GRPCOptions) GetGRPCClientConn() (*grpc.ClientConn, error) {
	if len(o.CAFile) != 0 {
		certPool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}

		caPEM, err := os.ReadFile(o.CAFile)
		if err != nil {
			return nil, err
		}

		if ok := certPool.AppendCertsFromPEM(caPEM); !ok {
			return nil, fmt.Errorf("invalid CA %s", o.CAFile)
		}

		clientCerts, err := tls.LoadX509KeyPair(o.ClientCertFile, o.ClientKeyFile)
		if err != nil {
			return nil, err
		}

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{clientCerts},
			RootCAs:      certPool,
			MinVersion:   tls.VersionTLS13,
			MaxVersion:   tls.VersionTLS13,
		}

		conn, err := grpc.Dial(o.URL, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to grpc server %s, %v", o.URL, err)
		}

		return conn, nil
	}

	conn, err := grpc.Dial(o.URL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to grpc server %s, %v", o.URL, err)
	}

	return conn, nil
}

func (o *GRPCOptions) GetCloudEventsClient(ctx context.Context, errorHandler func(error), clientOpts ...protocol.Option) (cloudevents.Client, error) {
	conn, err := o.GetGRPCClientConn()
	if err != nil {
		return nil, err
	}

	// Periodically (every 100ms) check the connection status and reconnect if necessary.
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				conn.Close()
			case <-ticker.C:
				if conn.GetState() == connectivity.TransientFailure {
					errorHandler(fmt.Errorf("grpc connection is disconnected"))
					ticker.Stop()
					conn.Close()
					return // exit the goroutine as the error handler function will handle the reconnection.
				}
			}
		}
	}()

	opts := []protocol.Option{}
	opts = append(opts, clientOpts...)
	p, err := protocol.NewProtocol(conn, opts...)
	if err != nil {
		return nil, err
	}

	return cloudevents.NewClient(p)
}

// Replace the nth occurrence of old in str by new.
func replaceNth(str, old, new string, n int) string {
	i := 0
	for m := 1; m <= n; m++ {
		x := strings.Index(str[i:], old)
		if x < 0 {
			break
		}
		i += x
		if m == n {
			return str[:i] + new + str[i+len(old):]
		}
		i += len(old)
	}
	return str
}
