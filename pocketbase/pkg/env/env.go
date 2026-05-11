package env

import (
	"log"

	"github.com/caarlos0/env/v8"
)

type config struct {
	Local               bool   `env:"LOCAL"`
	LocalKubeConfigFile string `env:"LOCAL_KUBECONFIG_FILE" envDefault:"~/.kube/config"`
	CronTick            string `env:"CRON_TICK" envDefault:"*/1 * * * *"`

	// EncryptionKeyHex is the 64-character hex-encoded 32-byte AES-256 key used
	// to encrypt AWS credentials at rest. Must be set and MUST NOT change between
	// restarts — changing it will make all stored credentials unreadable.
	// Generate with: openssl rand -hex 32
	EncryptionKeyHex string `env:"AUTOSTACK_ENCRYPTION_KEY"`

	// TerraformWorkDir is the root directory where per-deployment Terraform
	// working directories are created. Defaults to /tmp/autostack.
	TerraformWorkDir string `env:"AUTOSTACK_TERRAFORM_WORKDIR" envDefault:"/tmp/autostack"`
}

var Config config

func Init() {
	if err := env.Parse(&Config); err != nil {
		log.Printf("%+v\n", err)
	}

	if Config.Local {
		log.Println("Running in local mode and kubeconfig located at: " + Config.LocalKubeConfigFile)
	}
}
