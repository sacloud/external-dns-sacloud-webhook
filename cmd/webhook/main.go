// Copyright 2025- The sacloud/external-dns-sacloud-webhook authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/sacloud/external-dns-sacloud-webhook/internal/config"
	"github.com/sacloud/external-dns-sacloud-webhook/internal/server"
)

const banner = "==============================================================\n" +
	"   _____            __                __   ____  _   _______\n" +
	"  / ___/____ ______/ /___  __  ______/ /  / __ \\/ | / / ___/\n" +
	"  \\__ \\/ __ `/ ___/ / __ \\/ / / / __  /  / / / /  |/ /\\__ \\ \n" +
	" ___/ / /_/ / /__/ / /_/ / /_/ / /_/ /  / /_/ / /|  /___/ / \n" +
	"/____/\\__,_/\\___/_/\\____/\\__,_/\\__,_/  /_____/_/ |_//____/  \n" +
	"                                                             \n" +
	"SakuraCloud External-DNS Webhook\n" +
	"version: %s (%s)\n" +
	"==============================================================\n"

var (
	Version = "dev"
	GitSha  = "unknown"
)

func main() {
	viper.SetEnvPrefix("WEBHOOK")
	viper.AutomaticEnv()

	var cfgFile string
	root := &cobra.Command{
		Use:   "webhook",
		Short: "SakuraCloud External DNS webhook provider",
		PreRun: func(cmd *cobra.Command, args []string) {
			if cfgFile != "" {
				viper.SetConfigFile(cfgFile)
				viper.SetConfigType("yaml")
				if err := viper.ReadInConfig(); err != nil {
					log.Fatalf("Error reading config file '%s': %v", cfgFile, err)
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(banner, Version, GitSha)

			var cfg config.Config
			if err := viper.Unmarshal(&cfg); err != nil {
				log.Fatalf("failed to load configuration: %v", err)
			}

			server.Run(cfg)
		},
	}

	root.Flags().StringVar(&cfgFile, "config", "", "path to config file")
	root.Flags().String("sakura-api-token", "", "SakuraCloud API token")
	root.Flags().String("sakura-api-secret", "", "SakuraCloud API secret")
	root.Flags().String("provider-ip", "0.0.0.0", "Webhook listen host")
	root.Flags().String("provider-port", "8080", "Webhook listen port")
	root.Flags().Bool("registry-txt", false, "Enable TXT registry mode")
	root.Flags().String("txt-owner-id", "default", "TXT owner ID for registry mode")
	root.Flags().String("zone-name", "", "DNS zone name")

	if err := viper.BindPFlag("sakura-api-token", root.Flags().Lookup("sakura-api-token")); err != nil {
		log.Fatalf("failed to bind --sakura-api-token flag: %v", err)
	}
	if err := viper.BindPFlag("sakura-api-secret", root.Flags().Lookup("sakura-api-secret")); err != nil {
		log.Fatalf("failed to bind --sakura-api-secret flag: %v", err)
	}
	if err := viper.BindPFlag("provider-ip", root.Flags().Lookup("provider-ip")); err != nil {
		log.Fatalf("failed to bind --provider-ip flag: %v", err)
	}
	if err := viper.BindPFlag("provider-port", root.Flags().Lookup("provider-port")); err != nil {
		log.Fatalf("failed to bind --provider-port flag: %v", err)
	}
	if err := viper.BindPFlag("registry-txt", root.Flags().Lookup("registry-txt")); err != nil {
		log.Fatalf("failed to bind --registry-txt flag: %v", err)
	}
	if err := viper.BindPFlag("txt-owner-id", root.Flags().Lookup("txt-owner-id")); err != nil {
		log.Fatalf("failed to bind --txt-owner-id flag: %v", err)
	}
	if err := viper.BindPFlag("zone-name", root.Flags().Lookup("zone-name")); err != nil {
		log.Fatalf("failed to bind --zone-name flag: %v", err)
	}

	if err := viper.BindEnv("sakura-api-token", "WEBHOOK_SAKURA_API_TOKEN"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_SAKURA_API_TOKEN: %v", err)
	}
	if err := viper.BindEnv("sakura-api-secret", "WEBHOOK_SAKURA_API_SECRET"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_SAKURA_API_SECRET: %v", err)
	}
	if err := viper.BindEnv("provider-ip", "WEBHOOK_PROVIDER_IP"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_PROVIDER_IP: %v", err)
	}
	if err := viper.BindEnv("provider-port", "WEBHOOK_PROVIDER_PORT"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_PROVIDER_PORT: %v", err)
	}
	if err := viper.BindEnv("registry-txt", "WEBHOOK_REGISTRY_TXT"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_REGISTRY_TXT: %v", err)
	}
	if err := viper.BindEnv("txt-owner-id", "WEBHOOK_TXT_OWNER_ID"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_TXT_OWNER_ID: %v", err)
	}
	if err := viper.BindEnv("zone-name", "WEBHOOK_ZONE_NAME"); err != nil {
		log.Fatalf("failed to bind env WEBHOOK_ZONE_NAME: %v", err)
	}

	if err := root.Execute(); err != nil {
		log.Fatalf("command execution failed: %v", err)
		os.Exit(1)
	}
}
