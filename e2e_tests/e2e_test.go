//go:build e2e

package e2e_tests

import (
	"github.com/IljaN/opencloud-sftp/e2e_tests/gateway"
	"github.com/IljaN/opencloud-sftp/e2e_tests/sftp"
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const EnvPrefix = "OC_SFTP_E2E_TEST_"

var testConfig Config

type Config struct {
	SFTPClient    sftp.Config    `envPrefix:"SFTP_CLIENT_"`
	GatewayClient gateway.Config `envPrefix:"GATEWAY_CLIENT_"`
}

func TestMain(m *testing.M) {
	err := godotenv.Load("config.env")
	if err != nil {
		panic(err)
	}

	err = env.ParseWithOptions(&testConfig, env.Options{
		Prefix: EnvPrefix,
	})

	if err != nil {
		log.Fatalf("Failed to parse environment variables: %v", err)
	}

	// Run the tests
	exitCode := m.Run()

	os.Exit(exitCode)
}

type TestSuite struct {
	cfg               *Config
	sftpClientFactory *ClientFactory
	testUsers         *userStates
}

func TestE2E(t *testing.T) {
	ts := &TestSuite{
		cfg:               &testConfig,
		sftpClientFactory: NewClientFactory(testConfig.SFTPClient, testConfig.GatewayClient),
		testUsers:         &userStates{},
	}

	// Register the test methods dynamically
	methods := getTypedMethods(ts, "Test")
	for _, method := range methods {
		t.Run(method.Name, method.Func)
	}
}

// GetGateway creates a new gateway client for the specified user ID to be used during a test case.
func (ts *TestSuite) GetGateway(uid string) *gateway.Client {
	client, err := gateway.NewClient(&ts.cfg.GatewayClient, uid)
	if err != nil {
		log.Fatalf("Failed to create gateway client: %v", err)
	}

	// Ensure the user has a home directory created, which is normally only done after the first login.
	if !ts.testUsers.hasHome(uid) {
		err = client.CreateHome()
		if err != nil {
			log.Fatalf("Failed to create home for user %s: %v", uid, err)
		}
		ts.testUsers.setHomeCreated(uid)
	}

	return client
}

func (ts *TestSuite) GetSFTPClient(uid string) (*sftp.Client, func()) {
	sc, err := ts.sftpClientFactory.NewClient(uid)
	if err != nil {
		log.Fatalf("Failed to create SFTP client: %v", err)
	}

	cleanup := func() {
		if err := sc.Close(); err != nil {
			log.Printf("Error closing SFTP client: %v", err)
		}

		err := ts.GetGateway(uid).ClearPersonalSpace()
		if err != nil {
			log.Printf("Error clearing personal space for user %s: %v", uid, err)
		}
	}

	return sc, cleanup
}

type MethodEntry struct {
	Name string
	Func func(t *testing.T)
}

func getTypedMethods(instance *TestSuite, prefix string) []MethodEntry {
	var methods []MethodEntry

	instanceValue := reflect.ValueOf(instance)
	instanceType := instanceValue.Type()

	for i := 0; i < instanceType.NumMethod(); i++ {
		method := instanceType.Method(i)
		if strings.HasPrefix(method.Name, prefix) {
			methodValue := instanceValue.Method(i)

			// Type assertion for func(*testing.T) signature
			if fn, ok := methodValue.Interface().(func(t *testing.T)); ok {
				methods = append(methods, MethodEntry{
					Name: method.Name,
					Func: fn,
				})
			}
		}
	}

	// Sort by method name to ensure consistent ordering
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name < methods[j].Name
	})

	return methods
}

type userStates map[string]struct {
	HomeCreated bool
}

func (us *userStates) setHomeCreated(uid string) {
	if _, exists := (*us)[uid]; !exists {
		(*us)[uid] = struct {
			HomeCreated bool
		}{}
	}
	(*us)[uid] = struct {
		HomeCreated bool
	}{HomeCreated: true}
}

func (us *userStates) hasHome(uid string) bool {
	_, exists := (*us)[uid]
	return exists && (*us)[uid].HomeCreated
}
