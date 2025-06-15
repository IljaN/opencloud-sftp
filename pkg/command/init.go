package command

import (
	"fmt"
	"github.com/IljaN/opencloud-sftp/pkg/config"
	"github.com/IljaN/opencloud-sftp/pkg/config/parser"
	"github.com/IljaN/opencloud-sftp/pkg/keygen"
	"github.com/goccy/go-yaml"
	"github.com/opencloud-eu/opencloud/pkg/config/configlog"
	ocdefaults "github.com/opencloud-eu/opencloud/pkg/config/defaults"
	"github.com/urfave/cli/v2"
	"os"
	"path"
	"reflect"
)

func Init(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:        "init",
		Usage:       "initialize OpenCloud SFTP service",
		Category:    "service",
		Description: `This command initializes the OpenCloud SFTP service by setting up the necessary configuration and environment.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "sftp-addr",
				Usage:       "Address for the SFTP server (host:port)",
				Destination: &cfg.SFTPAddress,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "host-private-key-path",
				Aliases:     []string{"k"},
				Usage:       "Path to the host private key for SFTP server",
				Value:       path.Join(ocdefaults.BaseDataPath(), cfg.Service.Name, "id_rsa"),
				Destination: &cfg.HostPrivateKeyPath,
				Required:    false,
			},
			&cli.BoolFlag{
				Name:     "force-overwrite",
				Aliases:  []string{"f"},
				EnvVars:  []string{"OC_FORCE_CONFIG_OVERWRITE", "OCSFTP_FORCE_CONFIG_OVERWRITE"},
				Value:    false,
				Usage:    "Force overwrite existing config file",
				Required: false,
			},
		},
		Before: func(c *cli.Context) error {
			if err := parser.ParseConfig(cfg); err != nil {
				return configlog.ReturnError(err)
			}

			return nil
		},
		Action: func(c *cli.Context) error {
			baseConfigPath := ocdefaults.BaseConfigPath()
			baseDataPath := ocdefaults.BaseDataPath()

			serviceConfigPath := path.Join(baseConfigPath, cfg.Service.Name) + ".yaml"
			serviceDataPath := path.Join(baseDataPath, cfg.Service.Name)
			hostKeyPath := cfg.HostPrivateKeyPath

			// Check file in serviceConfigPath exists
			configExists := fileExists(serviceConfigPath)
			hostKeyExists := fileExists(hostKeyPath)

			if (configExists || hostKeyExists) && !c.Bool("force-overwrite") {
				return fmt.Errorf("configuration already exists at %s or host key at %s", serviceConfigPath, hostKeyPath)
			}

			err := deleteFiles(serviceDataPath, serviceConfigPath)

			fmt.Printf("Genrating config %s\n", serviceConfigPath)
			rawYML, err := configSubsetToYAML(cfg, []string{"SFTPAddress", "HostPrivateKeyPath"})
			if err != nil {
				return fmt.Errorf("failed to generate YAML configuration: %v", err)
			}

			// Write the configuration to the service config file, overwrite existing file if it force-overwrite is set otherwise return error
			if err := os.WriteFile(serviceConfigPath, rawYML, 0644); err != nil {
				return fmt.Errorf("failed to write configuration to %s: %v", serviceConfigPath, err)
			}

			fmt.Printf("Creating service data directory: %s\n", serviceDataPath)
			// Create service data directory if it does not exist
			if err := os.MkdirAll(serviceDataPath, 0755); err != nil {
				return fmt.Errorf("failed to create service data directory %s: %v", serviceDataPath, err)
			}

			fmt.Printf("Generating SSH Host-Key pair in %s\n", serviceDataPath)
			keyPair, err := keygen.GenerateSSHKeyPair(keygen.KeyTypeRSA)
			if err != nil {
				return fmt.Errorf("failed to generate SSH key pair: %v", err)
			}

			fmt.Printf("...%s\n", path.Join(serviceDataPath, "id_rsa"))
			fmt.Printf("...%s\n", path.Join(serviceDataPath, "id_rsa.pub"))

			err = writeKeyPairToFile(keyPair, serviceDataPath, "id_rsa")
			if err != nil {
				return fmt.Errorf("failed to write key pair to file: %v", err)
			}

			return nil
		},
	}
}

func writeKeyPairToFile(keyPair *keygen.KeyPair, keyPath string, keyName string) error {
	privateKeyFile := path.Join(keyPath, keyName)
	publicKeyFile := path.Join(keyPath, keyName+".pub")

	if err := os.WriteFile(privateKeyFile, keyPair.PrivateKey, 0600); err != nil {
		return fmt.Errorf("failed to write private key to %s: %v", privateKeyFile, err)
	}

	if err := os.WriteFile(publicKeyFile, keyPair.PublicKey, 0644); err != nil {
		return fmt.Errorf("failed to write public key to %s: %v", publicKeyFile, err)
	}

	return nil
}

func configSubsetToYAML(cfg *config.Config, fieldNames []string) ([]byte, error) {
	// Extract the specified fields from the config struct
	extracted, err := extractFields(*cfg, fieldNames)
	if err != nil {
		return nil, fmt.Errorf("failed to extract fields: %v", err)
	}

	// Marshal the extracted fields to YAML
	yamlData, err := yaml.Marshal(extracted)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to YAML: %v", err)
	}

	return yamlData, nil
}

// extractFields takes any struct instance and a list of field names,
// and returns a new anonymous struct with only those fields (with tags).
func extractFields(input interface{}, fieldNames []string) (interface{}, error) {
	v := reflect.ValueOf(input)
	t := reflect.TypeOf(input)

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("input must be a struct")
	}

	fieldMap := make(map[string]struct{})
	for _, name := range fieldNames {
		fieldMap[name] = struct{}{}
	}

	var selectedFields []reflect.StructField
	var selectedValues []reflect.Value

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if _, ok := fieldMap[field.Name]; ok {
			selectedFields = append(selectedFields, reflect.StructField{
				Name: field.Name,
				Type: field.Type,
				Tag:  field.Tag,
			})
			selectedValues = append(selectedValues, v.Field(i))
		}
	}

	if len(selectedFields) == 0 {
		return nil, fmt.Errorf("no matching fields found")
	}

	newStructType := reflect.StructOf(selectedFields)
	newStruct := reflect.New(newStructType).Elem()

	for i := range selectedValues {
		newStruct.Field(i).Set(selectedValues[i])
	}

	return newStruct.Interface(), nil
}

func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func deleteFiles(paths ...string) error {
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %v", p, err)
		}
	}
	return nil
}
