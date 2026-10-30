// Copyright (c) RintUp
// SPDX-License-Identifier: MPL-2.0

// Command terraform-provider-rintup is the Terraform provider for RintUp (https://rintup.com).
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/rintuphq/terraform-provider-rintup/internal/provider"
)

// Docs are generated from the schemas and examples/ with tfplugindocs.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name rintup

// version is set by goreleaser (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/rintuphq/rintup",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
