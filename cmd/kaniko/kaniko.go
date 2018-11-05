package main

import (
	"github.com/justinbarrick/farm/pkg/executors/local"
	"log"
	"os"
	"fmt"
	"encoding/base64"
	"encoding/json"
)

type DockerAuth struct {
	Auth string `json"auth"`
}

type DockerConfig struct {
	Auths map[string]DockerAuth `json:"auths"`
}

func main() {
	if os.Getenv("DOCKER_USER") != "" && os.Getenv("DOCKER_PASS") != "" {
		config := DockerConfig{
			Auths: map[string]DockerAuth{},
		}

		auth := fmt.Sprintf("%s:%s", os.Getenv("DOCKER_USER"), os.Getenv("DOCKER_PASS"))
		token := base64.StdEncoding.EncodeToString([]byte(auth))

		registry := os.Getenv("DOCKER_REGISTRY")
		if registry == "" {
			registry = "index.docker.io"
		}

		config.Auths[fmt.Sprintf("https://%s/v1/", registry)] = DockerAuth{
			Auth: token,
		}

		err := os.MkdirAll("/root/.docker", 0600)
		if err != nil {
			log.Fatal(err)
		}

		cfgFile, err := os.OpenFile("/root/.docker/config.json", os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			log.Fatal(err)
		}

		err = json.NewEncoder(cfgFile).Encode(config)
		cfgFile.Close()
		if err != nil {
			log.Fatal(err)
		}
	}

	args := []string{"/executor",}
	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}

	if err := local.Exec(args); err != nil {
		log.Fatal(err)
	}
}