package main

import (
	"context"
	"flag"
	"log"

	"github.com/example/terraform-provider-optimizelycmssaas/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name optimizelycmssaas

var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/example/optimizelycmssaas",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
